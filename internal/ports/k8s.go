package ports

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type NamespacedName types.NamespacedName

func (n NamespacedName) Equals(other NamespacedName) bool {
	return n.Namespace == other.Namespace && n.Name == other.Name
}

func (n NamespacedName) Validate() error {
	if n.Name == "" {
		return fmt.Errorf("name is required")
	}
	if n.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}

func (n NamespacedName) String() string {
	return fmt.Sprintf("%s/%s", n.Namespace, n.Name)
}

type ConfigMapReader interface {
	Get(ctx context.Context, namespace string, name string) (map[string]string, error)
}

type SecretReader interface {
	Get(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
}

type SecretClient interface {
	SecretReader
	Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error
	Delete(ctx context.Context, namespace string, name string) error
	DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error
	Label(ctx context.Context, namespace, name string, labels map[string]string) error
}

type AccountReader interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error)
}

type NatsClusterReader interface {
	Get(ctx context.Context, clusterRef NamespacedName) (*v1alpha1.NatsCluster, error)
}
