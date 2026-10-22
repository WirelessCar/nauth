package nats

import (
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
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
