package nauth

import (
	"fmt"

	"github.com/WirelessCar/nauth/internal/domain"
)

type ClusterTarget struct {
	NatsURL            string
	SystemAdminCreds   domain.NatsUserCreds
	OperatorSigningKey domain.NatsOperatorSigningKey
}

func NewClusterTarget(natsURL string, systemAdminCreds domain.NatsUserCreds, operatorSigningKey domain.NatsOperatorSigningKey) (*ClusterTarget, error) {
	target := &ClusterTarget{
		NatsURL:            natsURL,
		SystemAdminCreds:   systemAdminCreds,
		OperatorSigningKey: operatorSigningKey,
	}
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cluster target: %w", err)
	}
	return target, nil
}

func (c *ClusterTarget) Validate() error {
	if c.NatsURL == "" {
		return fmt.Errorf("URL is required")
	}
	if err := c.SystemAdminCreds.Validate(); err != nil {
		return fmt.Errorf("invalid system admin credentials: %w", err)
	}
	if c.OperatorSigningKey == nil {
		return fmt.Errorf("operator signing key is required")
	}
	return nil
}

type ClusterRefType int64

const (
	ClusterRefTypeUnknown ClusterRefType = iota
	ClusterRefTypeNamespacedName
)

type ClusterRef string

func NewClusterRef(value string) (ClusterRef, error) {
	result := ClusterRef(value)
	if err := result.Validate(); err != nil {
		return "", err
	}
	return result, nil
}

func (r ClusterRef) GetType() ClusterRefType {
	if _, err := r.AsNamespacedName(); err == nil {
		return ClusterRefTypeNamespacedName
	}
	return ClusterRefTypeUnknown
}

func (r ClusterRef) Validate() error {
	// For now, we only support NamespacedName format, so validation is just checking if it can be parsed as such.
	if _, err := r.AsNamespacedName(); err != nil {
		return err
	}
	return nil
}

func (r ClusterRef) AsNamespacedName() (*domain.NamespacedName, error) {
	result, err := domain.ParseNamespacedName(string(r))
	if err != nil {
		return nil, fmt.Errorf("invalid cluster ref %q (expected NamespacedName): %w", r, err)
	}
	return &result, nil
}
