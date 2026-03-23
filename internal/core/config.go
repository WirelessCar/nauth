package core

import (
	"fmt"
	"strings"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	k8sval "k8s.io/apimachinery/pkg/api/validation"
)

type Config struct {
	OperatorNatsCluster *OperatorNatsCluster
	// OperatorNamespace is the Kubernetes namespace where the operator is deployed.
	OperatorNamespace domain.Namespace
}

func NewConfig(operatorNatsCluster *OperatorNatsCluster, operatorNamespace domain.Namespace) (*Config, error) {
	config := &Config{
		OperatorNatsCluster: operatorNatsCluster,
		OperatorNamespace:   operatorNamespace,
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Config) validate() error {
	if c.OperatorNatsCluster != nil {
		if err := c.OperatorNatsCluster.validate(); err != nil {
			return fmt.Errorf("invalid operator NATS cluster: %w", err)
		}
	}
	if c.OperatorNamespace != "" {
		if err := c.OperatorNamespace.Validate(); err != nil {
			return fmt.Errorf("invalid operator namespace %q: %s", c.OperatorNamespace, err)
		}
	}

	return nil
}

type OperatorNatsCluster struct {
	ClusterRef v1alpha1.NatsClusterRef
	// Optional controls account-level overrides when ClusterRef is configured.
	// false (default) means account-level cluster refs must not deviate.
	Optional bool
}

func NewOperatorNatsCluster(clusterRef v1alpha1.NatsClusterRef, optional bool) (*OperatorNatsCluster, error) {
	cluster := &OperatorNatsCluster{
		ClusterRef: v1alpha1.NatsClusterRef{
			Namespace: clusterRef.Namespace,
			Name:      clusterRef.Name,
		},
		Optional: optional,
	}
	if err := cluster.validate(); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (c *OperatorNatsCluster) validate() error {
	namespace := c.ClusterRef.Namespace
	namespaceTrimmed := strings.TrimSpace(namespace)
	name := c.ClusterRef.Name
	nameTrimmed := strings.TrimSpace(name)

	if namespaceTrimmed == "" || nameTrimmed == "" {
		return fmt.Errorf("both namespace and name must be provided for operator NATS cluster reference")
	}
	if namespace != namespaceTrimmed || name != nameTrimmed {
		return fmt.Errorf("namespace and name in operator NATS cluster reference must not have leading or trailing whitespace")
	}
	if errs := k8sval.ValidateNamespaceName(namespace, false); len(errs) > 0 {
		return fmt.Errorf("invalid namespace %q in operator NATS cluster reference: %s", namespace, strings.Join(errs, ", "))
	}
	if errs := k8sval.NameIsDNSSubdomain(name, false); len(errs) > 0 {
		return fmt.Errorf("invalid name %q in operator NATS cluster reference: %s", name, strings.Join(errs, ", "))
	}

	c.ClusterRef.Namespace = namespace
	c.ClusterRef.Name = name
	return nil
}
