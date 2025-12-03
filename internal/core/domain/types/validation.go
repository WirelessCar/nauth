package types

import (
	"fmt"
	"regexp"

	k8s "k8s.io/apimachinery/pkg/types"
)

func ValidateString(val string, name string, req bool, regex *regexp.Regexp) error {
	if req && val == "" {
		return fmt.Errorf("%s required", name)
	}
	if val != "" && !regex.MatchString(val) {
		return fmt.Errorf("%s malformed", name)
	}
	return nil
}

func ValidateNamespacedName(namespacedName k8s.NamespacedName) error {
	if namespacedName.Namespace == "" {
		return fmt.Errorf("namespace required")
	}
	if namespacedName.Name == "" {
		return fmt.Errorf("name required")
	}
	return nil
}
