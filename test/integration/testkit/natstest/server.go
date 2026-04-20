package natstest

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

type Server struct {
	URL                      string
	OperatorSigningSeed      []byte
	OperatorSigningPublicKey string
	SystemAccountID          string
	SystemCreds              []byte

	server   *natsserver.Server
	sysConn  *nats.Conn
	resolver *natsserver.DirAccResolver
}

func Start(t *testing.T) *Server {
	t.Helper()

	operatorKey, err := nkeys.CreateOperator()
	require.NoError(t, err)
	operatorPublicKey, err := operatorKey.PublicKey()
	require.NoError(t, err)
	operatorClaims := jwt.NewOperatorClaims(operatorPublicKey)
	operatorJWT, err := operatorClaims.Encode(operatorKey)
	require.NoError(t, err)
	decodedOperatorClaims, err := jwt.DecodeOperatorClaims(operatorJWT)
	require.NoError(t, err)

	systemAccountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	systemAccountID, err := systemAccountKey.PublicKey()
	require.NoError(t, err)
	systemAccountClaims := jwt.NewAccountClaims(systemAccountID)
	systemAccountJWT, err := systemAccountClaims.Encode(operatorKey)
	require.NoError(t, err)

	resolver, err := natsserver.NewDirAccResolver(t.TempDir(), 0, time.Minute, natsserver.NoDelete)
	require.NoError(t, err)
	require.NoError(t, resolver.Store(systemAccountID, systemAccountJWT))

	opts := &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
		TrustedOperators:      []*jwt.OperatorClaims{decodedOperatorClaims},
		AccountResolver:       resolver,
		SystemAccount:         systemAccountID,
	}

	server, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	go server.Start()
	require.True(t, server.ReadyForConnections(3*time.Second), "nats-server did not become ready in time")

	systemCreds := newUserCreds(t, systemAccountKey, systemAccountID)
	sysConn, err := nats.Connect(server.ClientURL(), nats.UserCredentialBytes(systemCreds))
	require.NoError(t, err)

	operatorSigningSeed, err := operatorKey.Seed()
	require.NoError(t, err)

	t.Cleanup(func() {
		sysConn.Close()
		server.Shutdown()
		server.WaitForShutdown()
		resolver.Close()
	})

	return &Server{
		URL:                      server.ClientURL(),
		OperatorSigningSeed:      operatorSigningSeed,
		OperatorSigningPublicKey: operatorPublicKey,
		SystemAccountID:          systemAccountID,
		SystemCreds:              systemCreds,
		server:                   server,
		sysConn:                  sysConn,
		resolver:                 resolver,
	}
}

func newUserCreds(t *testing.T, accountKey nkeys.KeyPair, accountID string) []byte {
	t.Helper()

	userKey, err := nkeys.CreateUser()
	require.NoError(t, err)
	userPublicKey, err := userKey.PublicKey()
	require.NoError(t, err)
	userSeed, err := userKey.Seed()
	require.NoError(t, err)

	claims := jwt.NewUserClaims(userPublicKey)
	claims.IssuerAccount = accountID
	userJWT, err := claims.Encode(accountKey)
	require.NoError(t, err)

	creds, err := jwt.FormatUserConfig(userJWT, userSeed)
	require.NoError(t, err)
	return creds
}

func (s *Server) LookupAccountJWT(t *testing.T, accountID string) string {
	t.Helper()

	msg, err := s.sysConn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, 3*time.Second)
	require.NoError(t, err)
	return string(msg.Data)
}
