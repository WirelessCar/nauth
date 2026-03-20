package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NatsClusterClient struct {
	reader client.Reader
}

func NewNatsClusterClient(reader client.Reader) *NatsClusterClient {
	return &NatsClusterClient{
		reader: reader,
	}
}

func (c *NatsClusterClient) Get(ctx context.Context, clusterRef domain.NamespacedName) (*v1alpha1.NatsCluster, error) {
	if err := clusterRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NATS cluster reference %q: %w", clusterRef, err)
	}

	r := &v1alpha1.NatsCluster{}
	err := c.reader.Get(ctx, client.ObjectKey{Namespace: clusterRef.Namespace, Name: clusterRef.Name}, r)
	if err != nil {
		return nil, fmt.Errorf("failed getting NatsCluster %s/%s: %w", clusterRef.Namespace, clusterRef.Name, err)
	}
	return r, nil
}

// Compile-time assertion that implementation satisfies the ports interface
var _ outbound.NatsClusterReader = (*NatsClusterClient)(nil)
