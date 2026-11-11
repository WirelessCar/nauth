package nats

import (
	"fmt"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestUserPermissions_DoesNotSubstituteUserTagsIntoPublishPermission verifies
// that a regular signing key can issue valid credentials with a tenant tag and
// an inline publish permission containing {{tag(tenant)}}. NATS Server accepts
// the credentials but treats the permission as a literal subject: publishing
// to orders.my-tenant.create is denied, while publishing to the unexpanded
// orders.{{tag(tenant)}}.create subject succeeds.
func TestUserPermissions_DoesNotSubstituteUserTagsIntoPublishPermission(t *testing.T) {
	const permission = "orders.{{tag(tenant)}}.create"
	operator := newOperator(t)
	accountKey := testutil.CreateNatsTestAccountKey()
	signingKey := testutil.CreateNatsTestAccountKey()
	userKey := testutil.CreateNatsTestUserKey()

	account := newAccountWithKey(t, operator, accountKey, func(_ string, claims *jwt.AccountClaims) {
		claims.SigningKeys.Add(signingKey.PublicKey)
	})

	server, sysConn := runServer(t, operator)
	require.NoError(t, applyAccountJWT(t, server, sysConn, account))

	observerConn := connectWithUserCreds(t, server, newUserCreds(t, account))
	sub, err := observerConn.SubscribeSync("orders.>")
	require.NoError(t, err)
	require.NoError(t, observerConn.Flush())

	userClaims := jwt.NewUserClaims(userKey.PublicKey)
	userClaims.IssuerAccount = account.key.PublicKey
	userClaims.Tags.Add("tenant:my-tenant")
	userClaims.Pub.Allow = jwt.StringList{permission}
	userJWT, err := userClaims.Encode(signingKey.Key)
	require.NoError(t, err)

	decodedClaims, err := jwt.DecodeUserClaims(userJWT)
	require.NoError(t, err)
	validation := jwt.CreateValidationResults()
	decodedClaims.Validate(validation)
	require.Empty(t, validation.Errors())
	require.Equal(t, signingKey.PublicKey, decodedClaims.Issuer)
	require.Contains(t, decodedClaims.Tags, "tenant:my-tenant")
	require.Equal(t, jwt.StringList{permission}, decodedClaims.Pub.Allow)

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

	tcs := []struct {
		name    string
		subject string
		allowed bool
	}{
		{
			name:    "literal_template_subject_is_allowed",
			subject: permission,
			allowed: true,
		},
		{
			name:    "substituted_tenant_subject_is_denied",
			subject: "orders.my-tenant.create",
		},
		{
			name:    "other_tenant_subject_is_denied",
			subject: "orders.other-tenant.create",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, userConn.Publish(tc.subject, []byte(tc.name)))
			require.NoError(t, userConn.Flush())

			if tc.allowed {
				msg, err := sub.NextMsg(time.Second)
				require.NoError(t, err)
				require.Equal(t, tc.subject, msg.Subject)
				require.Equal(t, []byte(tc.name), msg.Data)
				return
			}

			select {
			case err := <-permissionErrors:
				require.ErrorContains(t, err, fmt.Sprintf(`Permissions Violation for Publish to %q`, tc.subject))
			case <-time.After(time.Second):
				t.Fatalf("expected NATS Server to reject publish to %q", tc.subject)
			}
			_, err := sub.NextMsg(100 * time.Millisecond)
			require.ErrorIs(t, err, nats.ErrTimeout)
		})
	}
}
