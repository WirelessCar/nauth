package ports

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SecretOwner struct {
	Owner metav1.Object
}

type SecretStorer interface {
	// TODO: Keys created should be immutable
	ApplySecret(ctx context.Context, owner *SecretOwner, meta metav1.ObjectMeta, valueMap map[string]string) error
	GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
	DeleteSecret(ctx context.Context, namespace string, name string) error
	DeleteSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) error
	LabelSecret(ctx context.Context, namespace, name string, labels map[string]string) error
}

type NATSClient interface {
	EnsureConnected(namespace string) error
	Disconnect()
	LookupAccountJWT(string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}

type AccountGetter interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error)
}
