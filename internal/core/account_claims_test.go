package core

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	testClaimsDisplayName   = "test-namespace/test-account"
	testClaimsOperatorSeed  = "SOAF43LTJSU54DLV5VPWKF2ROVF2V6FZZG662Z2CCHDAFKCK5JGLQRP7SA"
	testClaimsAccountPubKey = "AAJCK7774DXTQZAFJLSQIVU76UHGXFZNJVWMT4F7PNRBCYM75LS75UYE"
	testClaimsSigningKey01  = "ACI73NE4LXWVHSYSFXY73WTZVKIKE54PQUMRDYA4EUFYFGEGHKTPCOI4"
	testClaimsSigningKey02  = "ADCECGT44IBBMSNGOEZTVK2QUQSVTJW6FABW7JBFFTITDBHMP6TXM4XG"
)

type TestAccountClaimsSpec struct {
	AccountLimits    *nauth.AccountLimits   `json:"accountLimits,omitempty"`
	JetStreamLimits  *nauth.JetStreamLimits `json:"jetStreamLimits,omitempty"`
	JetStreamEnabled *bool                  `json:"jetStreamEnabled,omitempty"`
	NatsLimits       *nauth.NatsLimits      `json:"natsLimits,omitempty"`
	Exports          v1alpha1.Exports       `json:"exports,omitempty"` // TODO: Migrate to nauth.Exports
	Imports          nauth.Imports          `json:"imports,omitempty"`
}

func Test_AccountClaims(t *testing.T) {

	opSigningKey, _ := nkeys.FromSeed([]byte(testClaimsOperatorSeed))

	testCases := discoverTestCases("approvals/account_claims_test.Test_AccountClaims.{TestCase}.input.yaml")
	require.NotEmpty(t, testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func(t *testing.T) {
			spec, err := loadTestAccountClaimsSpec(testCase.InputFile)
			require.NoError(t, err)

			unitUnderTest := func(spec *TestAccountClaimsSpec) (*jwt.AccountClaims, error) {
				builder := newAccountClaimsBuilder(testClaimsAccountPubKey, spec.JetStreamEnabled).
					displayName(testClaimsDisplayName).
					accountLimits(spec.AccountLimits).
					jetStreamLimits(spec.JetStreamLimits).
					natsLimits(spec.NatsLimits).
					exports(spec.Exports)
				if len(spec.Imports) > 0 {
					inlineImportGroup := nauth.ImportGroup{
						Name:    GroupNameInline,
						Imports: spec.Imports,
					}
					require.NoError(t, builder.addImportGroup(inlineImportGroup))
				}
				builder.signingKey(testClaimsSigningKey01)
				builder.signingKey(testClaimsSigningKey02)
				return builder.build()
			}

			// Build NATS JWT AccountClaims from AccountSpec
			natsClaims, err := unitUnderTest(spec)
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
			rebuiltNatsClaims := &TestAccountClaimsSpec{
				JetStreamEnabled: nauthClaims.JetStreamEnabled,
				AccountLimits:    nauthClaims.AccountLimits,
				JetStreamLimits:  nauthClaims.JetStreamLimits,
				NatsLimits:       nauthClaims.NatsLimits,
				Exports:          nauthClaims.Exports,
				Imports:          nauthClaims.Imports,
			}
			natsClaimsRebuilt, err := unitUnderTest(rebuiltNatsClaims)
			require.NoError(t, err)
			require.NotNil(t, natsClaimsRebuilt)
			// Sign the JWT to ensure matching issuer details
			_, err = natsClaimsRebuilt.Encode(opSigningKey)
			require.NoError(t, err)

			normalizedNatsClaimsRebuilt := normalizeClaimsForApproval(natsClaimsRebuilt)
			assert.Equal(t, normalizedNatsClaims, normalizedNatsClaimsRebuilt)
		})
	}
}

func Test_AccountClaims_addExportRuleGroup_ShouldNotAlterExistingRulesOnConflict(t *testing.T) {
	// Given
	builder := newAccountClaimsBuilder(testClaimsAccountPubKey, nil).
		exports(v1alpha1.Exports{
			{
				Subject: "foo.>",
				Type:    v1alpha1.Stream,
			},
		})

	// When
	err := builder.addExportRuleGroup([]v1alpha1.AccountExportRule{
		{
			Subject: "bar.>",
			Type:    v1alpha1.Stream,
		},
		{
			Subject: "foo.*",
			Type:    v1alpha1.Stream,
		},
	})

	// Then
	require.ErrorContains(t, err, "failed to append export rule group:")
	require.ErrorContains(t, err, "stream export subject \"foo.*\" already exports \"foo.>\"")
	expected := jwt.Exports{
		{
			Subject: "foo.>",
			Type:    jwt.Stream,
		},
	}
	require.Equal(t, expected, builder.claim.Exports)
}

func Test_AccountClaims_convertNatsAccountClaims_ShouldSucceed_WhenMinimal(t *testing.T) {
	// Given
	claims := jwt.NewAccountClaims("ACCID")

	// When
	result := convertNatsAccountClaims(claims)

	// Then
	boolFalse := false
	require.Equal(t, nauth.AccountClaims{
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

	builder := newAccountClaimsBuilder("ACCID", &boolTrue).
		jetStreamLimits(&nauth.JetStreamLimits{DiskStorage: &zero, MemoryStorage: &zero})

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

func loadTestAccountClaimsSpec(filePath string) (*TestAccountClaimsSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var spec TestAccountClaimsSpec
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
