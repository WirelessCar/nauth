package nats

import (
	"fmt"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestScopedSigningKey_SubstitutesUserTagsIntoPublishPermission verifies that
// NATS Server expands multiple tags from a User JWT in a scoped signing key's
// publish template. The template
// `ingestion.{{tag(tenant)}}.{{tag(vehicleId)}}` and tags
// `tenant:/my/tenant` and `vehicleId:abc123` should allow publishing to
// `ingestion./my/tenant.abc123` and reject publishing outside that permission.
// The scoped User JWT has no own permissions or limits; its effective publish
// permission comes from the signing key's template.
func TestScopedSigningKey_SubstitutesUserTagsIntoPublishPermission(t *testing.T) {
	operator := newOperator(t)
	accountKey := testutil.CreateNatsTestAccountKey()
	scopedSignerKey := testutil.CreateNatsTestAccountKey()
	userKey := testutil.CreateNatsTestUserKey()

	account := newAccountWithKey(t, operator, accountKey, func(_ string, claims *jwt.AccountClaims) {
		scope := jwt.NewUserScope()
		scope.Key = scopedSignerKey.PublicKey
		scope.Template.Pub.Allow = jwt.StringList{"ingestion.{{tag(tenant)}}.{{tag(vehicleId)}}"}
		claims.SigningKeys.AddScopedSigner(scope)
	})

	server, sysConn := runServer(t, operator)
	require.NoError(t, applyAccountJWT(t, server, sysConn, account))

	// A separate account user observes messages, since the scoped user only has
	// the publish permission supplied by its signing-key template.
	observerKey := testutil.CreateNatsTestUserKey()
	observerClaims := jwt.NewUserClaims(observerKey.PublicKey)
	observerClaims.IssuerAccount = account.key.PublicKey
	observerClaims.Sub.Allow = jwt.StringList{"ingestion.>"}
	observerJWT, err := observerClaims.Encode(accountKey.Key)
	require.NoError(t, err)
	observerCreds, err := jwt.FormatUserConfig(observerJWT, observerKey.Seed)
	require.NoError(t, err)
	observerConn := connectWithUserCreds(t, server, observerCreds)
	sub, err := observerConn.SubscribeSync("ingestion.>")
	require.NoError(t, err)
	require.NoError(t, observerConn.Flush())

	userClaims := jwt.NewUserClaims(userKey.PublicKey)
	userClaims.IssuerAccount = account.key.PublicKey
	userClaims.SetScoped(true)
	userClaims.Tags.Add("tenant:/my/tenant", "vehicleId:abc123")
	userJWT, err := userClaims.Encode(scopedSignerKey.Key)
	require.NoError(t, err)
	userCreds, err := jwt.FormatUserConfig(userJWT, userKey.Seed)
	require.NoError(t, err)

	permissionErrors := make(chan error, 1)
	userConn, err := nats.Connect(server.ClientURL(),
		nats.UserCredentialBytes(userCreds),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			permissionErrors <- err
		}),
	)
	require.NoError(t, err)
	t.Cleanup(userConn.Close)

	require.NoError(t, userConn.Publish("ingestion./my/tenant.abc123", []byte("allowed")))
	require.NoError(t, userConn.Flush())

	msg, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, "ingestion./my/tenant.abc123", msg.Subject)
	require.Equal(t, []byte("allowed"), msg.Data)

	require.NoError(t, userConn.Publish("ingestion./my/tenant.other", []byte("denied")))
	require.NoError(t, userConn.Flush())
	select {
	case err := <-permissionErrors:
		require.ErrorContains(t, err, `Permissions Violation for Publish to "ingestion./my/tenant.other"`)
	case <-time.After(time.Second):
		t.Fatal("expected NATS Server to reject publish outside the substituted permission")
	}

	_, err = sub.NextMsg(100 * time.Millisecond)
	require.ErrorIs(t, err, nats.ErrTimeout)
}

// TestScopedSigningKey_LargePermissionSetCanAuthenticateAndEnforcePermissions
// verifies that an account can carry 500 distinct publish subjects in a scoped
// signing key while the User JWT stays small enough for the CONNECT control
// line. Putting those same subjects directly in a User JWT produces a
// 17,308-byte token with the current JWT library, exceeding the server's
// default 4 KiB max_control_line. The permissionless scoped user must connect,
// publish to an allowed subject, and be rejected outside the signer's allowlist.
func TestScopedSigningKey_LargePermissionSetCanAuthenticateAndEnforcePermissions(t *testing.T) {
	const permissionCount = 500
	operator := newOperator(t)
	accountKey := testutil.CreateNatsTestAccountKey()
	scopedSignerKey := testutil.CreateNatsTestAccountKey()
	userKey := testutil.CreateNatsTestUserKey()

	permissions := make(jwt.StringList, permissionCount)
	for i := range permissions {
		permissions[i] = fmt.Sprintf("scoped.permissions.%03d", i)
	}

	inlineUserClaims := jwt.NewUserClaims(userKey.PublicKey)
	inlineUserClaims.IssuerAccount = accountKey.PublicKey
	inlineUserClaims.Sub.Allow = permissions
	inlineUserJWT, err := inlineUserClaims.Encode(accountKey.Key)
	require.NoError(t, err)
	require.Greater(t, len(inlineUserJWT), natsserver.MAX_CONTROL_LINE_SIZE,
		"inline permissions produce a %d-byte User JWT, exceeding max_control_line (%d bytes)",
		len(inlineUserJWT), natsserver.MAX_CONTROL_LINE_SIZE)

	account := newAccountWithKey(t, operator, accountKey, func(_ string, claims *jwt.AccountClaims) {
		scope := jwt.NewUserScope()
		scope.Key = scopedSignerKey.PublicKey
		scope.Role = "large-permission-test-user"
		scope.Template.Pub.Allow = permissions
		claims.SigningKeys.AddScopedSigner(scope)
	})

	server, sysConn := runServer(t, operator)
	require.Greater(t, len(account.jwt), natsserver.MAX_CONTROL_LINE_SIZE)
	require.LessOrEqual(t, len(account.jwt), jwt.MaxTokenSize,
		"account JWT size is %d bytes; maximum is %d bytes", len(account.jwt), jwt.MaxTokenSize)
	require.Less(t, len(account.jwt), natsserver.MAX_PAYLOAD_SIZE)
	require.NoError(t, applyAccountJWT(t, server, sysConn, account))

	observerKey := testutil.CreateNatsTestUserKey()
	observerClaims := jwt.NewUserClaims(observerKey.PublicKey)
	observerClaims.IssuerAccount = account.key.PublicKey
	observerClaims.Sub.Allow = jwt.StringList{permissions[0]}
	observerJWT, err := observerClaims.Encode(accountKey.Key)
	require.NoError(t, err)
	observerCreds, err := jwt.FormatUserConfig(observerJWT, observerKey.Seed)
	require.NoError(t, err)
	observerConn := connectWithUserCreds(t, server, observerCreds)
	sub, err := observerConn.SubscribeSync(permissions[0])
	require.NoError(t, err)
	require.NoError(t, observerConn.Flush())

	userClaims := jwt.NewUserClaims(userKey.PublicKey)
	userClaims.IssuerAccount = account.key.PublicKey
	userClaims.SetScoped(true)
	require.True(t, userClaims.HasEmptyPermissions())
	userJWT, err := userClaims.Encode(scopedSignerKey.Key)
	require.NoError(t, err)
	require.Less(t, len(userJWT), natsserver.MAX_CONTROL_LINE_SIZE)
	userCreds, err := jwt.FormatUserConfig(userJWT, userKey.Seed)
	require.NoError(t, err)

	permissionErrors := make(chan error, 1)
	userConn, err := nats.Connect(server.ClientURL(),
		nats.UserCredentialBytes(userCreds),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			permissionErrors <- err
		}),
	)
	require.NoError(t, err)
	t.Cleanup(userConn.Close)

	require.NoError(t, userConn.Publish(permissions[0], []byte("allowed")))
	require.NoError(t, userConn.Flush())
	msg, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, permissions[0], msg.Subject)
	require.Equal(t, []byte("allowed"), msg.Data)

	const deniedSubject = "scoped.permissions.not-in-list"
	require.NoError(t, userConn.Publish(deniedSubject, []byte("denied")))
	require.NoError(t, userConn.Flush())
	select {
	case err := <-permissionErrors:
		require.ErrorContains(t, err, fmt.Sprintf(`Permissions Violation for Publish to %q`, deniedSubject))
	case <-time.After(time.Second):
		t.Fatal("expected NATS Server to reject publish outside the scoped permission list")
	}
	_, err = sub.NextMsg(100 * time.Millisecond)
	require.ErrorIs(t, err, nats.ErrTimeout)
}
