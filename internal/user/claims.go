package user

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/nats-io/jwt/v2"
)

type claimsBuilder struct {
	claim *jwt.UserClaims
}

func newClaimsBuilder(
	displayName string,
	spec v1alpha1.UserSpec,
	userPublicKey string,
	issuerAccountId string,
) *claimsBuilder {
	claim := jwt.NewUserClaims(userPublicKey)
	claim.Name = displayName

	// Permissions
	if spec.Permissions != nil {
		claim.Pub = jwt.Permission{
			Allow: jwt.StringList(spec.Permissions.Pub.Allow),
			Deny:  jwt.StringList(spec.Permissions.Pub.Deny),
		}
		claim.Sub = jwt.Permission{
			Allow: jwt.StringList(spec.Permissions.Sub.Allow),
			Deny:  jwt.StringList(spec.Permissions.Sub.Deny),
		}
		if spec.Permissions.Resp != nil {
			claim.Resp = &jwt.ResponsePermission{
				MaxMsgs: spec.Permissions.Resp.MaxMsgs,
				Expires: spec.Permissions.Resp.Expires,
			}
		}
	}

	// User Limits
	if spec.UserLimits != nil {
		for _, src := range spec.UserLimits.Src {
			claim.Src = append(claim.Src, src)
		}
		for _, times := range spec.UserLimits.Times {
			time := jwt.TimeRange{
				Start: times.Start,
				End:   times.End,
			}
			claim.Times = append(claim.Times, time)
		}
		claim.Locale = spec.UserLimits.Locale
	}

	// NATS Limits
	if spec.NatsLimits != nil {
		if spec.NatsLimits.Subs != nil {
			claim.Subs = *spec.NatsLimits.Subs
		}
		if spec.NatsLimits.Data != nil {
			claim.Data = *spec.NatsLimits.Data
		}
		if spec.NatsLimits.Payload != nil {
			claim.NatsLimits.Payload = *spec.NatsLimits.Payload
		}
	}

	claim.IssuerAccount = issuerAccountId

	return &claimsBuilder{
		claim: claim,
	}
}

func (u *claimsBuilder) build() *jwt.UserClaims {
	return u.claim
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
