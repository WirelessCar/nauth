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
				builder := newAccountClaimsBuilder(testClaimsDisplayName, testClaimsAccountPubKey, spec.JetStreamEnabled).
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
				JetStreamEnabled: nauthClaims.JetStreamEnabled,
				AccountLimits:    nauthClaims.AccountLimits,
				JetStreamLimits:  nauthClaims.JetStreamLimits,
				NatsLimits:       nauthClaims.NatsLimits,
				Exports:          nauthClaims.Exports,
				Imports:          nauthClaims.Imports,
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
	boolFalse := false
	require.Equal(t, v1alpha1.AccountClaims{
		JetStreamEnabled: &boolFalse,
	}, result)
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

func Test_AccountClaims_builder_ShouldReturnErrorWhenJetStreamEnablementConflict(t *testing.T) {
	// Given
	var zero int64 = 0
	boolTrue := true

	builder := newAccountClaimsBuilder("my-claims", "ACCID", &boolTrue).
		jetStreamLimits(&v1alpha1.JetStreamLimits{DiskStorage: &zero, MemoryStorage: &zero})

	// When
	claims, err := builder.build()

	// Then
	require.ErrorContains(t, err, "ambiguous JetStream config; requested to be enabled, but no allowed MemoryStorage or DiskStorage supplied")
	require.Nil(t, claims)

}

func Test_validateJetStreamLimits(t *testing.T) {
	operatorLimitsDefault := jwt.NewAccountClaims("test").Limits
	boolTrue := true
	boolFalse := false

	testCases := []struct {
		description             string
		jetStreamExpected       *bool
		limits                  jwt.OperatorLimits
		expectLimitsToEnablesJS bool
		expectErr               string
	}{
		{
			description:             "no expectation should succeed when default OperatorLimits",
			jetStreamExpected:       nil,
			limits:                  operatorLimitsDefault,
			expectLimitsToEnablesJS: false,
		},
		{
			description:       "no expectation should succeed when limits will enable JetStream",
			jetStreamExpected: nil,
			limits: jwt.OperatorLimits{
				JetStreamLimits: jwt.JetStreamLimits{
					DiskStorage:   1024,
					MemoryStorage: 1024,
				},
			},
			expectLimitsToEnablesJS: true,
		},
		{
			description:       "no expectation should succeed when limits will disable JetStream",
			jetStreamExpected: nil,
			limits: jwt.OperatorLimits{
				JetStreamLimits: jwt.JetStreamLimits{
					DiskStorage:   0,
					MemoryStorage: 0,
				},
			},
			expectLimitsToEnablesJS: false,
		},
		{
			description:       "validation should fail when JetStream expected but JetStreamLimits implicitly disables it",
			jetStreamExpected: &boolTrue,
			limits: jwt.OperatorLimits{
				JetStreamLimits: jwt.JetStreamLimits{
					DiskStorage:   0,
					MemoryStorage: 0,
				},
			},
			expectLimitsToEnablesJS: false,
			expectErr:               "ambiguous JetStream config; requested to be enabled, but no allowed MemoryStorage or DiskStorage supplied",
		},
		{
			description:       "validation should fail when JetStream not expected but JetStreamLimits implicitly enables it with explicit DiskStorage",
			jetStreamExpected: &boolFalse,
			limits: jwt.OperatorLimits{
				JetStreamLimits: jwt.JetStreamLimits{
					DiskStorage: 1024,
				},
			},
			expectLimitsToEnablesJS: true,
			expectErr:               "ambiguous JetStream config; requested to be disabled, but supplied MemoryStorage and/or DiskStorage would implicitly enables it",
		},
		{
			description:       "validation should fail when JetStream not expected but JetStreamLimits implicitly enables it with unlimited MemoryStorage",
			jetStreamExpected: &boolFalse,
			limits: jwt.OperatorLimits{
				JetStreamLimits: jwt.JetStreamLimits{
					MemoryStorage: -1,
				},
			},
			expectLimitsToEnablesJS: true,
			expectErr:               "ambiguous JetStream config; requested to be disabled, but supplied MemoryStorage and/or DiskStorage would implicitly enables it",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			// Given
			require.Equalf(t, testCase.expectLimitsToEnablesJS, testCase.limits.IsJSEnabled(), "precondition: limits should match expected JetStream enabled state")

			// When
			err := validateJetStreamLimits(testCase.jetStreamExpected, testCase.limits)

			// Then
			if testCase.expectErr != "" {
				require.ErrorContains(t, err, testCase.expectErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
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
