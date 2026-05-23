package outbound

import (
	"context"

	"github.com/WirelessCar/nauth/internal/domain"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigMapReader interface {
	// Get returns the ConfigMap data as a map of key to string value.
	// Keys from both Data and BinaryData are included.
	// Returns domain.ErrBadRequest if the configMapRef is invalid.
	// Returns domain.ErrConfigMapNotFound if the ConfigMap does not exist.
	Get(ctx context.Context, configMapRef domain.NamespacedName) (map[string]string, error)
}

type SecretReader interface {
	Get(ctx context.Context, secretRef domain.NamespacedName) (map[string]string, bool, error)
	GetByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) (*v1.SecretList, error)
}

type SecretClient interface {
	SecretReader
	Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, valueMap map[string]string) error
	Delete(ctx context.Context, secretRef domain.NamespacedName) error
	DeleteByLabels(ctx context.Context, namespace domain.Namespace, labels map[string]string) error
	Label(ctx context.Context, secretRef domain.NamespacedName, labels map[string]string) error
	// IsOwnedBy reports whether the Secret identified by secretRef has a controller owner reference
	// pointing to expectedOwner. Returns false (not an error) when the Secret has no owner or a different owner.
	IsOwnedBy(ctx context.Context, secretRef domain.NamespacedName, expectedOwner metav1.Object) (bool, error)
}
