package domain

import (
	"fmt"
	"strings"

	k8sval "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/types"
)

type Namespace string

func (n Namespace) WithName(name string) NamespacedName {
	return NamespacedName{
		Namespace: string(n),
		Name:      name,
	}
}

func (n Namespace) Validate() error {
	if strings.TrimSpace(string(n)) == "" {
		return fmt.Errorf("value required")
	}
	if errs := k8sval.ValidateNamespaceName(string(n), false); len(errs) > 0 {
		return fmt.Errorf("value invalid %q: %s", n, strings.Join(errs, ", "))
	}
	return nil
}

type NamespacedName types.NamespacedName

func NewNamespacedName(namespace, name string) NamespacedName {
	r := NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	return r
}

func ParseNamespacedName(value string) (NamespacedName, error) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return NamespacedName{}, fmt.Errorf("invalid Namespaced Name format %q: expected namespace/name", value)
	}
	result := NewNamespacedName(parts[0], parts[1])
	if err := result.Validate(); err != nil {
		return NamespacedName{}, fmt.Errorf("invalid Namespaced Name %q: %w", value, err)
	}
	return result, nil
}

func (n NamespacedName) GetNamespace() Namespace {
	return Namespace(n.Namespace)
}

func (n NamespacedName) Equals(other NamespacedName) bool {
	return n.Namespace == other.Namespace && n.Name == other.Name
}

func (n NamespacedName) Validate() error {
	if n.Namespace == "" {
		return fmt.Errorf("namespace required")
	}
	if err := Namespace(n.Namespace).Validate(); err != nil {
		return fmt.Errorf("namespace invalid %q: %w", n.Namespace, err)
	}
	if n.Name == "" {
		return fmt.Errorf("name required")
	}
	if errs := k8sval.NameIsDNSSubdomain(n.Name, false); len(errs) > 0 {
		return fmt.Errorf("name invalid %q: %s", n.Name, strings.Join(errs, ", "))
	}
	return nil
}

func (n NamespacedName) String() string {
	return types.NamespacedName(n).String()
}
