package service

import (
	"context"
	"fmt"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type UserManager struct {
	accounts     ports.AccountGetter
	secretStorer ports.SecretStorer
}

func (u *UserManager) CreateOrUpdateUser(ctx context.Context, state *v1alpha1.User) error {
	account, err := u.accounts.Get(ctx, state.Spec.AccountName, state.Namespace)
	if err != nil {
		return err
	}

	signingKeyMap, err := u.secretStorer.GetSecret(ctx, account.Namespace, account.GetAccountSignSecretName())
	if err != nil {
		return fmt.Errorf("failed to get signing key secret %s/%s: %w", account.Namespace, account.GetAccountSignSecretName(), err)
	}

	signingKey := signingKeyMap[domain.DefaultSecretKeyName]
	signingKeyPair, _ := nkeys.FromSeed([]byte(signingKey))
	signingKeyPublicKey, _ := signingKeyPair.PublicKey()

	userKeyPair, _ := nkeys.CreateUser()
	userPublicKey, _ := userKeyPair.PublicKey()
	userSeed, _ := userKeyPair.Seed()

	userJwt, err := newUserClaimsBuilder(state, userPublicKey).
		issuerAccount(*account).
		natsLimits().
		permissions().
		userLimits().
		encode(signingKeyPair)
	if err != nil {
		return err
	}

	userCreds, _ := jwt.FormatUserConfig(userJwt, userSeed)

	secretOwner := &ports.SecretOwner{
		Owner: state,
	}
	err = u.secretStorer.ApplySecret(ctx, secretOwner, state.Namespace, state.GetUserSecretName(), map[string]string{domain.UserCredentialSecretKeyName: string(userCreds)})
	if err != nil {
		return err
	}

	state.Status.Claims = v1alpha1.UserClaims{
		AccountName: state.Spec.AccountName,
		NatsLimits:  state.Spec.NatsLimits,
		Permissions: state.Spec.Permissions,
		UserLimits:  state.Spec.UserLimits,
	}

	if state.Labels == nil {
		state.Labels = map[string]string{}
	}

	state.GetLabels()[domain.LabelUserId] = userPublicKey
	state.GetLabels()[domain.LabelUserAccountId] = account.GetLabels()[domain.LabelAccountId]
	state.GetLabels()[domain.LabelUserSignedBy] = signingKeyPublicKey

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (u *UserManager) DeleteUser(ctx context.Context, state *v1alpha1.User) error {
	log := logf.FromContext(ctx)
	log.Info("Delete user", "userName", state.GetName())

	err := u.secretStorer.DeleteSecret(ctx, state.Namespace, state.GetUserSecretName())
	if err != nil {
		return fmt.Errorf("failed to delete user secret %s/%s: %w", state.Namespace, state.GetUserSecretName(), err)
	}

	return nil
}

func NewUserManager(accounts ports.AccountGetter, secretStorer ports.SecretStorer) *UserManager {
	return &UserManager{
		accounts:     accounts,
		secretStorer: secretStorer,
	}
}

type userClaimBuilder struct {
	userState *v1alpha1.User
	claim     *jwt.UserClaims
}

func newUserClaimsBuilder(state *v1alpha1.User, userPublicKey string) *userClaimBuilder {
	claim := jwt.NewUserClaims(userPublicKey)
	userState := state

	return &userClaimBuilder{
		userState: userState,
		claim:     claim,
	}
}

func (u *userClaimBuilder) permissions() *userClaimBuilder {
	if u.userState.Spec.Permissions != nil {
		permissions := jwt.Permissions{}

		permissions.Pub = jwt.Permission{
			Allow: jwt.StringList(u.userState.Spec.Permissions.Pub.Allow),
			Deny:  jwt.StringList(u.userState.Spec.Permissions.Pub.Deny),
		}
		permissions.Sub = jwt.Permission{
			Allow: jwt.StringList(u.userState.Spec.Permissions.Sub.Allow),
			Deny:  jwt.StringList(u.userState.Spec.Permissions.Sub.Deny),
		}
		if u.userState.Spec.Permissions.Resp != nil {
			permissions.Resp = &jwt.ResponsePermission{
				MaxMsgs: u.userState.Spec.Permissions.Resp.MaxMsgs,
				Expires: u.userState.Spec.Permissions.Resp.Expires,
			}
		}
		u.claim.Permissions = permissions
	}

	return u
}

func (u *userClaimBuilder) userLimits() *userClaimBuilder {
	if u.userState.Spec.UserLimits != nil {
		userLimits := jwt.UserLimits{}

		for _, src := range u.userState.Spec.UserLimits.Src {
			userLimits.Src = append(userLimits.Src, src)
		}
		for _, times := range u.userState.Spec.UserLimits.Times {
			time := jwt.TimeRange{
				Start: times.Start,
				End:   times.End,
			}
			userLimits.Times = append(userLimits.Times, time)
		}
		userLimits.Locale = u.userState.Spec.UserLimits.Locale

		u.claim.UserLimits = userLimits
	}

	return u
}

func (u *userClaimBuilder) natsLimits() *userClaimBuilder {
	if u.userState.Spec.NatsLimits != nil {
		natsLimits := jwt.NatsLimits{}

		if u.userState.Spec.NatsLimits.Subs != nil {
			natsLimits.Subs = *u.userState.Spec.NatsLimits.Subs
		} else {
			natsLimits.Subs = -1
		}
		if u.userState.Spec.NatsLimits.Data != nil {
			natsLimits.Data = *u.userState.Spec.NatsLimits.Data
		} else {
			natsLimits.Data = -1
		}
		if u.userState.Spec.NatsLimits.Payload != nil {
			natsLimits.Payload = *u.userState.Spec.NatsLimits.Payload
		} else {
			natsLimits.Payload = -1
		}

		u.claim.NatsLimits = natsLimits
	}

	return u
}

func (u *userClaimBuilder) issuerAccount(account v1alpha1.Account) *userClaimBuilder {
	u.claim.IssuerAccount = account.Labels[domain.LabelAccountId]
	return u
}

func (u *userClaimBuilder) encode(accountSigningKeyPair nkeys.KeyPair) (string, error) {
	signedJwt, err := u.claim.Encode(accountSigningKeyPair)
	if err != nil {
		return "", err
	}

	return signedJwt, nil
}
