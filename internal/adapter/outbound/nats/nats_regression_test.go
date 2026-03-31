package nats

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestGeneral_ExportImport_ShouldSucceed(t *testing.T) {
	op := newOperator(t)
	accExpKey, _ := nkeys.CreateAccount()
	accExpPubKey, _ := accExpKey.PublicKey()
	accImpKey, _ := nkeys.CreateAccount()
	accImpPubKey, _ := accImpKey.PublicKey()

	tcs := []struct {
		name   string
		accExp account
		accImp account
	}{
		{
			name: "PublicExportOfWildcard_ImportedWithExplicitSubject",
			accExp: newAccountWithKey(t, op, accExpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Exports.Add(&jwt.Export{
					Subject:  "foo.*",
					Type:     jwt.Stream,
					TokenReq: false,
				})
			}),
			accImp: newAccountWithKey(t, op, accImpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Imports.Add(&jwt.Import{
					Account: accExpPubKey,
					Subject: "foo.hello",
					Type:    jwt.Stream,
				})
			}),
		},
		{
			name: "TokenRequiredExportOfWildcardSubject_ImportedWithExplicitSubjectAndMatchingActivationToken",
			accExp: newAccountWithKey(t, op, accExpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Exports.Add(&jwt.Export{
					Subject:  "foo.*",
					Type:     jwt.Stream,
					TokenReq: true,
				})
			}),
			accImp: newAccountWithKey(t, op, accImpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Imports.Add(&jwt.Import{
					Account: accExpPubKey,
					Subject: "foo.hello",
					Token:   newActivationToken(t, accExpKey, accImpPubKey, "foo.hello", jwt.Stream),
					Type:    jwt.Stream,
				})
			}),
		},
		{
			name: "TokenRequiredExportOfWildcardSubject_ImportedWithExplicitSubjectAndWildcardActivationToken",
			accExp: newAccountWithKey(t, op, accExpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Exports.Add(&jwt.Export{
					Subject:  "foo.*",
					Type:     jwt.Stream,
					TokenReq: true,
				})
			}),
			accImp: newAccountWithKey(t, op, accImpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Imports.Add(&jwt.Import{
					Account: accExpPubKey,
					Subject: "foo.hello",
					Token:   newActivationToken(t, accExpKey, accImpPubKey, "foo.*", jwt.Stream),
					Type:    jwt.Stream,
				})
			}),
		},
		{
			name: "TokenRequiredExportOfExplicitSubject_ImportedWithExplicitSubjectAndWildcardActivationToken",
			accExp: newAccountWithKey(t, op, accExpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Exports.Add(&jwt.Export{
					Subject:  "foo.hello",
					Type:     jwt.Stream,
					TokenReq: true,
				})
			}),
			accImp: newAccountWithKey(t, op, accImpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Imports.Add(&jwt.Import{
					Account: accExpPubKey,
					Subject: "foo.hello",
					Token:   newActivationToken(t, accExpKey, accImpPubKey, "foo.*", jwt.Stream),
					Type:    jwt.Stream,
				})
			}),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			server, sysConn := runServer(t, op)

			require.NoError(t, applyAccountJWT(t, server, sysConn, tc.accExp))
			require.NoError(t, applyAccountJWT(t, server, sysConn, tc.accImp))

			accExpConn := connectWithUserCreds(t, server, newUserCreds(t, tc.accExp))
			accImpConn := connectWithUserCreds(t, server, newUserCreds(t, tc.accImp))

			sub, err := accImpConn.SubscribeSync("foo.*")
			require.NoError(t, err)
			require.NoError(t, accImpConn.Flush())

			require.NoError(t, accExpConn.Publish("foo.bar", []byte("bar")))
			require.NoError(t, accExpConn.Publish("foo.hello", []byte("hello")))
			require.NoError(t, accExpConn.Flush())

			msg, err := sub.NextMsg(time.Second)
			require.NoError(t, err)
			require.Equal(t, "foo.hello", msg.Subject)
			require.Equal(t, []byte("hello"), msg.Data)

			_, err = sub.NextMsg(200 * time.Millisecond)
			require.ErrorIs(t, err, nats.ErrTimeout)
		})
	}
}

func TestGeneral_ApplyAccount_ShouldFail(t *testing.T) {
	op := newOperator(t)
	accExpKey, _ := nkeys.CreateAccount()
	accExpPubKey, _ := accExpKey.PublicKey()
	accImpKey, _ := nkeys.CreateAccount()
	accImpPubKey, _ := accImpKey.PublicKey()

	tcs := []struct {
		name      string
		acc       account
		expectErr string
	}{
		{
			name: "ImportWildcardSubjectDoesNotMatchTokenSubject",
			acc: newAccountWithKey(t, op, accImpKey, func(accPubKey string, claims *jwt.AccountClaims) {
				claims.Imports.Add(&jwt.Import{
					Account: accExpPubKey,
					Subject: "foo.*",
					Token:   newActivationToken(t, accExpKey, accImpPubKey, "foo.hello", jwt.Stream),
					Type:    jwt.Stream,
				})
			}),
			expectErr: "[500] jwt validation failed - validation errors: [activation token import subject \"foo.hello\" doesn't match import \"foo.*\"]",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			server, sysConn := runServer(t, op)
			err := applyAccountJWT(t, server, sysConn, tc.acc)
			require.ErrorContains(t, err, "failed to apply account JWT")
			require.ErrorContains(t, err, tc.expectErr)
		})
	}
}

/* *****************************************************************
* Helpers
******************************************************************/

type operator struct {
	rootKey nkeys.KeyPair
	claims  *jwt.OperatorClaims
}

func newOperator(t *testing.T) operator {
	t.Helper()
	rootKey, _ := nkeys.CreateOperator()
	publicKey, _ := rootKey.PublicKey()
	claims := jwt.NewOperatorClaims(publicKey)
	operatorJWT, err := claims.Encode(rootKey)
	require.NoError(t, err)
	decodedClaims, err := jwt.DecodeOperatorClaims(operatorJWT)
	require.NoError(t, err)
	return operator{
		rootKey: rootKey,
		claims:  decodedClaims,
	}
}

type account struct {
	rootKey    nkeys.KeyPair
	rootPubKey string
	jwt        string
}

func newAccount(t *testing.T, operator operator, configure func(accPubKey string, claims *jwt.AccountClaims)) account {
	t.Helper()
	keyPair, _ := nkeys.CreateAccount()
	return newAccountWithKey(t, operator, keyPair, configure)
}

func newAccountWithKey(t *testing.T, operator operator, keyPair nkeys.KeyPair, configure func(accPubKey string, claims *jwt.AccountClaims)) account {
	t.Helper()
	accPubKey, err := keyPair.PublicKey()
	require.NoError(t, err)
	claims := jwt.NewAccountClaims(accPubKey)
	if configure != nil {
		configure(accPubKey, claims)
	}
	accountJWT, err := claims.Encode(operator.rootKey)
	require.NoError(t, err)
	return account{
		rootKey:    keyPair,
		rootPubKey: accPubKey,
		jwt:        accountJWT,
	}
}

func newUserCreds(t *testing.T, account account) []byte {
	t.Helper()
	keyPair, _ := nkeys.CreateUser()
	pubKey, _ := keyPair.PublicKey()
	seed, _ := keyPair.Seed()
	claims := jwt.NewUserClaims(pubKey)
	claims.IssuerAccount = account.rootPubKey
	userJWT, err := claims.Encode(account.rootKey)
	require.NoError(t, err)
	creds, err := jwt.FormatUserConfig(userJWT, seed)
	require.NoError(t, err)
	return creds
}

func newActivationToken(t *testing.T, exporterSignKey nkeys.KeyPair, importerPubKey string, subject string, exportType jwt.ExportType) string {
	t.Helper()
	claims := jwt.NewActivationClaims(importerPubKey)
	claims.ImportSubject = jwt.Subject(subject)
	claims.ImportType = exportType
	token, err := claims.Encode(exporterSignKey)
	require.NoError(t, err)
	return token
}

func runServer(t *testing.T, operator operator) (*natsserver.Server, *nats.Conn) {
	t.Helper()

	sysAcc := newAccount(t, operator, nil)
	resolver, err := natsserver.NewDirAccResolver(t.TempDir(), 0, time.Minute, natsserver.NoDelete)
	require.NoError(t, err)
	require.NoError(t, resolver.Store(sysAcc.rootPubKey, sysAcc.jwt))

	opts := &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
		TrustedOperators:      []*jwt.OperatorClaims{operator.claims},
		AccountResolver:       resolver,
		SystemAccount:         sysAcc.rootPubKey,
	}

	server, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	go server.Start()
	require.True(t, server.ReadyForConnections(3*time.Second), "nats-server did not become ready in time")

	sysConn := connectWithUserCreds(t, server, newUserCreds(t, sysAcc))

	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
		resolver.Close()
	})
	return server, sysConn
}

func applyAccountJWT(t *testing.T, server *natsserver.Server, sysConn *nats.Conn, account account) error {
	t.Helper()

	resp, err := sysConn.Request(
		fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.UPDATE", account.rootPubKey),
		[]byte(account.jwt),
		2*time.Second,
	)
	require.NoError(t, err)

	var result struct {
		Data struct {
			Account string `json:"account"`
			Code    int    `json:"code"`
		} `json:"data"`
		Error struct {
			Account     string `json:"account"`
			Code        int    `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}
	err = json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	if result.Data.Code != 200 {
		return fmt.Errorf("failed to apply account JWT: [%d] %s", result.Error.Code, result.Error.Description)
	}
	require.Equal(t, account.rootPubKey, result.Data.Account)
	_, err = server.LookupAccount(account.rootPubKey)
	require.NoError(t, err)
	return nil
}

func connectWithUserCreds(t *testing.T, server *natsserver.Server, creds []byte) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(server.ClientURL(), nats.UserCredentialBytes(creds))
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	return nc
}
