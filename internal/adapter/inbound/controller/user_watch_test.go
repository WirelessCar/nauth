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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ---- predicate tests ----

func TestAccountWatchPredicateForUsers_UpdateFunc(t *testing.T) {
	pred := accountWatchPredicateForUsers()

	baseAccount := func() *v1alpha1.Account {
		return &v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "ns"},
			Spec: v1alpha1.AccountSpec{
				SigningKeyRefs: []string{"key-a"},
			},
			Status: v1alpha1.AccountStatus{
				Claims: &v1alpha1.AccountClaims{
					SigningKeys: v1alpha1.SigningKeys{
						{Key: "APUBKEY1"},
					},
				},
			},
		}
	}

	tests := []struct {
		name          string
		mutate        func(*v1alpha1.Account)
		expectTrigger bool
	}{
		{
			name: "spec.signingKeyRefs_changed",
			mutate: func(a *v1alpha1.Account) {
				a.Spec.SigningKeyRefs = append(a.Spec.SigningKeyRefs, "key-b")
			},
			expectTrigger: true,
		},
		{
			name: "spec.signingKeyRefs_removed",
			mutate: func(a *v1alpha1.Account) {
				a.Spec.SigningKeyRefs = nil
			},
			expectTrigger: true,
		},
		{
			name: "status.claims.signingKeys_key_added",
			mutate: func(a *v1alpha1.Account) {
				a.Status.Claims.SigningKeys = append(a.Status.Claims.SigningKeys, &v1alpha1.SigningKey{Key: "APUBKEY2"})
			},
			expectTrigger: true,
		},
		{
			name: "status.claims.signingKeys_key_removed",
			mutate: func(a *v1alpha1.Account) {
				a.Status.Claims.SigningKeys = nil
			},
			expectTrigger: true,
		},
		{
			name: "status.claims_set_to_nil",
			mutate: func(a *v1alpha1.Account) {
				a.Status.Claims = nil
			},
			expectTrigger: true,
		},
		{
			name:          "no_relevant_change",
			mutate:        func(*v1alpha1.Account) {},
			expectTrigger: false,
		},
		{
			name: "metadata_only_label_change",
			mutate: func(a *v1alpha1.Account) {
				a.Labels = map[string]string{"some-label": "some-value"}
			},
			expectTrigger: false,
		},
		{
			name: "status_conditions_only_changed",
			mutate: func(a *v1alpha1.Account) {
				a.Status.Conditions = []metav1.Condition{
					{Type: conditionTypeReady, Status: metav1.ConditionTrue, Reason: conditionReasonOK, LastTransitionTime: metav1.Now()},
				}
			},
			expectTrigger: false,
		},
		{
			name: "status_observed_generation_only_changed",
			mutate: func(a *v1alpha1.Account) {
				a.Status.ObservedGeneration = 42
			},
			expectTrigger: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldAcc := baseAccount()
			newAcc := oldAcc.DeepCopy()
			tt.mutate(newAcc)

			got := pred.UpdateFunc(event.UpdateEvent{ObjectOld: oldAcc, ObjectNew: newAcc})
			assert.Equal(t, tt.expectTrigger, got)
		})
	}
}

func TestAccountWatchPredicateForUsers_CreateAndDelete(t *testing.T) {
	pred := accountWatchPredicateForUsers()

	acc := &v1alpha1.Account{ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "ns"}}
	assert.False(t, pred.CreateFunc(event.CreateEvent{Object: acc}), "Create should not trigger")
	assert.True(t, pred.DeleteFunc(event.DeleteEvent{Object: acc}), "Delete should trigger")
	assert.False(t, pred.GenericFunc(event.GenericEvent{Object: acc}), "Generic should not trigger")
}

func TestMapAccountToUsers_EnqueuesOnlyUsersWithSigningKeyRef(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "my-account", Namespace: "ns"},
	}

	// user with matching accountName and signingKeyRef set — should be enqueued
	userWithRef := &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-with-ref", Namespace: "ns"},
		Spec: v1alpha1.UserSpec{
			AccountName:   "my-account",
			SigningKeyRef: "my-ask",
		},
	}
	// user with matching accountName but no signingKeyRef — must not be enqueued
	userNoRef := &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-no-ref", Namespace: "ns"},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}
	// user with a different accountName — must not be enqueued
	userDiffAccount := &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-diff-account", Namespace: "ns"},
		Spec: v1alpha1.UserSpec{
			AccountName:   "other-account",
			SigningKeyRef: "my-ask",
		},
	}
	// user in a different namespace — must not be enqueued
	userDiffNS := &v1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-diff-ns", Namespace: "other-ns"},
		Spec: v1alpha1.UserSpec{
			AccountName:   "my-account",
			SigningKeyRef: "my-ask",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(account, userWithRef, userNoRef, userDiffAccount, userDiffNS).
		Build()

	r := &UserReconciler{Client: fakeClient}
	requests := r.mapAccountToUsers(context.Background(), account)

	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: client.ObjectKeyFromObject(userWithRef),
	}, requests[0])
}

func TestMapAccountToUsers_ReturnsEmptyWhenNoMatchingUsers(t *testing.T) {
	testScheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(testScheme))

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "lonely-account", Namespace: "ns"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(account).
		Build()

	r := &UserReconciler{Client: fakeClient}
	requests := r.mapAccountToUsers(context.Background(), account)
	assert.Empty(t, requests)
}
