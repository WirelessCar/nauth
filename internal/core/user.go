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

type userIdentity struct {
	userID nauth.UserID
	seed   []byte
}

// UserSigningRequest is the parameter to UserJWTSigner.SignUserJWT.
type UserSigningRequest struct {
	AccountRef domain.NamespacedName
	Claims     *jwt.UserClaims
	// Seed is the raw nkeys seed to use for signing. When nil the Account's
	// implicit signing key is used.
	Seed []byte
}

type UserJWTSigner interface {
	SignUserJWT(ctx context.Context, req UserSigningRequest) (*SignedUserJWT, error)
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
	if err := request.UserRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid user reference %q: %w", request.UserRef, err)
	}
	if err := request.AccountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", request.AccountRef, err)
	}

	displayName := request.DisplayName
	if displayName == "" {
		displayName = fmt.Sprintf("%s/%s", request.UserRef.Namespace, request.UserRef.Name)
	}

	var signingSeed []byte
	if request.SigningPrivateKeySecretRef != nil {
		if request.SigningPublicKey == "" {
			return nil, fmt.Errorf("signing public key is required when signing private key secret ref is set")
		}
		secretRef := *request.SigningPrivateKeySecretRef
		secretData, found, err := u.secretClient.Get(ctx, secretRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get signing key secret %q: %w", secretRef, err)
		}
		if !found {
			return nil, fmt.Errorf("signing key secret %q not found", secretRef)
		}
		seedStr, ok := secretData[k8s.DefaultSecretKeyName]
		if !ok {
			return nil, fmt.Errorf("signing key secret %q missing key %q", secretRef, k8s.DefaultSecretKeyName)
		}
		signingSeed = []byte(seedStr)

		// Re-verify the seed still derives to the public key the caller validated as trusted.
		// Closes the TOCTOU window between the controller's trust check and reading the Secret here.
		seedPubKey, err := signingKeyFromSeed(signingSeed)
		if err != nil {
			return nil, fmt.Errorf("signing key secret %q holds an invalid seed: %w", secretRef, err)
		}
		if seedPubKey != request.SigningPublicKey {
			return nil, fmt.Errorf("signing key secret %q derives to %s; caller expected trusted public key %s",
				secretRef, seedPubKey, request.SigningPublicKey)
		}
	}

	identity, err := u.loadOrCreateUserIdentity(ctx, request)
	if err != nil {
		return nil, err
	}

	natsClaims := newUserClaimsBuilder(string(identity.userID)).
		displayName(displayName).
		expires(request.ExpiresAt).
		permissions(request.Permissions).
		userLimits(request.Limits).
		natsLimits(request.NatsLimits).
		issuerAccountID(string(request.AccountID)).
		build()

	signedUserJWT, err := u.userJWTSigner.SignUserJWT(ctx, UserSigningRequest{
		AccountRef: request.AccountRef,
		Claims:     natsClaims,
		Seed:       signingSeed,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign user jwt for %s/%s: %w", request.UserRef.Namespace, request.UserRef.Name, err)
	}

	userCreds, err := jwt.FormatUserConfig(signedUserJWT.UserJWT, identity.seed)
	if err != nil {
		return nil, fmt.Errorf("failed to format user credentials: %w", err)
	}

	secretRef := userCredentialsSecretRef(request.UserRef)
	secretMeta := metav1.ObjectMeta{
		Name:      secretRef.Name,
		Namespace: secretRef.Namespace,
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
		UserID:    identity.userID,
		AccountID: signedUserJWT.AccountID,
		SignedBy:  signedUserJWT.SignedBy,
	}, nil
}

func (u *UserManager) ValidateIdentity(ctx context.Context, userRef domain.NamespacedName, expectedUserID nauth.UserID) error {
	if expectedUserID == "" {
		return fmt.Errorf("expected user ID is required to validate persisted identity")
	}
	if err := userRef.Validate(); err != nil {
		return fmt.Errorf("invalid user reference %q: %w", userRef, err)
	}
	identity, found, err := u.readUserIdentity(ctx, userRef)
	if err != nil {
		return err
	}
	if !found {
		return domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("user credentials secret for %q not found for expected user ID %s", userRef, expectedUserID))
	}
	return validateUserIdentityID(userRef, identity, expectedUserID)
}

func (u *UserManager) loadOrCreateUserIdentity(ctx context.Context, request nauth.UserRequest) (userIdentity, error) {
	identity, found, err := u.readUserIdentity(ctx, request.UserRef)
	if err != nil {
		return userIdentity{}, err
	}
	if found {
		if err := validateUserIdentityID(request.UserRef, identity, request.ExpectedUserID); err != nil {
			return userIdentity{}, err
		}
		return identity, nil
	}
	if request.ExpectedUserID != "" {
		secretRef := userCredentialsSecretRef(request.UserRef)
		return userIdentity{}, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("user credentials secret %q not found for expected user ID %s", secretRef, request.ExpectedUserID))
	}
	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return userIdentity{}, fmt.Errorf("failed to create user key pair: %w", err)
	}
	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return userIdentity{}, fmt.Errorf("failed to get user public key: %w", err)
	}
	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return userIdentity{}, fmt.Errorf("failed to get user seed: %w", err)
	}
	return userIdentity{userID: nauth.UserID(userPublicKey), seed: userSeed}, nil
}

func (u *UserManager) readUserIdentity(ctx context.Context, userRef domain.NamespacedName) (userIdentity, bool, error) {
	secretRef := userCredentialsSecretRef(userRef)
	secretData, found, err := u.secretClient.Get(ctx, secretRef)
	if err != nil {
		return userIdentity{}, false, fmt.Errorf("failed to get user credentials secret %q: %w", secretRef, err)
	}
	if !found {
		return userIdentity{}, false, nil
	}

	userCreds, ok := secretData[k8s.UserCredentialSecretKeyName]
	if !ok {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("user credentials secret %q is missing key %q", secretRef, k8s.UserCredentialSecretKeyName))
	}

	userJWT, err := jwt.ParseDecoratedJWT([]byte(userCreds))
	if err != nil {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("failed to parse user JWT from credentials secret %q: %w", secretRef, err))
	}
	userClaims, err := jwt.DecodeUserClaims(userJWT)
	if err != nil {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("failed to decode user JWT from credentials secret %q: %w", secretRef, err))
	}
	userKeyPair, err := jwt.ParseDecoratedUserNKey([]byte(userCreds))
	if err != nil {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("failed to parse user seed from credentials secret %q: %w", secretRef, err))
	}
	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("failed to get user public key from credentials secret %q: %w", secretRef, err))
	}
	if userClaims.Subject != userPublicKey {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("user credentials secret %q has JWT subject %s, but its seed derives to %s", secretRef, userClaims.Subject, userPublicKey))
	}
	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return userIdentity{}, true, domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("failed to get user seed from credentials secret %q: %w", secretRef, err))
	}
	return userIdentity{userID: nauth.UserID(userPublicKey), seed: userSeed}, true, nil
}

func userCredentialsSecretRef(userRef domain.NamespacedName) domain.NamespacedName {
	return domain.NewNamespacedName(userRef.Namespace, fmt.Sprintf("%s-nats-user-creds", userRef.Name))
}

func validateUserIdentityID(userRef domain.NamespacedName, identity userIdentity, expectedUserID nauth.UserID) error {
	if expectedUserID == "" || identity.userID == expectedUserID {
		return nil
	}
	return domain.ErrUserIdentityNotReady.WithCause(fmt.Errorf("user credentials secret for %q derives to user ID %s, expected %s", userRef, identity.userID, expectedUserID))
}

var _ inbound.UserManager = (*UserManager)(nil)
