package controller

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAccountImportReconciler_ShouldReconcileForExportAccountUpdate(t *testing.T) {
	createAccount := func() *v1alpha1.Account {
		return &v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "export-account",
				Namespace: "ns-a",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountIDAccA,
				},
			},
			Status: v1alpha1.AccountStatus{
				ObservedGeneration: 3,
				OperatorVersion:    "v1.0.0",
				ClaimsHash:         "hash-a",
				ReconcileTimestamp: metav1.Now(),
				Adoptions: &v1alpha1.AccountAdoptions{
					Imports: []v1alpha1.AccountAdoption{
						{
							Name:               "import-a",
							UID:                types.UID("import-uid"),
							ObservedGeneration: 1,
							Status: v1alpha1.AccountAdoptionStatus{
								Status: metav1.ConditionFalse,
								Reason: conditionReasonReconciling,
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name          string
		mutate        func(account *v1alpha1.Account)
		expectRequeue bool
	}{
		{
			name: "bound_account_id_label_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Labels[string(v1alpha1.AccountLabelAccountID)] = accountIDAccB
			},
			expectRequeue: true,
		},
		{
			name: "bound_account_id_label_removed",
			mutate: func(account *v1alpha1.Account) {
				delete(account.Labels, string(v1alpha1.AccountLabelAccountID))
			},
			expectRequeue: true,
		},
		{
			name: "adoption_status_only_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.Adoptions.Imports[0].Status.Status = metav1.ConditionTrue
				account.Status.Adoptions.Imports[0].Status.Reason = conditionReasonOK
			},
			expectRequeue: false,
		},
		{
			name: "claims_hash_only_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.ClaimsHash = "hash-b"
			},
			expectRequeue: false,
		},
		{
			name: "observed_generation_only_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.ObservedGeneration = 4
			},
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldAccount := createAccount()
			newAccount := oldAccount.DeepCopy()
			tt.mutate(newAccount)

			assert.Equal(t, tt.expectRequeue, accountUpdateAffectsExportAccountImports(oldAccount, newAccount))
		})
	}
}

func TestAccountImportReconciler_MapExportAccountToImports(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	byExportAccountRefIndexFunc := func(rawObj client.Object) []string {
		imp := rawObj.(*v1alpha1.AccountImport)
		ref := imp.Spec.ExportAccountRef
		if ref.Name == "" && ref.Namespace == "" {
			return nil
		}

		nsn := types.NamespacedName{
			Namespace: ref.Namespace,
			Name:      ref.Name,
		}
		if ref.Namespace == "" {
			nsn.Namespace = imp.Namespace
		}
		return []string{nsn.String()}
	}

	exportAccount := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "export-account",
			Namespace: "export-ns",
		},
	}
	matchingExplicitNamespace := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-explicit",
			Namespace: "import-ns",
		},
		Spec: v1alpha1.AccountImportSpec{
			ExportAccountRef: v1alpha1.AccountRef{
				Name:      "export-account",
				Namespace: "export-ns",
			},
		},
	}
	matchingImplicitNamespace := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-implicit",
			Namespace: "export-ns",
		},
		Spec: v1alpha1.AccountImportSpec{
			ExportAccountRef: v1alpha1.AccountRef{
				Name: "export-account",
			},
		},
	}
	nonMatchingImport := &v1alpha1.AccountImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "import-other",
			Namespace: "export-ns",
		},
		Spec: v1alpha1.AccountImportSpec{
			ExportAccountRef: v1alpha1.AccountRef{
				Name: "other-account",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithIndex(&v1alpha1.AccountImport{}, importExportAccountRefIndexKey, byExportAccountRefIndexFunc).
		WithObjects(exportAccount, matchingExplicitNamespace, matchingImplicitNamespace, nonMatchingImport).
		Build()

	reconciler := &AccountImportReconciler{Client: fakeClient}

	requests := reconciler.mapExportAccountToAccountImports(context.Background(), exportAccount)
	require.Len(t, requests, 2)
	assert.ElementsMatch(t, []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{Namespace: "import-ns", Name: "import-explicit"},
		},
		{
			NamespacedName: types.NamespacedName{Namespace: "export-ns", Name: "import-implicit"},
		},
	}, requests)
}
