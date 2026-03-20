package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
	secretClient  ports.SecretClient
}

func NewUserManager(userJWTSigner UserJWTSigner, secretClient ports.SecretClient) *UserManager {
	return &UserManager{
		userJWTSigner: userJWTSigner,
		secretClient:  secretClient,
	}
}

func (u *UserManager) CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error {
	userRef := domain.NewNamespacedName(state.Namespace, state.Name)
	accountRef := domain.NewNamespacedName(state.Namespace, state.Spec.AccountName)
	if err := accountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	existingUserAccountID := state.GetLabels()[k8s.LabelUserAccountID]

	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return fmt.Errorf("failed to create user key pair: %w", err)
	}
	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get user public key: %w", err)
	}
	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return fmt.Errorf("failed to get user seed: %w", err)
	}

	natsClaims := newUserClaimsBuilder(u.getUserDisplayName(state), state.Spec, userPublicKey, existingUserAccountID).
		build()
	signedUserJWT, err := u.userJWTSigner.SignUserJWT(ctx, accountRef, natsClaims)
	if err != nil {
		return fmt.Errorf("failed to sign user jwt for %s: %w", userRef, err)
	}

	userCreds, err := jwt.FormatUserConfig(signedUserJWT.UserJWT, userSeed)
	if err != nil {
		return fmt.Errorf("failed to format user credentials: %w", err)
	}

	secretMeta := metav1.ObjectMeta{
		Name:      state.GetUserSecretName(),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelSecretType: k8s.SecretTypeUserCredentials,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	secretValue := map[string]string{
		k8s.UserCredentialSecretKeyName: string(userCreds),
	}
	err = u.secretClient.Apply(ctx, state, secretMeta, secretValue)
	if err != nil {
		return err
	}

	state.Status.Claims = toNAuthUserClaims(natsClaims)

	if state.Labels == nil {
		state.Labels = make(map[string]string, 3)
	}

	state.GetLabels()[k8s.LabelUserID] = userPublicKey
	state.GetLabels()[k8s.LabelUserAccountID] = signedUserJWT.AccountID
	state.GetLabels()[k8s.LabelUserSignedBy] = signedUserJWT.SignedBy

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (u *UserManager) Delete(ctx context.Context, state *v1alpha1.User) error {
	log := logf.FromContext(ctx)
	log.Info("Delete user", "userName", state.GetName())

	secretRef := domain.NewNamespacedName(state.Namespace, state.GetUserSecretName())
	if err := secretRef.Validate(); err != nil {
		return fmt.Errorf("invalid secret reference %q: %w", secretRef, err)
	}
	err := u.secretClient.Delete(ctx, secretRef)
	if err != nil {
		return fmt.Errorf("failed to delete user secret %s: %w", secretRef, err)
	}

	return nil
}

func (u *UserManager) getUserDisplayName(user *v1alpha1.User) string {
	if user.Spec.DisplayName != "" {
		return user.Spec.DisplayName
	}
	return fmt.Sprintf("%s/%s", user.GetNamespace(), user.GetName())
}

var _ inbound.UserManager = (*UserManager)(nil)
