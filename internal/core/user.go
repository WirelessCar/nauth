package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SignedUserJWT struct {
	UserJWT   string
	AccountID string
	SignedBy  string
}

type UserJWTSigner interface {
	SignUserJWT(ctx context.Context, accountRef domain.NamespacedName, claims *jwt.UserClaims) (*SignedUserJWT, error)
}

type UserManager struct {
	userJWTSigner UserJWTSigner
	secretClient  outbound.SecretClient
}

func NewUserManager(userJWTSigner UserJWTSigner, secretClient outbound.SecretClient) *UserManager {
	return &UserManager{
		userJWTSigner: userJWTSigner,
		secretClient:  secretClient,
	}
}

func (u *UserManager) CreateOrUpdate(ctx context.Context, request nauth.UserRequest) (*nauth.UserResult, error) {
	if err := request.AccountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", request.AccountRef, err)
	}

	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("failed to create user key pair: %w", err)
	}
	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get user public key: %w", err)
	}
	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return nil, fmt.Errorf("failed to get user seed: %w", err)
	}

	displayName := request.DisplayName
	if displayName == "" {
		displayName = fmt.Sprintf("%s/%s", request.UserRef.Namespace, request.UserRef.Name)
	}

	natsClaims := newUserClaimsBuilder(userPublicKey).
		displayName(displayName).
		permissions(request.Permissions).
		userLimits(request.Limits).
		natsLimits(request.NatsLimits).
		issuerAccountID(string(request.AccountID)).
		build()
	signedUserJWT, err := u.userJWTSigner.SignUserJWT(ctx, request.AccountRef, natsClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign user jwt for %s/%s: %w", request.UserRef.Namespace, request.UserRef.Name, err)
	}

	userCreds, err := jwt.FormatUserConfig(signedUserJWT.UserJWT, userSeed)
	if err != nil {
		return nil, fmt.Errorf("failed to format user credentials: %w", err)
	}

	secretName := fmt.Sprintf("%s-nats-user-creds", request.UserRef.Name)
	secretMeta := metav1.ObjectMeta{
		Name:      secretName,
		Namespace: request.UserRef.Namespace,
		Labels: map[string]string{
			k8s.LabelSecretType: k8s.SecretTypeUserCredentials,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	secretValue := map[string]string{
		k8s.UserCredentialSecretKeyName: string(userCreds),
	}
	if err := u.secretClient.Apply(ctx, request.Owner, secretMeta, secretValue); err != nil {
		return nil, err
	}

	return &nauth.UserResult{
		UserPublicKey: userPublicKey,
		AccountID:     signedUserJWT.AccountID,
		SignedBy:      signedUserJWT.SignedBy,
	}, nil
}

var _ inbound.UserManager = (*UserManager)(nil)
