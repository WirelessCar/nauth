package core

import (
	"context"
	"errors"
	"fmt"

	k8s "github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ErrSigningKeyConflict = errors.New("signing key secret conflict")
	ErrInvalidSeed        = errors.New("invalid nkeys signing key seed")
	ErrSecretNotFound     = errors.New("signing key secret not found")
)

type AccountSigningKeyManager struct {
	secretClient outbound.SecretClient
}

func NewAccountSigningKeyManager(secretClient outbound.SecretClient) *AccountSigningKeyManager {
	return &AccountSigningKeyManager{secretClient: secretClient}
}

func (m *AccountSigningKeyManager) CreateOrUpdate(ctx context.Context, req nauth.AccountSigningKeyRequest) (*nauth.AccountSigningKeyResult, error) {
	data, found, err := m.secretClient.Get(ctx, req.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %q: %w", req.SecretRef, err)
	}

	if found {
		owned, err := m.secretClient.IsOwnedBy(ctx, req.SecretRef, req.Owner)
		if err != nil {
			return nil, fmt.Errorf("failed to check secret ownership for %q: %w", req.SecretRef, err)
		}
		if !owned {
			return nil, fmt.Errorf("%w: secret %q exists but is not owned by this AccountSigningKey. "+
				"To use an existing Secret, use observe mode (label nauth.io/management-policy=observe)",
				ErrSigningKeyConflict, req.SecretRef.Name)
		}
		pubKey, err := signingKeyFromData(data, req.SecretRef.Name)
		if err != nil {
			return nil, err
		}
		return &nauth.AccountSigningKeyResult{PublicKey: pubKey, SecretName: req.SecretRef.Name}, nil
	}

	kp, err := nkeys.CreateAccount()
	if err != nil {
		return nil, fmt.Errorf("failed to generate account signing key: %w", err)
	}
	seed, err := kp.Seed()
	if err != nil {
		return nil, fmt.Errorf("failed to read generated seed: %w", err)
	}
	pubKey, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to derive public key: %w", err)
	}

	secretMeta := metav1.ObjectMeta{
		Name:      req.SecretRef.Name,
		Namespace: req.SecretRef.Namespace,
		Labels: map[string]string{
			k8s.LabelManaged: k8s.LabelManagedValue,
		},
	}
	if err := m.secretClient.Apply(ctx, req.Owner, secretMeta, map[string]string{
		k8s.DefaultSecretKeyName: string(seed),
	}); err != nil {
		return nil, fmt.Errorf("failed to store signing key secret %q: %w", req.SecretRef.Name, err)
	}

	return &nauth.AccountSigningKeyResult{PublicKey: pubKey, SecretName: req.SecretRef.Name}, nil
}

func (m *AccountSigningKeyManager) Import(ctx context.Context, secretRef domain.NamespacedName) (*nauth.AccountSigningKeyResult, error) {
	data, found, err := m.secretClient.Get(ctx, secretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %q: %w", secretRef, err)
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, secretRef.Name)
	}
	pubKey, err := signingKeyFromData(data, secretRef.Name)
	if err != nil {
		return nil, err
	}
	return &nauth.AccountSigningKeyResult{PublicKey: pubKey, SecretName: secretRef.Name}, nil
}

func signingKeyFromData(data map[string]string, secretName string) (string, error) {
	seedStr, ok := data[k8s.DefaultSecretKeyName]
	if !ok {
		return "", fmt.Errorf("%w: secret %q is missing key %q", ErrInvalidSeed, secretName, k8s.DefaultSecretKeyName)
	}
	return signingKeyFromSeed([]byte(seedStr))
}

func signingKeyFromSeed(seed []byte) (string, error) {
	kp, err := nkeys.FromSeed(seed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSeed, err)
	}
	pubKey, err := kp.PublicKey()
	if err != nil {
		return "", fmt.Errorf("failed to derive public key: %w", err)
	}
	if !nkeys.IsValidPublicAccountKey(pubKey) {
		return "", fmt.Errorf("%w: expected an account public key (A-prefixed), got %q", ErrInvalidSeed, pubKey)
	}
	return pubKey, nil
}

var _ inbound.AccountSigningKeyManager = (*AccountSigningKeyManager)(nil)
