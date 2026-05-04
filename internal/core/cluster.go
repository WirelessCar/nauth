package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
)

type clusterTargetResolver interface {
	GetClusterTarget(ctx context.Context, accountClusterRef *nauth.ClusterRef) (*nauth.ClusterTarget, error)
}

type ClusterManager struct {
	clusterReader outbound.ClusterReader
	natsSysClient outbound.NatsSysClient
	config        *Config
}

func NewClusterManager(
	clusterReader outbound.ClusterReader,
	natsSysClient outbound.NatsSysClient,
	config *Config,
) (*ClusterManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	impl := &ClusterManager{
		clusterReader: clusterReader,
		natsSysClient: natsSysClient,
		config:        config,
	}
	if err := impl.validate(); err != nil {
		return nil, fmt.Errorf("invalid ClusterManager: %w", err)
	}
	return impl, nil
}

func (r *ClusterManager) validate() error {
	if r.clusterReader == nil {
		return fmt.Errorf("clusterReader is required")
	}
	if r.natsSysClient == nil {
		return fmt.Errorf("natsSysClient is required")
	}
	if r.config == nil {
		return fmt.Errorf("config is required")
	}
	return nil
}

func (r *ClusterManager) Validate(ctx context.Context, target nauth.ClusterTarget) error {
	if err := target.Validate(); err != nil {
		return fmt.Errorf("invalid cluster target: %w", err)
	}

	sysConn, err := r.natsSysClient.Connect(target.NatsURL, target.SystemAdminCreds)
	if err != nil {
		return fmt.Errorf("connect to NATS cluster using System Account User Credentials: %w", err)
	}

	defer sysConn.Disconnect()
	if err := sysConn.VerifySystemAccountAccess(); err != nil {
		return fmt.Errorf("verify NATS System Account access: %w", err)
	}
	return nil
}

func (r *ClusterManager) GetClusterTarget(ctx context.Context, accountClusterRef *nauth.ClusterRef) (*nauth.ClusterTarget, error) {
	opClusterRef, opClusterRequired := r.opClusterConfig()
	clusterRef, err := getEffectiveClusterRef(accountClusterRef, opClusterRef, opClusterRequired)
	if err != nil {
		return nil, err
	}
	result, err := r.clusterReader.GetTarget(ctx, *clusterRef)

	if err != nil {
		return nil, fmt.Errorf("resolve cluster target %q: %w", *clusterRef, err)
	}
	if err = result.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cluster target: %w", err)
	}
	return result, nil
}

func (r *ClusterManager) opClusterConfig() (clusterRef *nauth.ClusterRef, required bool) {
	if r.config == nil {
		return nil, false
	}
	cluster := r.config.OperatorNatsCluster
	if cluster == nil {
		return nil, false
	}
	clusterRef = &cluster.ClusterRef
	required = !cluster.Optional
	return
}

func getEffectiveClusterRef(accClusterRef *nauth.ClusterRef, opClusterRef *nauth.ClusterRef, opClusterRequired bool) (*nauth.ClusterRef, error) {
	if accClusterRef != nil {
		if err := accClusterRef.Validate(); err != nil {
			return nil, fmt.Errorf("invalid account cluster ref: %w", err)
		}
		if opClusterRef != nil && opClusterRequired {
			if err := opClusterRef.Validate(); err != nil {
				return nil, fmt.Errorf("invalid operator cluster ref: %w", err)
			}
			if *opClusterRef != *accClusterRef {
				return nil, fmt.Errorf("account cluster reference %s does not match required operator cluster %s", *accClusterRef, *opClusterRef)
			}
		}
		return accClusterRef, nil
	}
	if opClusterRef != nil {
		if err := opClusterRef.Validate(); err != nil {
			return nil, fmt.Errorf("invalid operator cluster ref: %w", err)
		}
		return opClusterRef, nil
	}
	return nil, fmt.Errorf("no cluster reference provided and no operator cluster configured")
}

var _ clusterTargetResolver = (*ClusterManager)(nil)
var _ inbound.ClusterManager = (*ClusterManager)(nil)
