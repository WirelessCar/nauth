package core

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

const (
	userClaimsTestDisplayName     = "test-namespace/test-user"
	userClaimsTestAccountPubKey   = "AAJCK7774DXTQZAFJLSQIVU76UHGXFZNJVWMT4F7PNRBCYM75LS75UYE"
	userClaimsTestAccountSignSeed = "SAAPQGHCXP3M5THZ4JIJ2X6DJPXIBDX4DHVEI2ODY37NKI7R7YTIHNSTW4"
	userClaimsTestUserPubKey      = "UAP35KHDBNR3WKNJ76YJMKEOFWNMPUN4U5LX2A2BCYSSXL3AXKCAEIM7"
)

func TestClaims(t *testing.T) {
	acSigningKey, _ := nkeys.FromSeed([]byte(userClaimsTestAccountSignSeed))

	testCases := discoverTestCases("approvals/userClaims_test.TestClaims.{TestCase}.input.yaml")
	require.NotEmpty(t, testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func(t *testing.T) {
			spec, err := loadUserSpec(testCase.InputFile)
			require.NoError(t, err)

			// Build NATS JWT UserClaims from UserSpec
			builder := newUserClaimsBuilder(userClaimsTestDisplayName, *spec, userClaimsTestUserPubKey, userClaimsTestAccountPubKey)

			natsClaims := builder.build()
			require.NotNil(t, natsClaims)
			// Ensure that the NATS JWT can be encoded
			natsJwt, err := natsClaims.Encode(acSigningKey)
			require.NoError(t, err)
			require.NotEmpty(t, natsJwt)

			normalizedNatsClaims := normalizeUserClaimsForApproval(natsClaims)

			// Verify NATS JWT claims structure
			approvals.VerifyJSONStruct(t, normalizedNatsClaims,
				approvals.Options().ForFile().WithAdditionalInformation("output.nats"))

			// Convert back to NAuth UserClaims and verify YAML structure (used in User CR `status.claims`)
			nauthClaims := toNAuthUserClaims(natsClaims)
			nauthClaimsYaml, err := yaml.Marshal(nauthClaims)
			require.NoError(t, err)
			approvals.VerifyString(t, string(nauthClaimsYaml), approvals.Options().
				ForFile().WithAdditionalInformation("output.nauth").
				ForFile().WithExtension(".yaml"))

			// Verify that the resulting NAuth UserClaim generates the same NATS JWT when encoded
			rebuiltNatsClaims := &v1alpha1.UserSpec{
				AccountName: nauthClaims.AccountName,
				Permissions: nauthClaims.Permissions,
				UserLimits:  nauthClaims.UserLimits,
				NatsLimits:  nauthClaims.NatsLimits,
			}
			rebuilder := newUserClaimsBuilder(userClaimsTestDisplayName, *rebuiltNatsClaims, userClaimsTestUserPubKey, userClaimsTestAccountPubKey)

			natsClaimsRebuilt := rebuilder.build()
			require.NoError(t, err)
			require.NotNil(t, natsClaimsRebuilt)
			// Sign the JWT to ensure matching issuer details
			_, err = natsClaimsRebuilt.Encode(acSigningKey)
			require.NoError(t, err)

			normalizedNatsClaimsRebuilt := normalizeUserClaimsForApproval(natsClaimsRebuilt)
			assert.Equal(t, normalizedNatsClaims, normalizedNatsClaimsRebuilt)
		})
	}
}

func loadUserSpec(filePath string) (*v1alpha1.UserSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var spec v1alpha1.UserSpec
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

func normalizeUserClaimsForApproval(claims *jwt.UserClaims) map[string]interface{} {
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
