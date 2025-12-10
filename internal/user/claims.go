package user

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

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
	u.claim.IssuerAccount = account.Labels[k8s.LabelAccountID]
	return u
}

func (u *userClaimBuilder) encode(accountSigningKeyPair nkeys.KeyPair) (string, error) {
	signedJwt, err := u.claim.Encode(accountSigningKeyPair)
	if err != nil {
		return "", err
	}

	return signedJwt, nil
}
