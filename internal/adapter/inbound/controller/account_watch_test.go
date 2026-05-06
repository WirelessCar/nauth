package controller

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

			assert.Equal(t, tt.expectRequeue, accountExportUpdateAffectsReferencedResources(oldExport, newExport))
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

	reconciler := &AccountReconciler{Client: fakeClient}

	requests := reconciler.mapAccountExportToAccounts(context.Background(), export)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: client.ObjectKeyFromObject(accountA),
	}, requests[0])
}
