package controller

import (
	"reflect"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type accountExportAdoptionState struct {
	ObservedGeneration             int64
	Status                         metav1.ConditionStatus
	Reason                         string
	Message                        string
	DesiredClaimObservedGeneration *int64
}

func accountExportAccountWatchPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldAccount, oldOK := e.ObjectOld.(*v1alpha1.Account)
			newAccount, newOK := e.ObjectNew.(*v1alpha1.Account)
			return oldOK && newOK && accountUpdateAffectsAccountExports(oldAccount, newAccount)
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func accountUpdateAffectsAccountExports(oldAccount *v1alpha1.Account, newAccount *v1alpha1.Account) bool {
	if oldAccount == nil || newAccount == nil {
		return false
	}

	oldAccountID := oldAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	newAccountID := newAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	// this is only possible if account is deleted and recreated with new name
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountExportAdoptionSnapshot(oldAccount), accountExportAdoptionSnapshot(newAccount))
}

// Builds a comparable snapshot of export adoption state so Account changes relevant to AccountExports can be detected efficiently.
func accountExportAdoptionSnapshot(account *v1alpha1.Account) map[string]accountExportAdoptionState {
	if account == nil || account.Status.Adoptions == nil {
		return nil
	}

	adoptions := make(map[string]accountExportAdoptionState, len(account.Status.Adoptions.Exports))
	for _, adoption := range account.Status.Adoptions.Exports {
		adoptions[string(adoption.UID)] = accountExportAdoptionState{
			ObservedGeneration:             adoption.ObservedGeneration,
			Status:                         adoption.Status.Status,
			Reason:                         adoption.Status.Reason,
			Message:                        adoption.Status.Message,
			DesiredClaimObservedGeneration: adoption.Status.DesiredClaimObservedGeneration,
		}
	}
	return adoptions
}
