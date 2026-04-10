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

func (s *accountExportStatus) setAccountFound(accountID string) {
	c := &metav1.Condition{
		Type:               "AccountFound",
		Status:             metav1.ConditionTrue,
		Reason:             "Found",
		Message:            fmt.Sprintf("Account ID: %s", accountID),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setAccountFoundFalse(err error) {
	c := &metav1.Condition{
		Type:               "AccountFound",
		Status:             metav1.ConditionFalse,
		Reason:             "Unknown",
		Message:            err.Error(),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setStatusValidRules(rules []v1alpha1.AccountExportRule) {
	s.accountExport.Status.Claim = &v1alpha1.AccountExportClaim{
		Rules: rules,
	}

	c := &metav1.Condition{
		Type:               "ValidRules",
		Status:             metav1.ConditionTrue,
		Reason:             "Valid",
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setStatusValidRulesFalse(err error) {
	c := &metav1.Condition{
		Type:               "ValidRules",
		Status:             metav1.ConditionFalse,
		Reason:             "Invalid",
		Message:            err.Error(),
		ObservedGeneration: s.accountExport.Generation,
	}
	meta.SetStatusCondition(s.accountExport.GetConditions(), *c)
	s.evaluateReadyCondition()
}

func (s *accountExportStatus) setAdoptedByAccount() {
	c := &metav1.Condition{
		Type:               "AdoptedByAccount",
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
		Type:               "Ready",
		ObservedGeneration: s.accountExport.Generation,
	}

	if s.failed != nil {
		readyCondition.Reason = "Errored"
		readyCondition.Message = s.failed.Error()
	} else {
		if s.isReady([]string{"AccountFound", "ValidRules", "AdoptedByAccount"}) {
			readyCondition.Status = metav1.ConditionTrue
			readyCondition.Reason = "Ready"
		} else {
			readyCondition.Status = metav1.ConditionFalse
			readyCondition.Reason = "NotReady"
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
