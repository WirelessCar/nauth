package controller

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAccountReconciler_ShouldReconcileForAccountExportUpdate(t *testing.T) {
	createExport := func() *v1alpha1.AccountExport {
		return &v1alpha1.AccountExport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "export-a",
				Namespace: "ns-a",
				Labels: map[string]string{
					string(v1alpha1.AccountExportLabelAccountID): accountIDAccA,
				},
			},
			Status: v1alpha1.AccountExportStatus{
				AccountID: accountIDAccA,
				DesiredClaim: &v1alpha1.AccountExportClaim{
					ObservedGeneration: 1,
					Rules: []v1alpha1.AccountExportRule{
						{Name: "rule-a", Subject: "foo.*", Type: v1alpha1.Stream},
					},
				},
				Conditions: []metav1.Condition{
					{Type: conditionTypeReady, Status: metav1.ConditionFalse, Reason: conditionReasonReconciling},
				},
				ObservedGeneration: 1,
				OperatorVersion:    "v1.0.0",
			},
		}
	}

	tests := []struct {
		name          string
		mutate        func(export *v1alpha1.AccountExport)
		expectRequeue bool
	}{
		{
			name: "bound_account_id_label_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Labels[string(v1alpha1.AccountExportLabelAccountID)] = accountIDAccB
			},
			expectRequeue: true,
		},
		{
			name: "desired_claim_observed_generation_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.DesiredClaim.ObservedGeneration = 2
			},
			expectRequeue: true,
		},
		{
			name: "desired_claim_rules_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.DesiredClaim.Rules = append(export.Status.DesiredClaim.Rules, v1alpha1.AccountExportRule{
					Name: "rule-b", Subject: "bar.*", Type: v1alpha1.Stream,
				})
			},
			expectRequeue: true,
		},
		{
			name: "desired_claim_removed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.DesiredClaim = nil
			},
			expectRequeue: true,
		},
		{
			name: "conditions_only_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.Conditions[0].Status = metav1.ConditionTrue
				export.Status.Conditions[0].Reason = conditionReasonOK
			},
			expectRequeue: false,
		},
		{
			name: "status_account_id_only_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.AccountID = accountIDAccB
			},
			expectRequeue: false,
		},
		{
			name: "observed_generation_only_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.ObservedGeneration = 2
			},
			expectRequeue: false,
		},
		{
			name: "operator_version_only_changed",
			mutate: func(export *v1alpha1.AccountExport) {
				export.Status.OperatorVersion = "v1.1.0"
			},
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldExport := createExport()
			newExport := oldExport.DeepCopy()
			tt.mutate(newExport)

			assert.Equal(t, tt.expectRequeue, accountExportUpdateAffectsAccounts(oldExport, newExport))
		})
	}
}

func TestAccountReconciler_MapAccountExportToAccounts(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	accountA := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-a",
			Namespace: "ns-a",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountIDAccA,
			},
		},
	}
	accountB := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-b",
			Namespace: "ns-a",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountIDAccB,
			},
		},
	}
	accountOtherNamespace := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-c",
			Namespace: "ns-b",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountIDAccA,
			},
		},
	}
	export := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "export-a",
			Namespace: "ns-a",
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): accountIDAccA,
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(accountA, accountB, accountOtherNamespace, export).
		Build()

	reconciler := &AccountReconciler{kubernetes: newKubernetesClient(fakeClient)}

	requests := reconciler.mapAccountExportToAccounts(context.Background(), export)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: client.ObjectKeyFromObject(accountA),
	}, requests[0])
}

func TestAccountReconciler_MapExportAccountToImportingAccounts(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	byExportAccountRefIndexFunc := func(rawObj client.Object) []string {
		imp := rawObj.(*v1alpha1.AccountImport)
		ref := imp.Spec.ExportAccountRef
		if ref.Name == "" && ref.Namespace == "" {
			return nil
		}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = imp.Namespace
		}
		return []string{types.NamespacedName{Namespace: namespace, Name: ref.Name}.String()}
	}

	exportAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "export-account", Namespace: "export-ns"},
	}
	targetAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-account",
			Namespace: "import-ns",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountIDAccA,
			},
		},
	}
	matchingImport := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-rule",
			Namespace: "import-ns",
			Labels: map[string]string{
				string(v1alpha1.AccountImportLabelAccountID): accountIDAccA,
			},
		},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: "import-account",
			ExportAccountRef: v1alpha1.AccountRef{
				Name:      "export-account",
				Namespace: "export-ns",
			},
		},
	}
	nonMatchingImport := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{Name: "import-other", Namespace: "import-ns"},
		Spec: v1alpha1.AccountImportSpec{
			AccountName: "import-account",
			ExportAccountRef: v1alpha1.AccountRef{
				Name:      "other-account",
				Namespace: "export-ns",
			},
		},
	}
	inlineTargetAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "inline-import-account", Namespace: "inline-ns"},
		Spec: v1alpha1.AccountSpec{
			Imports: v1alpha1.Imports{{
				AccountRef: v1alpha1.AccountRef{
					Name:      "export-account",
					Namespace: "export-ns",
				},
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithIndex(&v1alpha1.AccountImport{}, importExportAccountRefIndexKey, byExportAccountRefIndexFunc).
		WithObjects(exportAccount, targetAccount, matchingImport, nonMatchingImport, inlineTargetAccount).
		Build()

	reconciler := &AccountReconciler{kubernetes: newKubernetesClient(fakeClient)}
	requests := reconciler.mapExportAccountToImportingAccounts(context.Background(), exportAccount)

	assert.ElementsMatch(t, []reconcile.Request{
		{NamespacedName: client.ObjectKeyFromObject(targetAccount)},
		{NamespacedName: client.ObjectKeyFromObject(inlineTargetAccount)},
	}, requests)
}

func TestAccountReconciler_ImportDependenciesHashIncludesInlineAndManagedExportAccounts(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	inlineExportAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "inline-export-account", Namespace: "export-ns"},
		Status:     v1alpha1.AccountStatus{ClaimsHash: "claims-inline"},
	}
	managedExportAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "managed-export-account", Namespace: "export-ns"},
		Status:     v1alpha1.AccountStatus{ClaimsHash: "claims-resource"},
	}
	target := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "target-ns"},
		Spec: v1alpha1.AccountSpec{
			Imports: v1alpha1.Imports{{
				AccountRef: v1alpha1.AccountRef{Name: inlineExportAccount.Name, Namespace: inlineExportAccount.Namespace},
			}},
		},
	}
	accountImport := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-import", Namespace: target.Namespace},
		Spec: v1alpha1.AccountImportSpec{
			ExportAccountRef: v1alpha1.AccountRef{Name: managedExportAccount.Name, Namespace: managedExportAccount.Namespace},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(inlineExportAccount, managedExportAccount, target).
		Build()
	reconciler := &AccountReconciler{kubernetes: newKubernetesClient(fakeClient)}

	fingerprint, err := reconciler.importDependenciesHash(context.Background(), target, &v1alpha1.AccountImportList{Items: []v1alpha1.AccountImport{*accountImport}})

	require.NoError(t, err)
	assert.Equal(t, nauth.HashAccountImportDependencies([]nauth.AccountImportDependency{
		{AccountRef: domain.NewNamespacedName(inlineExportAccount.Namespace, inlineExportAccount.Name), ClaimsHash: inlineExportAccount.Status.ClaimsHash},
		{AccountRef: domain.NewNamespacedName(managedExportAccount.Namespace, managedExportAccount.Name), ClaimsHash: managedExportAccount.Status.ClaimsHash},
	}), fingerprint)
}

func TestAccountReconciler_MapAccountSigningKeyToAccounts(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	signingKey := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{Name: "sk-1", Namespace: "ns-a"},
	}
	// references sk-1
	accountReferencing := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "account-a", Namespace: "ns-a"},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: []v1alpha1.AccountSigningKeyRef{{Name: "sk-1"}}},
	}
	// references a different key
	accountOtherKey := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "account-b", Namespace: "ns-a"},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: []v1alpha1.AccountSigningKeyRef{{Name: "sk-other"}}},
	}
	// no signing key refs
	accountNoRefs := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "account-c", Namespace: "ns-a"},
	}
	// different namespace, ref defaults to account's own namespace — excluded
	accountOtherNSDefault := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "account-d", Namespace: "ns-b"},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: []v1alpha1.AccountSigningKeyRef{{Name: "sk-1"}}},
	}
	// different namespace, ref explicitly points at ns-a/sk-1 — must match
	accountOtherNSCross := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "account-e", Namespace: "ns-b"},
		Spec:       v1alpha1.AccountSpec{SigningKeyRefs: []v1alpha1.AccountSigningKeyRef{{Name: "sk-1", Namespace: "ns-a"}}},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(signingKey, accountReferencing, accountOtherKey, accountNoRefs, accountOtherNSDefault, accountOtherNSCross).
		Build()

	reconciler := &AccountReconciler{kubernetes: newKubernetesClient(fakeClient)}
	requests := reconciler.mapAccountSigningKeyToAccounts(context.Background(), signingKey)

	require.Len(t, requests, 2)
	assert.ElementsMatch(t,
		[]reconcile.Request{
			{NamespacedName: client.ObjectKeyFromObject(accountReferencing)},
			{NamespacedName: client.ObjectKeyFromObject(accountOtherNSCross)},
		},
		requests,
	)
}

func TestAccountReconciler_ShouldReconcileForAccountSigningKeyUpdate(t *testing.T) {
	readyCond := metav1.Condition{Type: conditionTypeReady, Status: metav1.ConditionTrue, Reason: conditionReasonOK}
	notReadyCond := metav1.Condition{Type: conditionTypeReady, Status: metav1.ConditionFalse, Reason: conditionReasonReconciling}

	createASK := func(ready metav1.Condition, publicKey string) *v1alpha1.AccountSigningKey {
		return &v1alpha1.AccountSigningKey{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-1", Namespace: "ns-a"},
			Status: v1alpha1.AccountSigningKeyStatus{
				PublicKey:  publicKey,
				Conditions: []metav1.Condition{ready},
			},
		}
	}

	tests := []struct {
		name          string
		old           *v1alpha1.AccountSigningKey
		new           *v1alpha1.AccountSigningKey
		expectRequeue bool
	}{
		{
			name:          "ready_transition_false_to_true",
			old:           createASK(notReadyCond, ""),
			new:           createASK(readyCond, "APUBKEY"),
			expectRequeue: true,
		},
		{
			name:          "ready_transition_true_to_false",
			old:           createASK(readyCond, "APUBKEY"),
			new:           createASK(notReadyCond, ""),
			expectRequeue: true,
		},
		{
			name:          "public_key_changed",
			old:           createASK(readyCond, "APUBKEY1"),
			new:           createASK(readyCond, "APUBKEY2"),
			expectRequeue: true,
		},
		{
			name:          "unrelated_status_change",
			old:           createASK(readyCond, "APUBKEY"),
			new:           createASK(readyCond, "APUBKEY"),
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pred := accountSigningKeyWatchPredicateForAccounts()
			got := pred.Update(newUpdateEvent(tt.old, tt.new))
			assert.Equal(t, tt.expectRequeue, got)
		})
	}
}

func TestAccountReconciler_AccountSigningKeyPredicate_CreateAndDeleteAlwaysRequeue(t *testing.T) {
	ask := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{Name: "sk-1", Namespace: "ns-a"},
	}
	pred := accountSigningKeyWatchPredicateForAccounts()

	assert.True(t, pred.Create(event.CreateEvent{Object: ask}), "create should always trigger requeue")
	assert.True(t, pred.Delete(event.DeleteEvent{Object: ask}), "delete should always trigger requeue")
	assert.False(t, pred.Generic(event.GenericEvent{Object: ask}), "generic should never trigger requeue")
}

func newUpdateEvent(old, new client.Object) event.UpdateEvent {
	return event.UpdateEvent{ObjectOld: old, ObjectNew: new}
}
