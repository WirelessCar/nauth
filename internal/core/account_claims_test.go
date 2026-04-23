package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	testClaimsDisplayName   = "test-namespace/test-account"
	testClaimsFakeAccountID = "A000000000000000000000000000000000000000000000000000FAKE"
	testClaimsOperatorSeed  = "SOAF43LTJSU54DLV5VPWKF2ROVF2V6FZZG662Z2CCHDAFKCK5JGLQRP7SA"
	testClaimsAccountPubKey = "AAJCK7774DXTQZAFJLSQIVU76UHGXFZNJVWMT4F7PNRBCYM75LS75UYE"
	testClaimsSigningKey01  = "ACI73NE4LXWVHSYSFXY73WTZVKIKE54PQUMRDYA4EUFYFGEGHKTPCOI4"
	testClaimsSigningKey02  = "ADCECGT44IBBMSNGOEZTVK2QUQSVTJW6FABW7JBFFTITDBHMP6TXM4XG"
)

func Test_AccountClaims(t *testing.T) {

	opSigningKey, _ := nkeys.FromSeed([]byte(testClaimsOperatorSeed))

	testCases := discoverTestCases("approvals/account_claims_test.Test_AccountClaims.{TestCase}.input.yaml")
	require.NotEmpty(t, testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func(t *testing.T) {
			spec, err := loadAccountSpec(testCase.InputFile)
			require.NoError(t, err)

			unitUnderTest := func(spec *v1alpha1.AccountSpec, resolveAccountID resolveAccountIDFn) (*jwt.AccountClaims, error) {
				builder := newAccountClaimsBuilder(testClaimsDisplayName, testClaimsAccountPubKey).
					accountLimits(spec.AccountLimits).
					jetStreamLimits(spec.JetStreamLimits).
					natsLimits(spec.NatsLimits).
					exports(spec.Exports).
					imports(spec.Imports, resolveAccountID)
				builder.signingKey(testClaimsSigningKey01)
				builder.signingKey(testClaimsSigningKey02)
				return builder.build()
			}

			// Build NATS JWT AccountClaims from AccountSpec
			natsClaims, err := unitUnderTest(spec, func(accountRef domain.NamespacedName) (accountID string, err error) {
				accountID = fakeAccountId(accountRef)
				return
			})
			require.NoError(t, err)
			require.NotNil(t, natsClaims)
			// Ensure that the NATS JWT can be encoded
			natsJWT, err := signAccountJWT(natsClaims, opSigningKey)
			require.NoError(t, err)
			require.NotEmpty(t, natsJWT)

			normalizedNatsClaims := normalizeClaimsForApproval(natsClaims)

			// Verify NATS JWT claims structure
			approvals.VerifyJSONStruct(t, normalizedNatsClaims,
				approvals.Options().ForFile().WithAdditionalInformation("output.nats"))

			// Convert back to NAuth AccountClaims and verify YAML structure (used in Account CR `status.claims`)
			nauthClaims := convertNatsAccountClaims(natsClaims)
			nauthClaimsYaml, err := yaml.Marshal(nauthClaims)
			require.NoError(t, err)
			approvals.VerifyString(t, string(nauthClaimsYaml), approvals.Options().
				ForFile().WithAdditionalInformation("output.nauth").
				ForFile().WithExtension(".yaml"))

			// Finally; rebuild the claims from the output to verify round-trip integrity

			// Verify that the resulting NAuth AccountClaim generates the same NATS JWT when encoded
			rebuiltNatsClaims := &v1alpha1.AccountSpec{
				AccountLimits:   nauthClaims.AccountLimits,
				JetStreamLimits: nauthClaims.JetStreamLimits,
				NatsLimits:      nauthClaims.NatsLimits,
				Exports:         nauthClaims.Exports,
				Imports:         nauthClaims.Imports,
			}
			natsClaimsRebuilt, err := unitUnderTest(rebuiltNatsClaims, func(accountRef domain.NamespacedName) (accountID string, err error) {
				// For the rebuild, override the mock to always return the fake account ID (account ref is lost)
				accountID = testClaimsFakeAccountID
				return
			})
			require.NoError(t, err)
			require.NotNil(t, natsClaimsRebuilt)
			// Sign the JWT to ensure matching issuer details
			_, err = natsClaimsRebuilt.Encode(opSigningKey)
			require.NoError(t, err)

			normalizedNatsClaimsRebuilt := normalizeClaimsForApproval(natsClaimsRebuilt)
			// The rebuilt claims will have fake account ID for imports, normalize for equality check
			overrideImportAccountIDs(normalizedNatsClaims, testClaimsFakeAccountID)
			assert.Equal(t, normalizedNatsClaims, normalizedNatsClaimsRebuilt)
		})
	}
}

func Test_AccountClaims_convertNatsAccountClaims_ShouldSucceed_WhenMinimal(t *testing.T) {
	// Given
	claims := jwt.NewAccountClaims(testClaimsFakeAccountID)

	// When
	result := convertNatsAccountClaims(claims)

	// Then
	require.Equal(t, v1alpha1.AccountClaims{}, result)
}

func Test_AccountClaims_hashSignedAccountJWTClaims_ShouldGenerateDeterministicHash(t *testing.T) {
	// Given
	opSignKey, _ := nkeys.CreateOperator()
	accKey, _ := nkeys.CreateAccount()
	accID, _ := accKey.PublicKey()
	accSignKey, _ := nkeys.CreateAccount()
	accSignPubKey, _ := accSignKey.PublicKey()
	toJWT := func(claims *jwt.AccountClaims, opSignKey nkeys.KeyPair) string {
		signedJWT, err := claims.Encode(opSignKey)
		require.NoError(t, err)
		return signedJWT
	}

	claims0 := jwt.NewAccountClaims(accID)
	claims0.Name = "Test Account"
	claims0.SigningKeys.Add(accSignPubKey)
	jwt0 := toJWT(claims0, opSignKey)

	time.Sleep(1010 * time.Millisecond) // Ensure that time-based fields would differ if not fixed

	claims1 := jwt.NewAccountClaims(accID)
	claims1.Name = "Test Account"
	claims1.SigningKeys.Add(accSignPubKey)
	jwt1 := toJWT(claims1, opSignKey)

	unitUnderTest := func(jwt string) string {
		hash, err := hashSignedAccountJWTClaims(jwt)
		require.NoError(t, err)
		return hash
	}

	// When
	claims0Hash := unitUnderTest(jwt0)

	// Then
	require.Equal(t, claims0Hash, unitUnderTest(jwt0), "expected hash to be deterministic for same JWT")
	require.Equal(t, claims0Hash, unitUnderTest(jwt1), "expected hash to be deterministic for same claims and signing key")

	opSignKeyOther, _ := nkeys.CreateOperator()
	require.NotEqual(t, claims0Hash, unitUnderTest(toJWT(claims0, opSignKeyOther)), "expected hash to change when signing key changes")

	claimsOther := *claims0
	claimsOther.Description = "Claims V2"
	require.NotEqual(t, claims0Hash, unitUnderTest(toJWT(&claimsOther, opSignKey)), "expected hash to change when claims content changes")
}

// Test_NatsJWTUnlimitedCheckShouldNotBeBroken verifies that JWT lib is still broken. If this test fails, we should be able to clean up custom IsUnlimited() functions.
// Bug: https://github.com/nats-io/jwt/issues/249 (bound to nats-io/jwt v2.8.1) JetStreamLimits.IsUnlimited() not consistent with (unlimited) NewAccountClaims
func Test_NatsJWTUnlimitedCheckShouldNotBeBroken(t *testing.T) {
	claims := jwt.NewAccountClaims("test")

	require.Truef(t, claims.Limits.AccountLimits.IsUnlimited(), "expected AccountLimits to be unlimited by default")
	require.Truef(t, claims.Limits.NatsLimits.IsUnlimited(), "expected NatsLimits to be unlimited by default")

	// Verify temporary fix:
	// isUnlimitedJetStreamLimits and newUnlimitedJetStreamLimits should be deleted when the bug is fixed.
	require.Truef(t, isUnlimitedJetStreamLimits(claims.Limits.JetStreamLimits), "expected JetStreamLimits to be unlimited by default using custom check")
	require.Equalf(t, claims.Limits.JetStreamLimits, newUnlimitedJetStreamLimits(), "expected default NewAccountClaim JetStreamLimits to match newUnlimitedJetStreamLimits()")

	// Verify issue:
	require.Falsef(t, claims.Limits.JetStreamLimits.IsUnlimited(), "expected JWT lib to still be broken for JetStreamLimits.IsUnlimited()")
	require.Falsef(t, claims.Limits.IsUnlimited(), "expected JWT lib to still be broken for OperatorLimits.IsUnlimited()")

}

func fakeAccountId(accountRef domain.NamespacedName) string {
	return fmt.Sprintf("A%055s", strings.ToUpper(strings.ReplaceAll(accountRef.Name+accountRef.Namespace, "-", "")))
}

func loadAccountSpec(filePath string) (*v1alpha1.AccountSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var spec v1alpha1.AccountSpec
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

func normalizeClaimsForApproval(claims *jwt.AccountClaims) *jwt.AccountClaims {
	data, _ := json.Marshal(claims)
	result := jwt.NewAccountClaims(claims.Subject)
	err := json.Unmarshal(data, &result)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal claims JSON: %s", err.Error()))
	}
	result.IssuedAt = int64(1700000000)
	result.ID = "TEST-JWT-ID-STATIC-FOR-APPROVAL-TESTS"
	return result
}

func overrideImportAccountIDs(claims *jwt.AccountClaims, overrideAccount string) {
	for _, importClaim := range claims.Imports {
		importClaim.Account = overrideAccount
	}
}
