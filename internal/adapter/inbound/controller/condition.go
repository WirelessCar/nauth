package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newCondition(condType string, status metav1.ConditionStatus, reason string, msg string) metav1.Condition {
	return metav1.Condition{
		Type:    condType,
		Status:  status,
		Reason:  reason,
		Message: msg,
	}
}

func conditionsReady(conditions []metav1.Condition, conditionType []string) bool {
	for _, ct := range conditionType {
		c := meta.FindStatusCondition(conditions, ct)
		ready := c != nil && c.Status == metav1.ConditionTrue
		if !ready {
			return false
		}
	}

	return true
}
