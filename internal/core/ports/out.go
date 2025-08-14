package ports

import (
	"context"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SecretOwner struct {
	Owner metav1.Object
}

type SecretStorer interface {
	// TODO: Keys created should be immutable
	ApplySecret(ctx context.Context, owner *SecretOwner, namespace string, name string, valueMap map[string]string) error
	GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error)
	DeleteSecret(ctx context.Context, namespace string, name string) error
}

type NATSClient interface {
	Connect(namespace string, secretName string) error
	EnsureConnected(namespace string, secretName string) error
	Disconnect()
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}

type AccountGetter interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error)
}
