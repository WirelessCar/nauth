package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

			ctx := context.Background()
			accountReaderMock := NewAccountReaderMock()
			getAccountCall := accountReaderMock.mockGetCallback(mock.Anything, mock.Anything, func(accountRef domain.NamespacedName) (*v1alpha1.Account, error) {
				accountID := fakeAccountId(accountRef)
				account := &v1alpha1.Account{}
				account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
				return account, nil
			})

			// Build NATS JWT AccountClaims from AccountSpec
			builder := newAccountClaimsBuilder(ctx, testClaimsDisplayName, *spec, testClaimsAccountPubKey, accountReaderMock)
			builder.signingKey(testClaimsSigningKey01)
			builder.signingKey(testClaimsSigningKey02)

			natsClaims, err := builder.build()
			require.NoError(t, err)
			require.NotNil(t, natsClaims)
			// Ensure that the NATS JWT can be encoded
			natsJwt, err := signAccountJWT(natsClaims, opSigningKey)
			require.NoError(t, err)
			require.NotEmpty(t, natsJwt)

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

			// For the rebuild, override the mock to always return the fake account ID (account ref is lost)
			getAccountCall.RunFn = func(args mock.Arguments) {
				account := &v1alpha1.Account{}
				account.SetLabel(v1alpha1.AccountLabelAccountID, testClaimsFakeAccountID)
				getAccountCall.Return(account, nil)
			}

			// Verify that the resulting NAuth AccountClaim generates the same NATS JWT when encoded
			rebuiltNatsClaims := &v1alpha1.AccountSpec{
				AccountLimits:   nauthClaims.AccountLimits,
				JetStreamLimits: nauthClaims.JetStreamLimits,
				NatsLimits:      nauthClaims.NatsLimits,
				Exports:         nauthClaims.Exports,
				Imports:         nauthClaims.Imports,
			}
			rebuilder := newAccountClaimsBuilder(ctx, testClaimsDisplayName, *rebuiltNatsClaims, testClaimsAccountPubKey, accountReaderMock)
			rebuilder.signingKey(testClaimsSigningKey01)
			rebuilder.signingKey(testClaimsSigningKey02)

			natsClaimsRebuilt, err := rebuilder.build()
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
	var ptrNoLimit int64 = -1
	var ptrDisabled int64 = 0
	var ptrTrue = true
	require.Equal(t, v1alpha1.AccountClaims{
		AccountLimits: &v1alpha1.AccountLimits{
			Imports:         &ptrNoLimit,
			Exports:         &ptrNoLimit,
			WildcardExports: &ptrTrue,
			Conn:            &ptrNoLimit,
			LeafNodeConn:    &ptrNoLimit,
		},
		JetStreamLimits: &v1alpha1.JetStreamLimits{
			MemoryStorage:        &ptrDisabled,
			DiskStorage:          &ptrDisabled,
			Streams:              &ptrDisabled,
			Consumer:             &ptrDisabled,
			MaxAckPending:        &ptrDisabled,
			MemoryMaxStreamBytes: &ptrDisabled,
			DiskMaxStreamBytes:   &ptrDisabled,
			MaxBytesRequired:     false,
		},
		NatsLimits: &v1alpha1.NatsLimits{
			Subs:    &ptrNoLimit,
			Data:    &ptrNoLimit,
			Payload: &ptrNoLimit,
		},
	}, result)
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

func normalizeClaimsForApproval(claims *jwt.AccountClaims) map[string]interface{} {
	data, _ := json.Marshal(claims)
	var result map[string]interface{}
	err := json.Unmarshal(data, &result)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal claims JSON: %s", err.Error()))
	}

	result["iat"] = int64(1700000000)
	result["jti"] = "TEST-JWT-ID-STATIC-FOR-APPROVAL-TESTS"

	return result
}

func overrideImportAccountIDs(claims map[string]interface{}, overrideAccount string) {
	nats := claims["nats"].(map[string]interface{})
	if nats["imports"] != nil {
		for _, importClaim := range nats["imports"].([]interface{}) {
			importMap := importClaim.(map[string]interface{})
			importMap["account"] = overrideAccount
		}
	}
}
