package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterClient struct {
	reader client.Reader
}

func NewClusterClient(reader client.Reader) *ClusterClient {
	return &ClusterClient{
		reader: reader,
	}
}

func (c *ClusterClient) GetNatsCluster(ctx context.Context, namespace, name string) (*v1alpha1.NatsCluster, error) {
	r := &v1alpha1.NatsCluster{}
	err := c.reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, r)
	if err != nil {
		return nil, fmt.Errorf("failed getting NatsCluster %s/%s: %w", namespace, name, err)
	}
	return r, nil
}
