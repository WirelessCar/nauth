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
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonBinding, msg)
		} else {
			msg := "Account not found or could not be read"
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonNotFound, msg)
		}

	case domain.AccountBindingStateConflict:
		msg := fmt.Sprintf("Account ID conflict: previously bound to %s, now found %s", resolution.BoundAccountID, resolution.AccountID)
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonConflict, msg)

	case domain.AccountBindingStateBound:
		updateConditionTrue(conditionTypeBoundToAccount)

	default:
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonErrored, "Unknown binding state")
	}

	// account adoption condition
	switch resolution.AdoptionState {
	case domain.AccountAdoptionStateMissing:
		if resolution.AccountID == "" {
			msg := "Account not found or could not be read"
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonNotFound, msg)
		} else {
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonFailed, resolution.AdoptionError.Error())
		}

	case domain.AccountAdoptionStateNotAdopted:
		updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonAdopting, resolution.AdoptionError.Error())

	case domain.AccountAdoptionStateAdopted:
		updateConditionTrue(conditionTypeAdoptedByAccount)

	default:
		updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonErrored, "Unknown adopted state")
	}

	// ready condition
	if isAccountExportReady(state) {
		updateConditionTrue(conditionTypeReady)
	} else {
		updateConditionFalse(conditionTypeReady, conditionReasonReconciling, "All conditions not met")
	}

	return patchLabels, updateStatus
}

func isAccountExportReady(state *v1alpha1.AccountExport) bool {
	allConditionsReady := conditionsReady(state.Status.Conditions, []string{
		conditionTypeBoundToAccount,
		conditionTypeValidRules,
		conditionTypeAdoptedByAccount,
	})
	claimsReady := state.Status.DesiredClaim != nil && state.Status.DesiredClaim.ObservedGeneration == state.Generation

	return allConditionsReady && claimsReady
}
