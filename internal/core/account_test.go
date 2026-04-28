package core

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type AccountManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	keys          testKeys
	sauCreds      domain.NatsUserCreds
	natsURL       string
	clusterTarget clusterTarget

	accountReaderMock         *AccountReaderMock
	natsSysClientMock         *NatsSysClientMock
	natsSysConnMock           *NatsSysConnectionMock
	natsAccClientMock         *NatsAccountClientMock
	natsAccConnMock           *NatsAccConnectionMock
	clusterTargetResolverMock *clusterTargetResolverMock
	secretManagerMock         *secretManagerMock

	unitUnderTest *AccountManager
}

func (t *AccountManagerTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.keys = testKeys1()
	t.sauCreds = domain.NatsUserCreds{
		Creds:     []byte("FAKE_CREDENTIALS"),
		AccountID: "FAKE_SYS_ACCOUNT_ID",
	}
	t.natsURL = "nats://nats:4222"
	t.clusterTarget = clusterTarget{
		NatsURL:            t.natsURL,
		OperatorSigningKey: t.keys.OpSign.KeyPair,
		SystemAdminCreds:   t.sauCreds,
	}

	t.clusterTargetResolverMock = newClusterTargetResolverMock()
	t.secretManagerMock = newSecretManagerMock()
	t.accountReaderMock = NewAccountReaderMock()
	t.natsSysClientMock = NewNatsSysClientMock()
	t.natsSysConnMock = NewNatsSysConnectionMock()
	t.natsAccClientMock = NewNatsAccountClientMock()
	t.natsAccConnMock = NewNatsAccountConnectionMock()

	var err error
	t.unitUnderTest, err = newAccountManager(
		t.natsSysClientMock,
		t.natsAccClientMock,
		t.accountReaderMock,
		t.clusterTargetResolverMock,
		t.secretManagerMock,
	)
	t.NoError(err)
}

func (t *AccountManagerTestSuite) TearDownTest() {
	t.assertAndResetAllMock()
}

func (t *AccountManagerTestSuite) assertAndResetAllMock() {
	t.assertAllMocks()
	t.resetAllMocks()
}

func (t *AccountManagerTestSuite) assertAllMocks() {
	t.clusterTargetResolverMock.AssertExpectations(t.T())
	t.secretManagerMock.AssertExpectations(t.T())
	t.accountReaderMock.AssertExpectations(t.T())
	t.natsSysClientMock.AssertExpectations(t.T())
	t.natsSysConnMock.AssertExpectations(t.T())
	t.natsAccClientMock.AssertExpectations(t.T())
	t.natsAccConnMock.AssertExpectations(t.T())
}

func (t *AccountManagerTestSuite) resetAllMocks() {
	t.clusterTargetResolverMock.Mock = mock.Mock{}
	t.secretManagerMock.Mock = mock.Mock{}
	t.accountReaderMock.Mock = mock.Mock{}
	t.natsSysClientMock.Mock = mock.Mock{}
	t.natsSysConnMock.Mock = mock.Mock{}
	t.natsAccClientMock.Mock = mock.Mock{}
	t.natsAccConnMock.Mock = mock.Mock{}
}

func TestAccountManager_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountManagerTestSuite))
}

func (t *AccountManagerTestSuite) Test_Create_ShouldSucceed() {
	// Given
	var (
		caughtAccountJWT    string
		caughtRootKeyPair   nkeys.KeyPair
		caughtSignAccountID string
		caughtSignKeyPair   nkeys.KeyPair
	)
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	var natsLimitsSubs int64 = 100

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, "")
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(accountID string, signKeyPair nkeys.KeyPair) {
		caughtSignAccountID = accountID
		caughtSignKeyPair = signKeyPair
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
			},
			Spec: v1alpha1.AccountSpec{
				NatsLimits: &v1alpha1.NatsLimits{
					Subs: &natsLimitsSubs,
				},
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, caughtRootKeyPair, caughtSignKeyPair)

	t.Equal(result.AccountID, caughtSignAccountID)
	t.Equal(natsLimitsSubs, jwtClaims.Limits.Subs)
}

func (t *AccountManagerTestSuite) Test_Create_ShouldSucceed_WhenAccountExplicitCluster() {
	// Given
	var (
		caughtAccountJWT    string
		caughtRootKeyPair   nkeys.KeyPair
		caughtSignAccountID string
		caughtSignKeyPair   nkeys.KeyPair
	)

	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	natsLimitsSubs := int64(100)

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, &v1alpha1.NatsClusterRef{
		Namespace: "account-namespace",
		Name:      "account-namespace-cluster",
	}, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, "")
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(accountID string, signKeyPair nkeys.KeyPair) {
		caughtSignAccountID = accountID
		caughtSignKeyPair = signKeyPair
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
			},
			Spec: v1alpha1.AccountSpec{
				NatsClusterRef: &v1alpha1.NatsClusterRef{
					Name: "account-namespace-cluster",
				},
				NatsLimits: &v1alpha1.NatsLimits{
					Subs: &natsLimitsSubs,
				},
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, caughtRootKeyPair, caughtSignKeyPair)

	t.Equal(result.AccountID, caughtSignAccountID)
	t.Equal(natsLimitsSubs, jwtClaims.Limits.Subs)
}

func (t *AccountManagerTestSuite) Test_Create_ShouldSucceed_WhenSecretsAlreadyExist() {
	// Given
	var (
		caughtAccountJWT string
	)
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	keys := testKeys1()
	var natsLimitsSubs int64 = 100

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, "", &Secrets{
		Root: keys.AcRoot.KeyPair,
		Sign: keys.AcSign.KeyPair,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
			},
			Spec: v1alpha1.AccountSpec{
				NatsLimits: &v1alpha1.NatsLimits{
					Subs: &natsLimitsSubs,
				},
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, keys.AcRoot.KeyPair, keys.AcSign.KeyPair)

	t.Equal(natsLimitsSubs, jwtClaims.Limits.Subs)
}

func (t *AccountManagerTestSuite) Test_CreateOrUpdate_ShouldSucceed_Adoptions() {
	testCases := discoverTestCases("approvals/account_test.TestAccountManager_TestSuite.Test_CreateOrUpdate_ShouldSucceed_Adoptions.{TestCase}.input.yaml")
	t.Require().NotEmpty(testCases, "no test cases discovered")

	for _, testCase := range testCases {
		t.Run(testCase.TestName, func() {
			// Given
			t.resetAllMocks()
			inputData, err := os.ReadFile(testCase.InputFile)
			t.Require().NoError(err)
			var input domain.AccountResources
			t.Require().NoError(yaml.UnmarshalStrict(inputData, &input))
			t.Require().NotEmpty(input.Account.Name, "account.name must be present in input file")
			t.Require().NotEmpty(input.Account.Namespace, "account.namespace must be present in input file")
			t.Require().Nil(input.Account.Spec.NatsClusterRef, "account.natsClusterRef must be absent in input file")

			accountRef := domain.NewNamespacedName(input.Account.Namespace, input.Account.Name)
			keys := testKeys1()

			t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
			t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, "", &Secrets{
				Root: keys.AcRoot.KeyPair,
				Sign: keys.AcSign.KeyPair,
			})
			t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
			var caughtAccountJWT string
			t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
			t.natsSysConnMock.mockDisconnect()

			// When
			result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, input)

			// Then
			t.assertAndResetAllMock()
			t.Require().NoError(err)
			t.Require().NotNil(result)
			t.Require().NotEmpty(caughtAccountJWT)

			t.NotNil(result.Claims)
			t.NotEmpty(result.ClaimsHash)
			t.verifyAccountResult(result, caughtAccountJWT, t.keys.AcRoot.KeyPair, t.keys.AcSign.KeyPair)

			resultYaml, err := yaml.Marshal(result)
			t.Require().NoError(err)
			approvals.VerifyString(t.T(), string(resultYaml), approvalOptionsForTestSuite(&t.Suite).
				ForFile().WithExtension(".yaml"))
		})
	}
}

func (t *AccountManagerTestSuite) Test_Create_ShouldFail_WhenClusterNotFound() {
	// Given
	t.clusterTargetResolverMock.mockGetClusterTargetError(t.ctx, nil, fmt.Errorf("test cluster not found"))

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})

	// Then
	t.ErrorContains(err, "test cluster not found")
	t.Nil(result)
}

func (t *AccountManagerTestSuite) Test_Create_ShouldFail_WhenExistingSecretsAreInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsFoundError(t.ctx, accountRef, "", fmt.Errorf("root secret is malformed"))

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "existing account secrets are invalid; account creation requires manual intervention")
	t.ErrorContains(err, "root secret is malformed")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldSucceed() {
	// Given
	var (
		caughtAccountJWT string
	)
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := t.keys.AcRoot.PublicKey

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: t.keys.AcRoot.KeyPair,
		Sign: t.keys.AcSign.KeyPair,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	t.verifyAccountResult(result, caughtAccountJWT, t.keys.AcRoot.KeyPair, t.keys.AcSign.KeyPair)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldSkipUpload_WhenClaimsHashUnchanged() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := t.keys.AcRoot.PublicKey

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: t.keys.AcRoot.KeyPair,
		Sign: t.keys.AcSign.KeyPair,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) {})
	t.natsSysConnMock.mockDisconnect()

	initialResult, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})
	t.Require().NoError(err)
	t.Require().NotNil(initialResult)
	t.Require().NotEmpty(initialResult.ClaimsHash)
	t.assertAndResetAllMock()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: t.keys.AcRoot.KeyPair,
		Sign: t.keys.AcSign.KeyPair,
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
			Status: v1alpha1.AccountStatus{
				ClaimsHash: initialResult.ClaimsHash,
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(initialResult.ClaimsHash, result.ClaimsHash)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldUploadNewAccountJWT_WhenOperatorSigningKeyHashChanged() {
	// Given
	var caughtAccountJWT string
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := t.keys.AcRoot.PublicKey

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: t.keys.AcRoot.KeyPair,
		Sign: t.keys.AcSign.KeyPair,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) {})
	t.natsSysConnMock.mockDisconnect()

	initialResult, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})
	t.Require().NoError(err)
	t.Require().NotNil(initialResult)
	t.Require().NotEmpty(initialResult.ClaimsHash)
	t.assertAndResetAllMock()

	newOpSignKey, err := nkeys.CreateOperator()
	t.Require().NoError(err)
	newOpSignKeyPublic, err := newOpSignKey.PublicKey()
	t.Require().NoError(err)
	t.clusterTarget.OperatorSigningKey = newOpSignKey

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: t.keys.AcRoot.KeyPair,
		Sign: t.keys.AcSign.KeyPair,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
			Status: v1alpha1.AccountStatus{
				ClaimsHash: initialResult.ClaimsHash,
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.NotEqual(initialResult.ClaimsHash, result.ClaimsHash)
	t.Equal(newOpSignKeyPublic, result.AccountSignedBy)
	t.NotEmpty(caughtAccountJWT)

	parsedClaims, err := jwt.DecodeAccountClaims(caughtAccountJWT)
	t.NoError(err)
	t.Equal(result.AccountID, parsedClaims.Subject)
	t.Equal(newOpSignKeyPublic, parsedClaims.Issuer)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenAccountSecretsAreMissing() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := "ACMISSINGACCOUNTID"

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, accountID)

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "account secrets not found for account ACMISSINGACCOUNTID")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenUpdatingSystemAccount() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := t.sauCreds.AccountID
	accountRootKey, _ := nkeys.CreateAccount()
	accountSignKey, _ := nkeys.CreateAccount()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{},
		},
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "reconciling system account is not supported")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenAccountClaimsAreInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	importAccountKey, _ := nkeys.CreateAccount()
	importAccountID, _ := importAccountKey.PublicKey()
	importAccountRef := domain.NewNamespacedName("import-namespace", "import-account")

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.accountReaderMock.mockGet(t.ctx, importAccountRef, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "import-namespace",
			Name:      "import-account",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): importAccountID,
			},
		},
	}).Once()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, domain.AccountResources{
		Account: v1alpha1.Account{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "account-namespace",
				Name:      "account-name",
				Labels: map[string]string{
					string(v1alpha1.AccountLabelAccountID): accountID,
				},
			},
			Spec: v1alpha1.AccountSpec{
				Imports: v1alpha1.Imports{
					{
						Name: "import-once",
						AccountRef: v1alpha1.AccountRef{
							Namespace: "import-namespace",
							Name:      "import-account",
						},
						Subject: "foo",
						Type:    v1alpha1.Service,
					},
					{
						Name: "import-twice",
						AccountRef: v1alpha1.AccountRef{
							Namespace: "import-namespace",
							Name:      "import-account",
						},
						Subject: "foo",
						Type:    v1alpha1.Service,
					},
				},
			},
		},
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "failed to add inline imports")
	t.ErrorContains(err, "failed to add import \"import-twice\"")
}

func (t *AccountManagerTestSuite) Test_Import_ShouldSucceed() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	accountSignKeyPublic, _ := accountSignKey.PublicKey()

	existingNatsLimitsSubs := int64(100)
	existingClaims, err := newAccountClaimsBuilder(accountID, nil).
		natsLimits(&v1alpha1.NatsLimits{Subs: &existingNatsLimitsSubs}).
		signingKey(accountSignKeyPublic).
		build()
	t.NoError(err, "failed to build existing account claims")
	existingJWT, err := existingClaims.Encode(accountSignKey)
	t.NoError(err, "failed to encode existing account JWT")

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Import(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(accountID, result.AccountID)
	t.Equal(accountSignKeyPublic, result.AccountSignedBy)
	t.Equal(existingNatsLimitsSubs, *result.Claims.NatsLimits.Subs)
}

func (t *AccountManagerTestSuite) Test_Delete_ShouldSucceed() {
	// Given
	var (
		caughtDeleteJWT    string
		caughtNatsAccCreds *domain.NatsUserCreds
	)
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})

	t.natsAccClientMock.mockConnectMatchingCreds(t.natsURL, func(userCreds domain.NatsUserCreds) bool {
		if userCreds.AccountID == accountID {
			caughtNatsAccCreds = &userCreds
			return true
		}
		return false
	}, t.natsAccConnMock).Once()
	t.natsAccConnMock.mockListAccountStreams([]string{}).Once()
	t.natsAccConnMock.mockDisconnect().Once()

	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock).Once()
	t.natsSysConnMock.mockDeleteAccountJWTCatch(func(jwt string) { caughtDeleteJWT = jwt }).Once()
	t.natsSysConnMock.mockDisconnect().Once()
	t.secretManagerMock.mockDeleteAll(t.ctx, accountRef, accountID).Once()

	// When
	err := t.unitUnderTest.Delete(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	})

	// Then
	t.Require().NoError(err)

	t.Require().NotNil(caughtNatsAccCreds, "expected to connect to NATS account with credentials for account ID "+accountID)
	t.Equal(accountID, caughtNatsAccCreds.AccountID)
	t.NotEmpty(caughtNatsAccCreds.Creds, "expected credentials for account to be non-empty")
	accUserJWT, err := jwt.ParseDecoratedJWT(caughtNatsAccCreds.Creds)
	t.Require().NoError(err)
	accUserClaims, err := jwt.DecodeUserClaims(accUserJWT)
	t.Require().NoError(err)
	t.Equal(accountID, accUserClaims.Issuer, "expected account user JWT to be issued by the account root key")
	t.Equal(accountID, accUserClaims.IssuerAccount)
	t.Equal(jwt.StringList{"$JS.API.>"}, accUserClaims.Pub.Allow)
	t.Equal(jwt.StringList{"_INBOX.>"}, accUserClaims.Sub.Allow)

	t.Require().NotEmpty(caughtDeleteJWT, "expected deletion JWT to be published to NATS")
	deleteClaims, err := jwt.DecodeGeneric(caughtDeleteJWT)
	t.Require().NoError(err, "failed to decode deletion JWT")
	t.Equal([]interface{}{accountID}, deleteClaims.Data["accounts"])
}

func (t *AccountManagerTestSuite) Test_Delete_ShouldSucceed_WhenAccountSecretsAreMissing() {
	// Given
	var caughtDeleteJWT string
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, accountID)
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock).Once()
	t.natsSysConnMock.mockDeleteAccountJWTCatch(func(jwt string) { caughtDeleteJWT = jwt }).Once()
	t.natsSysConnMock.mockDisconnect().Once()
	t.secretManagerMock.mockDeleteAll(t.ctx, accountRef, accountID).Once()

	// When
	err := t.unitUnderTest.Delete(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotEmpty(caughtDeleteJWT, "expected deletion JWT to be published to NATS")
}

func (t *AccountManagerTestSuite) Test_signAccountJWT_ShouldFailWhenInvalidClaims() {
	// Given
	acRoot, _ := nkeys.CreateAccount()
	acPub, _ := acRoot.PublicKey()
	opSign, _ := nkeys.CreateOperator()
	claims := jwt.NewAccountClaims(acPub)

	acOtherRoot, _ := nkeys.CreateAccount()
	acOtherPub, _ := acOtherRoot.PublicKey()
	claims.Imports.Add(&jwt.Import{
		Name:    "import-once",
		Type:    jwt.Service,
		Subject: "foo",
		Account: acOtherPub,
	})
	claims.Imports.Add(&jwt.Import{
		Name:    "import-twice",
		Type:    jwt.Service,
		Subject: "foo",
		Account: acOtherPub,
	})

	// When
	accountJWT, err := signAccountJWT(claims, opSign)

	// Then
	t.Empty(accountJWT)
	t.ErrorContains(err, "account claims validation failed: [overlapping subject namespace for \"foo\" and \"foo\"")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldSucceed() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	accountSignKeyPublic, _ := accountSignKey.PublicKey()

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}
	t.accountReaderMock.mockGet(t.ctx, accountRef, account)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})

	userKey, _ := nkeys.CreateUser()
	userKeyPublic, _ := userKey.PublicKey()
	claims := jwt.NewUserClaims(userKeyPublic)

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, accountRef, claims)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(accountID, result.AccountID)
	t.Equal(accountSignKeyPublic, result.SignedBy)

	// Verify the JWT is signed with the account's signing key
	parsedClaims, err := jwt.DecodeUserClaims(result.UserJWT)
	t.NoError(err, "failed to decode signed user JWT")
	t.Equal(accountID, parsedClaims.IssuerAccount)
	t.Equal(accountSignKeyPublic, parsedClaims.Issuer)
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenAccountIsNotReady() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
		},
	}
	t.accountReaderMock.mockGet(t.ctx, accountRef, account)

	userKey, _ := nkeys.CreateUser()
	userKeyPublic, _ := userKey.PublicKey()
	claims := jwt.NewUserClaims(userKeyPublic)

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, accountRef, claims)

	// Then
	t.Nil(result)
	t.ErrorContains(err, "account ID is missing for account account-namespace/account-name during user JWT signing")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsIssuerAccountDoesNotMatchFoundAccountID() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}
	t.accountReaderMock.mockGet(t.ctx, accountRef, account)

	userKey, _ := nkeys.CreateUser()
	userKeyPublic, _ := userKey.PublicKey()
	claims := jwt.NewUserClaims(userKeyPublic)
	claims.IssuerAccount = "some-other-account-id"

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, accountRef, claims)

	// Then
	t.Nil(result)
	t.ErrorContains(err, "claims issuer account ID some-other-account-id does not match "+
		accountID+" bound to account \"account-namespace/account-name\" during user JWT signing")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsValidationFails() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}
	t.accountReaderMock.mockGet(t.ctx, accountRef, account)

	userKey, _ := nkeys.CreateUser()
	userKeyPublic, _ := userKey.PublicKey()
	claims := jwt.NewUserClaims(userKeyPublic)
	claims.Locale = "funky-BUSINESS"

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, accountRef, claims)

	// Then
	t.Nil(result)
	t.ErrorContains(err, "claims validation failed during user JWT signing: [could not parse iana time zone by name")
}

/* ****************************************************
* Helpers
*****************************************************/

func (t *AccountManagerTestSuite) verifyAccountResult(result *domain.AccountResult, caughtAccountJWT string, expectRootKey, expectSignKey nkeys.KeyPair) *jwt.AccountClaims {
	t.Require().NotEmpty(caughtAccountJWT, "caught Account JWT must not be empty")

	rootKeyPublic, err := expectRootKey.PublicKey()
	t.NoError(err, "failed to get public key from expect root key pair")
	signKeyPublic, err := expectSignKey.PublicKey()
	t.NoError(err, "failed to get public key from expect signing key pair")

	t.NotNil(result)
	t.NotEmpty(result.AccountID)
	t.Equal(result.AccountID, rootKeyPublic)
	t.Equal(t.keys.OpSign.PublicKey, result.AccountSignedBy)
	t.NotEmpty(result.ClaimsHash)

	accountClaims, err := jwt.DecodeAccountClaims(caughtAccountJWT)
	t.NoError(err, "failed to decode caught account JWT")

	t.Equal(t.keys.OpSign.PublicKey, accountClaims.Issuer)
	t.Equal(result.AccountID, accountClaims.Subject)

	t.Equal([]string{signKeyPublic}, accountClaims.SigningKeys.Keys(), "account claims should contain the expected signing key")

	return accountClaims
}

/* ****************************************************
* clusterTargetResolver Mock
*****************************************************/
type clusterTargetResolverMock struct {
	mock.Mock
}

func newClusterTargetResolverMock() *clusterTargetResolverMock {
	return &clusterTargetResolverMock{}
}

func (m *clusterTargetResolverMock) GetClusterTarget(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef) (*clusterTarget, error) {
	args := m.Called(ctx, accountClusterRef)
	return args.Get(0).(*clusterTarget), args.Error(1)
}

func (m *clusterTargetResolverMock) mockGetClusterTarget(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef, result *clusterTarget) {
	m.On("GetClusterTarget", ctx, accountClusterRef).Return(result, nil)
}

func (m *clusterTargetResolverMock) mockGetClusterTargetError(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef, err error) {
	m.On("GetClusterTarget", ctx, accountClusterRef).Return((*clusterTarget)(nil), err)
}

var _ clusterTargetResolver = (*clusterTargetResolverMock)(nil)

/* ****************************************************
* secretManager Mock
*****************************************************/

type secretManagerMock struct {
	mock.Mock
}

func newSecretManagerMock() *secretManagerMock {
	return &secretManagerMock{}
}

func (m *secretManagerMock) ApplyRootSecret(ctx context.Context, accountRef domain.NamespacedName, rootKeyPair nkeys.KeyPair) error {
	args := m.Called(ctx, accountRef, rootKeyPair)
	return args.Error(0)
}

func (m *secretManagerMock) mockApplyRootSecretUnknown(ctx context.Context, accountRef domain.NamespacedName, catch func(rootKeyPair nkeys.KeyPair)) {
	m.On("ApplyRootSecret", ctx, accountRef, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if catch != nil {
				catch(args.Get(2).(nkeys.KeyPair))
			}
		})
}

func (m *secretManagerMock) ApplySignSecret(ctx context.Context, accountRef domain.NamespacedName, accountID string, signKeyPair nkeys.KeyPair) error {
	args := m.Called(ctx, accountRef, accountID, signKeyPair)
	return args.Error(0)
}

func (m *secretManagerMock) mockApplySignSecretUnknown(ctx context.Context, accountRef domain.NamespacedName, catch func(accountID string, signKeyPair nkeys.KeyPair)) {
	m.On("ApplySignSecret", ctx, accountRef, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if catch != nil {
				catch(args.String(2), args.Get(3).(nkeys.KeyPair))
			}
		})
}

func (m *secretManagerMock) DeleteAll(ctx context.Context, accountRef domain.NamespacedName, accountID string) error {
	args := m.Called(ctx, accountRef, accountID)
	return args.Error(0)
}

func (m *secretManagerMock) mockDeleteAll(ctx context.Context, accountRef domain.NamespacedName, accountID string) *mock.Call {
	return m.On("DeleteAll", ctx, accountRef, accountID).Return(nil)
}

func (m *secretManagerMock) GetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string) (*Secrets, bool, error) {
	args := m.Called(ctx, accountRef, accountID)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*Secrets), args.Bool(1), args.Error(2)
}

func (m *secretManagerMock) mockGetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string, result *Secrets) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(result, true, nil)
}

func (m *secretManagerMock) mockGetSecretsFoundError(ctx context.Context, accountRef domain.NamespacedName, accountID string, err error) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(nil, true, err)
}

func (m *secretManagerMock) mockGetSecretsMissing(ctx context.Context, accountRef domain.NamespacedName, accountID string) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(nil, false, nil)
}

var _ secretManager = (*secretManagerMock)(nil)
