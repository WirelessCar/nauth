package controller

import (
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type accountExportStatus struct {
	accountExport *v1alpha1.AccountExport
	failed        error
}

func (s *accountExportStatus) setBoundToAccountOK(accountID string) {
	s.accountExport.Status.AccountID = accountID
	c := &metav1.Condition{
		Type:               conditionTypeBoundToAccount,
		Status:             metav1.ConditionTrue,
		Reason:             conditionReasonOK,
		Message:            fmt.Sprintf("Account ID: %s", accountID),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setBoundToAccountNotFound(err error) {
	c := &metav1.Condition{
		Type:               conditionTypeBoundToAccount,
		Status:             metav1.ConditionFalse,
		Reason:             string(metav1.StatusReasonNotFound),
		Message:            err.Error(),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setBoundToAccountConflict(boundAccountID string, newAccountID string) {
	c := &metav1.Condition{
		Type:               conditionTypeBoundToAccount,
		Status:             metav1.ConditionFalse,
		Reason:             conditionReasonConflict,
		Message:            fmt.Sprintf("Account ID conflict: previously bound to %s, now found %s", boundAccountID, newAccountID),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setStatusValidRules(rules []v1alpha1.AccountExportRule) {
	s.accountExport.Status.Claim = &v1alpha1.AccountExportClaim{
		Rules:              rules,
		ObservedGeneration: s.accountExport.Generation,
	}

	c := &metav1.Condition{
		Type:               conditionTypeValidRules,
		Status:             metav1.ConditionTrue,
		Reason:             conditionReasonOK,
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setStatusValidRulesFalse(err error) {
	c := &metav1.Condition{
		Type:               conditionTypeValidRules,
		Status:             metav1.ConditionFalse,
		Reason:             conditionReasonInvalid,
		Message:            err.Error(),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setAdoptedByAccount() {
	c := &metav1.Condition{
		Type:               conditionTypeAdoptedByAccount,
		Status:             metav1.ConditionFalse,
		Reason:             "NotImplemented",
		Message:            "Not Implemented",
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setFailed(err error) {
	s.failed = err
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) evaluateReadyCondition() {
	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: s.accountExport.Generation,
	}

	if s.failed != nil {
		readyCondition.Reason = conditionReasonErrored
		readyCondition.Message = s.failed.Error()
	} else {
		if s.isReady([]string{
			conditionTypeBoundToAccount,
			conditionTypeValidRules,
			conditionTypeAdoptedByAccount,
		}) {
			readyCondition.Status = metav1.ConditionTrue
			readyCondition.Reason = conditionReasonReady
		} else {
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Reason = conditionReasonNotReady
		}
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), readyCondition)
}

func (s *accountExportStatus) isReady(conditionType []string) bool {
	if s.failed != nil {
		return false
	}

	for _, ct := range conditionType {
		c := meta.FindStatusCondition(*s.accountExport.GetConditions(), ct)
		ready := c != nil && c.Status == metav1.ConditionTrue && c.ObservedGeneration == s.accountExport.Generation
		if !ready {
			return false
		}
	}

	return true
}
