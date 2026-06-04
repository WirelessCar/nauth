package core

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
)

type userClaimsBuilder struct {
	claim *jwt.UserClaims
}

func newUserClaimsBuilder(userPublicKey string) *userClaimsBuilder {
	return &userClaimsBuilder{
		claim: jwt.NewUserClaims(userPublicKey),
	}
}

func (b *userClaimsBuilder) displayName(name string) *userClaimsBuilder {
	b.claim.Name = name
	return b
}

func (b *userClaimsBuilder) permissions(permissions *nauth.UserPermissions) *userClaimsBuilder {
	if permissions != nil {
		b.claim.Pub = jwt.Permission{
			Allow: permissions.Pub.Allow,
			Deny:  permissions.Pub.Deny,
		}
		b.claim.Sub = jwt.Permission{
			Allow: permissions.Sub.Allow,
			Deny:  permissions.Sub.Deny,
		}
		if permissions.Resp != nil {
			b.claim.Resp = &jwt.ResponsePermission{
				MaxMsgs: permissions.Resp.MaxMsgs,
				Expires: permissions.Resp.Expires,
			}
		}
	}
	return b
}

func (b *userClaimsBuilder) userLimits(limits *nauth.UserLimits) *userClaimsBuilder {
	if limits != nil {
		for _, src := range limits.Src {
			b.claim.Src = append(b.claim.Src, src)
		}
		for _, times := range limits.Times {
			b.claim.Times = append(b.claim.Times, jwt.TimeRange{
				Start: times.Start,
				End:   times.End,
			})
		}
		b.claim.Locale = limits.Locale
	}
	return b
}

func (b *userClaimsBuilder) natsLimits(limits *nauth.NatsLimits) *userClaimsBuilder {
	if limits != nil {
		if limits.Subs != nil {
			b.claim.Subs = *limits.Subs
		}
		if limits.Data != nil {
			b.claim.Data = *limits.Data
		}
		if limits.Payload != nil {
			b.claim.NatsLimits.Payload = *limits.Payload
		}
	}
	return b
}

func (b *userClaimsBuilder) issuerAccountID(issuerAccountID string) *userClaimsBuilder {
	b.claim.IssuerAccount = issuerAccountID
	return b
}

func (b *userClaimsBuilder) build() *jwt.UserClaims {
	return b.claim
}

func toNAuthUserClaims(claims *jwt.UserClaims) v1alpha1.UserClaims {
	result := v1alpha1.UserClaims{}

	if claims == nil {
		return result
	}

	result.DisplayName = claims.Name

	// Permissions
	{
		result.Permissions = &v1alpha1.Permissions{}
		populated := false
		if !claims.Pub.Empty() {
			result.Permissions.Pub.Allow = v1alpha1.StringList(claims.Pub.Allow)
			result.Permissions.Pub.Deny = v1alpha1.StringList(claims.Pub.Deny)
			populated = true
		}
		if !claims.Sub.Empty() {
			result.Permissions.Sub.Allow = v1alpha1.StringList(claims.Sub.Allow)
			result.Permissions.Sub.Deny = v1alpha1.StringList(claims.Sub.Deny)
			populated = true
		}
		if claims.Resp != nil {
			result.Permissions.Resp = &v1alpha1.ResponsePermission{
				MaxMsgs: claims.Resp.MaxMsgs,
				Expires: claims.Resp.Expires,
			}
			populated = true
		}
		if !populated {
			result.Permissions = nil
		}
	}

	// User Limits
	{
		result.UserLimits = &v1alpha1.UserLimits{}
		populated := false
		if len(claims.Src) > 0 {
			for _, src := range claims.Src {
				result.UserLimits.Src = append(result.UserLimits.Src, src)
				populated = true
			}
		}
		if len(claims.Times) > 0 {
			for _, times := range claims.Times {
				time := v1alpha1.TimeRange{
					Start: times.Start,
					End:   times.End,
				}
				result.UserLimits.Times = append(result.UserLimits.Times, time)
				populated = true
			}
		}
		if claims.Locale != "" {
			result.UserLimits.Locale = claims.Locale
			populated = true
		}
		if !populated {
			result.UserLimits = nil
		}
	}

	// NATS Limits
	{
		result.NatsLimits = &v1alpha1.NatsLimits{}
		populated := false
		if claims.Subs != jwt.NoLimit {
			result.NatsLimits.Subs = &claims.Subs
			populated = true
		}
		if claims.Data != jwt.NoLimit {
			result.NatsLimits.Data = &claims.Data
			populated = true
		}
		if claims.NatsLimits.Payload != jwt.NoLimit {
			result.NatsLimits.Payload = &claims.NatsLimits.Payload
			populated = true
		}
		if !populated {
			result.NatsLimits = nil
		}
	}

	return result
}
