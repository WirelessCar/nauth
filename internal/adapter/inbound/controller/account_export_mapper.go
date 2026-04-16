package controller

import (
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func mapResolutionToStatus(resolution *domain.AccountExportResolution, state *v1alpha1.AccountExport) (patchLabels bool, updateStatus bool) {
	patchLabels = false
	updateStatus = true
	status := &state.Status

	updateConditionFalse := func(condType string, reason string, msg string) {
		meta.SetStatusCondition(&status.Conditions, newCondition(condType, metav1.ConditionFalse, reason, msg))
	}
	updateConditionTrue := func(condType string) {
		meta.SetStatusCondition(&status.Conditions, newCondition(condType, metav1.ConditionTrue, conditionReasonOK, ""))
	}

	// rule validations
	if resolution.ValidationError != nil {
		updateConditionFalse(conditionTypeValidRules, conditionReasonInvalid, resolution.ValidationError.Error())
	} else {
		updateConditionTrue(conditionTypeValidRules)
		status.DesiredClaim = &v1alpha1.AccountExportClaim{
			Rules:              resolution.DesiredClaim.Rules,
			ObservedGeneration: resolution.ObservedGeneration,
		}
	}

	// account lookup
	if resolution.AccountID != "" {
		status.AccountID = resolution.AccountID
	}

	// account binding
	switch resolution.BindingState {
	case domain.AccountBindingStateMissing:
		if resolution.AccountID != "" {
			state.SetLabel(v1alpha1.AccountExportLabelAccountID, resolution.AccountID)
			patchLabels = true
			msg := fmt.Sprintf("Binding to Account ID: %s", resolution.AccountID)
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonReconciling, msg)
		} else {
			msg := fmt.Sprintf("Account not found or could not be read")
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonErrored, msg)
		}

	case domain.AccountBindingStateConflict:
		msg := fmt.Sprintf("Account ID conflict: previously bound to %s, now found %s", resolution.BoundAccountID, resolution.AccountID)
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonConflict, msg)

	case domain.AccountBindingStateBound:
		updateConditionTrue(conditionTypeBoundToAccount)
	}

	// account adoption condition
	switch resolution.AdoptionState {
	case domain.AccountAdoptionStateMissing:
		if resolution.AccountID == "" {
			msg := fmt.Sprintf("Account not found or could not be read")
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonErrored, msg)
		} else {
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonErrored, resolution.AdoptionError.Error())
		}

	case domain.AccountAdoptionStateNotAdopted:
		updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonReconciling, resolution.AdoptionError.Error())

	case domain.AccountAdoptionStateAdopted:
		updateConditionTrue(conditionTypeAdoptedByAccount)
	}

	// ready condition
	if isAccountExportReady(state) {
		updateConditionTrue(conditionTypeReady)
	} else {
		updateConditionFalse(conditionTypeReady, conditionReasonReconciling, "Reconciling")
	}

	return patchLabels, updateStatus
}

func newCondition(condType string, status metav1.ConditionStatus, reason string, msg string) metav1.Condition {
	return metav1.Condition{
		Type:    condType,
		Status:  status,
		Reason:  reason,
		Message: msg,
	}
}

func isAccountExportReady(state *v1alpha1.AccountExport) bool {
	conditionsReady := conditionsReady(state, []string{
		conditionTypeBoundToAccount,
		conditionTypeValidRules,
		conditionTypeAdoptedByAccount,
	})
	claimsReady := state.Status.DesiredClaim != nil && state.Status.DesiredClaim.ObservedGeneration == state.Generation

	return conditionsReady && claimsReady
}

func conditionsReady(state *v1alpha1.AccountExport, conditionType []string) bool {
	for _, ct := range conditionType {
		c := meta.FindStatusCondition(state.Status.Conditions, ct)
		ready := c != nil && c.Status == metav1.ConditionTrue
		if !ready {
			return false
		}
	}

	return true
}
