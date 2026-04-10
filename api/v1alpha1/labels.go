package v1alpha1

import (
	"fmt"
)

type Label string

const (
	LabelManagementPolicy Label = "nauth.io/management-policy"
)

type ManagementPolicy string

const (
	ManagementPolicyDefault ManagementPolicy = "default"
	ManagementPolicyObserve ManagementPolicy = "observe"
)

func GetManagementPolicy(labels map[string]string) (ManagementPolicy, error) {
	result := labels[string(LabelManagementPolicy)]
	switch result {
	case string(ManagementPolicyObserve):
		return ManagementPolicyObserve, nil
	case string(ManagementPolicyDefault):
	case "":
		return ManagementPolicyDefault, nil
	}
	return "", fmt.Errorf("unsupported Management Policy: %s", result)
}
