package nauth

import (
	"github.com/WirelessCar/nauth/internal/domain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountSigningKeyRequest carries all inputs needed to reconcile a managed AccountSigningKey.
type AccountSigningKeyRequest struct {
	// SecretRef is the resolved name and namespace of the Secret holding the signing-key seed.
	SecretRef domain.NamespacedName
	// Owner is the AccountSigningKey resource; created Secrets are owned by it for GC.
	Owner metav1.Object
}

// AccountSigningKeyResult is returned by AccountSigningKeyManager on success.
type AccountSigningKeyResult struct {
	// PublicKey is the resolved NATS account public key (A-prefixed nkey).
	PublicKey string
	// SecretName is the resolved name of the Secret holding the seed.
	SecretName string
}
