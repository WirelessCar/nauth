package main

import (
	"testing"

	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func Test_createUserCreds_ShouldReturnFormattedUserCreds_WhenAccountSeedIsValid(t *testing.T) {
	accountKeyPair, err := nkeys.CreateAccount()
	require.NoError(t, err)

	accountID, err := accountKeyPair.PublicKey()
	require.NoError(t, err)

	accountSeed, err := accountKeyPair.Seed()
	require.NoError(t, err)

	creds, err := createUserCreds(accountID, string(accountSeed))
	require.NoError(t, err)
	require.NotEmpty(t, creds.jwt)
	require.NotEmpty(t, creds.seed)
	require.Contains(t, string(creds.formatted), "BEGIN NATS USER JWT")
	require.Contains(t, string(creds.formatted), "BEGIN USER NKEY SEED")
}
