package account

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
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

func TestMain(m *testing.M) {
	approvals.UseFolder("approvals")
	os.Exit(m.Run())
}

func TestClaims(t *testing.T) {

	opSigningKey, _ := nkeys.FromSeed([]byte(testClaimsOperatorSeed))

	testCases := discoverTestCases("approvals/claims_test.TestClaims.{TestCase}.input.yaml")
	require.NotEmpty(t, testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func(t *testing.T) {
			spec, err := loadAccountSpec(testCase.InputFile)
			require.NoError(t, err)

			ctx := context.Background()
			accountGetterMock := NewAccountGetterMock()
			getAccountCall := accountGetterMock.On("Get", mock.Anything, mock.Anything, mock.Anything)
			getAccountCall.RunFn = func(args mock.Arguments) {
				accountID := fakeAccountId(args.String(1), args.String(2))
				account := &v1alpha1.Account{}
				account.Labels = map[string]string{
					k8s.LabelAccountID: accountID,
				}
				getAccountCall.Return(*account, nil)
			}

			// Build NATS JWT AccountClaims from AccountSpec
			builder := newClaimsBuilder(ctx, testClaimsDisplayName, *spec, testClaimsAccountPubKey, accountGetterMock)
			builder.signingKey(testClaimsSigningKey01)
			builder.signingKey(testClaimsSigningKey02)

			natsClaims, err := builder.build()
			require.NoError(t, err)
			require.NotNil(t, natsClaims)
			// Ensure that the NATS JWT can be encoded
			natsJwt, err := natsClaims.Encode(opSigningKey)
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
				account.Labels = map[string]string{
					k8s.LabelAccountID: testClaimsFakeAccountID,
				}
				getAccountCall.Return(*account, nil)
			}

			// Verify that the resulting NAuth AccountClaim generates the same NATS JWT when encoded
			rebuiltNatsClaims := &v1alpha1.AccountSpec{
				AccountLimits:   nauthClaims.AccountLimits,
				JetStreamLimits: nauthClaims.JetStreamLimits,
				NatsLimits:      nauthClaims.NatsLimits,
				Exports:         nauthClaims.Exports,
				Imports:         nauthClaims.Imports,
			}
			rebuilder := newClaimsBuilder(ctx, testClaimsDisplayName, *rebuiltNatsClaims, testClaimsAccountPubKey, accountGetterMock)
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

type TestCaseInputFile struct {
	TestName  string
	InputFile string
}

func discoverTestCases(pattern string) []TestCaseInputFile {
	testCasePlaceholder := "{TestCase}"
	if !strings.Contains(pattern, testCasePlaceholder) {
		panic(fmt.Sprintf("pattern must contain %s placeholder: %s", testCasePlaceholder, pattern))
	}
	globPattern := strings.ReplaceAll(pattern, testCasePlaceholder, "*")
	files, err := filepath.Glob(globPattern)
	if err != nil {
		panic(fmt.Sprintf("unable to glob pattern %q: %s", globPattern, err.Error()))
	}
	testPattern := strings.ReplaceAll(pattern, testCasePlaceholder, "(?P<TestCase>.*)")
	regex := regexp.MustCompile(testPattern)
	var testCases []TestCaseInputFile
	for _, file := range files {
		if regex.MatchString(file) {
			match := regex.FindStringSubmatch(file)
			for i, name := range regex.SubexpNames() {
				if name == "TestCase" {
					testCases = append(testCases, TestCaseInputFile{
						TestName:  match[i],
						InputFile: file,
					})
				}
			}
		}
	}
	return testCases
}

func fakeAccountId(accountNameRef string, namespace string) string {
	return fmt.Sprintf("A%055s", strings.ToUpper(strings.ReplaceAll(accountNameRef+namespace, "-", "")))
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
