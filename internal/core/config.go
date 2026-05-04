package core

import (
	"fmt"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
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
	ClusterRef nauth.ClusterRef
	// Optional controls account-level overrides when ClusterRef is configured.
	// false (default) means account-level cluster refs must not deviate.
	Optional bool
}

func NewOperatorNatsCluster(clusterRef nauth.ClusterRef, optional bool) (*OperatorNatsCluster, error) {

	cluster := &OperatorNatsCluster{
		ClusterRef: clusterRef,
		Optional:   optional,
	}
	if err := cluster.validate(); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (c *OperatorNatsCluster) validate() error {
	return c.ClusterRef.Validate()
}
