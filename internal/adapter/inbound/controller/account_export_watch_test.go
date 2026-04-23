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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestAccountExportReconciler_ShouldReconcileForAccountUpdate(t *testing.T) {
	exportUID := types.UID("export-uid")
	desiredGen1 := int64(1)
	desiredGen2 := int64(2)

	createAccount := func() *v1alpha1.Account {
		return &v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "account-a",
				Namespace: "ns-a",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): "ACCA",
				},
			},
			Status: v1alpha1.AccountStatus{
				ClaimsHash:         "hash-a",
				ObservedGeneration: 3,
				OperatorVersion:    "v1.0.0",
				ReconcileTimestamp: metav1.Now(),
				Adoptions: &v1alpha1.AccountAdoptions{
					Exports: []v1alpha1.AccountAdoption{
						{
							Name:               "export-a",
							UID:                exportUID,
							ObservedGeneration: 1,
							Status: v1alpha1.AccountAdoptionStatus{
								Status:                         metav1.ConditionFalse,
								Reason:                         conditionReasonReconciling,
								Message:                        "waiting",
								DesiredClaimObservedGeneration: &desiredGen1,
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
			// this is only possible if account is deleted and recreated with new name
			name: "account_id_label_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Labels[string(v1alpha1.AccountLabelAccountID)] = "ACCB"
			},
			expectRequeue: true,
		},
		{
			name: "adoption_status_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.Adoptions.Exports[0].Status.Status = metav1.ConditionTrue
				account.Status.Adoptions.Exports[0].Status.Reason = conditionReasonOK
				account.Status.Adoptions.Exports[0].Status.Message = ""
			},
			expectRequeue: true,
		},
		{
			name: "desired_claim_generation_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.Adoptions.Exports[0].Status.DesiredClaimObservedGeneration = &desiredGen2
			},
			expectRequeue: true,
		},
		{
			name: "adoption_removed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.Adoptions.Exports = nil
			},
			expectRequeue: true,
		},
		{
			name: "observed_generation_only_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.ObservedGeneration = 4
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
			name: "operator_version_only_changed",
			mutate: func(account *v1alpha1.Account) {
				account.Status.OperatorVersion = "v1.1.0"
			},
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldAccount := createAccount()
			newAccount := oldAccount.DeepCopy()
			tt.mutate(newAccount)

			assert.Equal(t, tt.expectRequeue, accountUpdateAffectsAccountExports(oldAccount, newAccount))
		})
	}
}

func TestAccountExportReconciler_MapAccountToExports(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-a",
			Namespace: "ns-a",
		},
	}
	exportA := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "export-a",
			Namespace: "ns-a",
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-a",
		},
	}
	exportB := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "export-b",
			Namespace: "ns-a",
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-b",
		},
	}
	exportOtherNamespace := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "export-c",
			Namespace: "ns-b",
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-a",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithIndex(&v1alpha1.AccountExport{}, exportAccountNameIndexKey, exportBySpecAccountNameIndexFunc).
		WithObjects(account, exportA, exportB, exportOtherNamespace).
		Build()

	reconciler := &AccountExportReconciler{Client: fakeClient}

	requests := reconciler.mapAccountToAccountExports(context.Background(), account)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "ns-a",
			Name:      "export-a",
		},
	}, requests[0])
}
