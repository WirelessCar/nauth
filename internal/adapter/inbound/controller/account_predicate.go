package controller

import (
	"reflect"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type accountExportDesiredClaimState struct {
	ObservedGeneration int64
	Rules              []v1alpha1.AccountExportRule
}

func accountExportWatchPredicateForAccounts() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountExport)
			return ok && export.GetLabel(v1alpha1.AccountExportLabelAccountID) != ""
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountExport)
			return ok && export.GetLabel(v1alpha1.AccountExportLabelAccountID) != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldExport, oldOK := e.ObjectOld.(*v1alpha1.AccountExport)
			newExport, newOK := e.ObjectNew.(*v1alpha1.AccountExport)
			return oldOK && newOK && accountExportUpdateAffectsAccounts(oldExport, newExport)
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func accountExportUpdateAffectsAccounts(oldExport *v1alpha1.AccountExport, newExport *v1alpha1.AccountExport) bool {
	if oldExport == nil || newExport == nil {
		return false
	}

	oldAccountID := oldExport.GetLabel(v1alpha1.AccountExportLabelAccountID)
	newAccountID := newExport.GetLabel(v1alpha1.AccountExportLabelAccountID)
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountExportDesiredClaimSnapshot(oldExport), accountExportDesiredClaimSnapshot(newExport))
}

func accountExportDesiredClaimSnapshot(export *v1alpha1.AccountExport) *accountExportDesiredClaimState {
	if export == nil || export.Status.DesiredClaim == nil {
		return nil
	}

	claim := export.Status.DesiredClaim
	rules := make([]v1alpha1.AccountExportRule, len(claim.Rules))
	copy(rules, claim.Rules)

	return &accountExportDesiredClaimState{
		ObservedGeneration: claim.ObservedGeneration,
		Rules:              rules,
	}
}
