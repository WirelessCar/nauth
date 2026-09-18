package domain

import (
	"testing"

	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/require"
)

func TestHashNatsAccountJWTClaims_ShouldGenerateDeterministicHash(t *testing.T) {
	operatorSigningKey := testutil.CreateNatsTestOperatorKey()
	account := testutil.CreateNatsTestAccount()
	encode := func(claims *jwt.AccountClaims, signingKey testutil.NatsTestOperatorKey) string {
		signedJWT, err := claims.Encode(signingKey.Key)
		require.NoError(t, err)
		return signedJWT
	}

	claims := jwt.NewAccountClaims(account.AccountID())
	claims.Name = "Test Account"
	claims.SigningKeys.Add(account.Sign.PublicKey)
	claims.IssuedAt = 1
	claims.ID = "claims-0"
	jwt0 := encode(claims, operatorSigningKey)

	equivalentClaims := jwt.NewAccountClaims(account.AccountID())
	equivalentClaims.Name = "Test Account"
	equivalentClaims.SigningKeys.Add(account.Sign.PublicKey)
	equivalentClaims.IssuedAt = 2
	equivalentClaims.ID = "claims-1"
	jwt1 := encode(equivalentClaims, operatorSigningKey)

	hash := func(accountJWT string) string {
		claimsHash, err := HashNatsAccountJWTClaims(accountJWT)
		require.NoError(t, err)
		return claimsHash
	}

	claimsHash := hash(jwt0)

	require.Equal(t, claimsHash, hash(jwt0), "expected hash to be deterministic for same JWT")
	require.Equal(t, claimsHash, hash(jwt1), "expected hash to be deterministic for same claims and signing key")

	otherOperatorSigningKey := testutil.CreateNatsTestOperatorKey()
	require.NotEqual(t, claimsHash, hash(encode(claims, otherOperatorSigningKey)), "expected hash to change when signing key changes")

	changedClaims := *claims
	changedClaims.Description = "Claims V2"
	require.NotEqual(t, claimsHash, hash(encode(&changedClaims, operatorSigningKey)), "expected hash to change when claims content changes")
}

func TestNatsAccountState_MatchesClaimsHash_ShouldOnlyCompareHashes(t *testing.T) {
	state := NatsAccountState{Status: NatsAccountStateComplete, ClaimsHash: "claims-hash"}

	require.True(t, state.MatchesClaimsHash("claims-hash"))
	require.False(t, state.MatchesClaimsHash("different-claims-hash"))
	require.True(t, (NatsAccountState{
		Status:     NatsAccountStateUnknown,
		ClaimsHash: "claims-hash",
	}).MatchesClaimsHash("claims-hash"))
	require.False(t, state.MatchesClaimsHash(""))
}
