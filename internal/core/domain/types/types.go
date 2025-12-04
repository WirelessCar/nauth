package types

import (
	"fmt"
	"regexp"

	k8s "k8s.io/apimachinery/pkg/types"
)

var (
	acPubKeyRegex = regexp.MustCompile("^A[0-9A-Z]{55}$")
	opPubKeyRegex = regexp.MustCompile("^O[0-9A-Z]{55}$")
)

type AcPubKey string

func (k AcPubKey) Validate() error {
	return validateString(string(k), "account public key", acPubKeyRegex)
}

type OpPubKey string

func (k OpPubKey) Validate() error {
	return validateString(string(k), "operator public key", opPubKeyRegex)
}

type NamespacedName k8s.NamespacedName

func (n NamespacedName) Validate() error {
	if n.Namespace == "" {
		return fmt.Errorf("namespace required")
	}
	if n.Name == "" {
		return fmt.Errorf("name required")
	}
	return nil
}

func validateString(val string, name string, regex *regexp.Regexp) error {
	if val == "" {
		return fmt.Errorf("%s required", name)
	}
	if !regex.MatchString(val) {
		return fmt.Errorf("%s malformed", name)
	}
	return nil
}
