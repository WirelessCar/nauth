package core

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	approvals "github.com/approvals/go-approval-tests"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/yaml"
)

type AccountManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	sauCreds      domain.NatsUserCreds
	natsURL       string
	clusterRef    nauth.ClusterRef
	clusterTarget nauth.ClusterTarget

	accountIDReaderMock *AccountIDReaderMock
	natsSysClientMock   *NatsSysClientMock
	natsSysConnMock     *NatsSysConnectionMock
	natsAccClientMock   *NatsAccountClientMock
	natsAccConnMock     *NatsAccConnectionMock
	secretManagerMock   *secretManagerMock

	unitUnderTest *AccountManager
}

const testAccountReconciliationInterval = time.Minute

func (t *AccountManagerTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.sauCreds = domain.NatsUserCreds{
		Creds:     []byte("FAKE_CREDENTIALS"),
		AccountID: "FAKE_SYS_ACCOUNT_ID",
	}
	t.natsURL = "nats://nats:4222"
	t.clusterRef = "account-namespace/account-namespace-cluster"
	t.clusterTarget = nauth.ClusterTarget{
		UID:                "cluster-target-uid",
		NatsURL:            t.natsURL,
		OperatorSigningKey: testutil.NatsTestOperatorA.Sign.Key,
		SystemAdminCreds:   t.sauCreds,
	}

	t.secretManagerMock = newSecretManagerMock()
	t.accountIDReaderMock = NewAccountIDReaderMock()
	t.natsSysClientMock = NewNatsSysClientMock()
	t.natsSysConnMock = NewNatsSysConnectionMock()
	t.natsAccClientMock = NewNatsAccountClientMock()
	t.natsAccConnMock = NewNatsAccountConnectionMock()

	var err error
	t.unitUnderTest, err = newAccountManager(
		t.natsSysClientMock,
		t.natsAccClientMock,
		t.accountIDReaderMock,
		t.secretManagerMock,
		testAccountReconciliationInterval,
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
	t.secretManagerMock.AssertExpectations(t.T())
	t.accountIDReaderMock.AssertExpectations(t.T())
	t.natsSysClientMock.AssertExpectations(t.T())
	t.natsSysConnMock.AssertExpectations(t.T())
	t.natsAccClientMock.AssertExpectations(t.T())
	t.natsAccConnMock.AssertExpectations(t.T())
}

func (t *AccountManagerTestSuite) resetAllMocks() {
	t.secretManagerMock.Mock = mock.Mock{}
	t.accountIDReaderMock.Mock = mock.Mock{}
	t.natsSysClientMock.Mock = mock.Mock{}
	t.natsSysConnMock.Mock = mock.Mock{}
	t.natsAccClientMock.Mock = mock.Mock{}
	t.natsAccConnMock.Mock = mock.Mock{}
}

func TestAccountManager_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountManagerTestSuite))
}

func TestAccountValidationOutcome(t *testing.T) {
	tests := []struct {
		name  string
		state domain.NatsAccountState
		want  nauth.AccountValidationOutcome
	}{
		{
			name: "complete state with matching claims hash is ready",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateComplete,
				ClaimsHash: "claims-hash",
			},
			want: nauth.AccountValidationReady,
		},
		{
			name: "incomplete state is pending",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateIncomplete,
				ClaimsHash: "claims-hash",
			},
			want: nauth.AccountValidationPending,
		},
		{
			name: "complete state with mismatching claims hash is pending",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateComplete,
				ClaimsHash: "observed-hash",
			},
			want: nauth.AccountValidationPending,
		},
		{
			name: "unknown state is unknown",
			state: domain.NatsAccountState{
				Status: domain.NatsAccountStateUnknown,
			},
			want: nauth.AccountValidationUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountValidationOutcome(tt.state, "claims-hash"); got != tt.want {
				t.Fatalf("expected validation outcome %q, got %q", tt.want, got)
			}
		})
	}
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
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		ClusterTarget: t.clusterTarget,
		NatsLimits: &nauth.NatsLimits{
			Subs: &natsLimitsSubs,
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
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		ClusterTarget: t.clusterTarget,
		NatsLimits: &nauth.NatsLimits{
			Subs: &natsLimitsSubs,
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
	var natsLimitsSubs int64 = 100

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, "", &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		ClusterTarget: t.clusterTarget,
		NatsLimits: &nauth.NatsLimits{
			Subs: &natsLimitsSubs,
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, testutil.NatsTestAccountA.Root.Key, testutil.NatsTestAccountA.Sign.Key)

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
			var input nauth.AccountRequest
			t.Require().NoError(yaml.UnmarshalStrict(inputData, &input))
			input.ClusterTarget = t.clusterTarget
			t.Require().NoError(input.Validate())

			t.secretManagerMock.mockGetSecrets(t.ctx, input.AccountRef, "", &Secrets{
				Root: testutil.NatsTestAccountA.Root.Key,
				Sign: testutil.NatsTestAccountA.Sign.Key,
			})
			t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
			var caughtAccountJWT string
			t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
			t.natsSysConnMock.mockCompleteAccountState()
			t.natsSysConnMock.mockDisconnect()

			// When
			result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, input)

			// Then
			t.assertAndResetAllMock()
			t.Require().NoError(err)
			t.Require().NotNil(result)
			t.Require().NotEmpty(caughtAccountJWT)

			t.NotNil(result.Claims)
			t.NotEmpty(result.State.ClaimsHash)
			t.verifyAccountResult(result, caughtAccountJWT, testutil.NatsTestAccountA.Root.Key, testutil.NatsTestAccountA.Sign.Key)

			approvalResult := struct {
				AccountID       string
				AccountSignedBy string
				Claims          *nauth.AccountClaims
				State           struct {
					ClaimsHash string `json:"claimsHash,omitempty"`
				}
				Adoptions *nauth.AccountAdoptions
			}{
				AccountID:       result.AccountID,
				AccountSignedBy: result.AccountSignedBy,
				Claims:          result.Claims,
				Adoptions:       result.Adoptions,
			}
			approvalResult.State.ClaimsHash = result.State.ClaimsHash
			resultYaml, err := yaml.Marshal(approvalResult)
			t.Require().NoError(err)
			approvals.VerifyString(t.T(), string(resultYaml), approvalOptionsForTestSuite(&t.Suite).
				ForFile().WithExtension(".yaml"))
		})
	}
}

func (t *AccountManagerTestSuite) Test_Create_ShouldFail_WhenExistingSecretsAreInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")

	t.secretManagerMock.mockGetSecretsFoundError(t.ctx, accountRef, "", fmt.Errorf("root secret is malformed"))

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		ClusterTarget: t.clusterTarget,
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
	accountID := testutil.NatsTestAccountA.AccountID()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	t.verifyAccountResult(result, caughtAccountJWT, testutil.NatsTestAccountA.Root.Key, testutil.NatsTestAccountA.Sign.Key)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldSkipNATSValidation_WhenStateUnchangedAndValidationFresh() {
	// Given
	accountRef, accountID, initialResult, _ := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})

	// When
	stateValidatedAt := time.Now()
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:         initialResult.State.ClaimsHash,
			ObservedClaimsHash: initialResult.State.ClaimsHash,
			ObservedStatus:     domain.NatsAccountStateComplete,
			StateValidatedAt:   stateValidatedAt,
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(initialResult.State.ClaimsHash, result.State.ClaimsHash)
	t.Equal(stateValidatedAt, result.State.StateValidatedAt)
	t.Equal(nauth.AccountValidationReady, result.ValidationOutcome)
	t.Nil(result.NatsState)
	t.natsSysConnMock.AssertNotCalled(t.T(), "RequestAccountLoad", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldRequestAccountLoadBeforeStateObservation() {
	// Given
	accountRef, accountID, _, _ := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	uploadCall := t.natsSysConnMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()
	loadCall := t.natsSysConnMock.On("RequestAccountLoad", accountID).Return(nil).Once().NotBefore(uploadCall)
	t.natsSysConnMock.On("LookupAccountState", accountID).Return(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		AccountID:  accountID,
		ClaimsHash: "observed-claims-hash",
	}, nil).Once().NotBefore(loadCall)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(domain.NatsAccountStateComplete, result.NatsState.Status)
	t.Equal(nauth.AccountValidationPending, result.ValidationOutcome)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldReturnUnknown_WhenAccountLoadCannotBeRequested() {
	// Given
	accountRef, accountID, _, _ := t.createExistingAccountForValidation()
	loadErr := fmt.Errorf("NATS account load request failed")

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	uploadCall := t.natsSysConnMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()
	t.natsSysConnMock.On("RequestAccountLoad", accountID).Return(loadErr).Once().NotBefore(uploadCall)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(domain.NatsAccountStateUnknown, result.NatsState.Status)
	t.Equal("failed to request runtime Account load: NATS account load request failed", result.NatsObservationMessage)
	t.Equal(nauth.AccountValidationUnknown, result.ValidationOutcome)
	t.Zero(result.State.StateValidatedAt)
	t.natsSysConnMock.AssertNotCalled(t.T(), "LookupAccountState", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldReuploadMatchingAccountJWT_WhenLastObservationIncomplete() {
	// Given
	accountRef, accountID, initialResult, existingJWT := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	lookupJWTCall := t.natsSysConnMock.On("LookupAccountJWT", accountID).Return(existingJWT, nil).Once()
	var uploadedJWT string
	uploadCall := t.natsSysConnMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once().
		Run(func(args mock.Arguments) { uploadedJWT = args.String(0) }).
		NotBefore(lookupJWTCall)
	loadCall := t.natsSysConnMock.On("RequestAccountLoad", accountID).Return(nil).Once().NotBefore(uploadCall)
	t.natsSysConnMock.On("LookupAccountState", accountID).Return(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		ServerID:   "server-b",
		AccountID:  accountID,
		ClaimsHash: initialResult.State.ClaimsHash,
	}, nil).Once().NotBefore(loadCall)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:         initialResult.State.ClaimsHash,
			ObservedClaimsHash: initialResult.State.ClaimsHash,
			ObservedStatus:     domain.NatsAccountStateIncomplete,
			StateValidatedAt:   time.Now(),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(domain.NatsAccountStateComplete, result.NatsState.Status)
	t.Equal(nauth.AccountValidationReady, result.ValidationOutcome)
	t.Equal("server-b", result.State.ObservedServerID)
	uploadedClaimsHash, err := domain.HashNatsAccountJWTClaims(uploadedJWT)
	t.Require().NoError(err)
	t.Equal(initialResult.State.ClaimsHash, uploadedClaimsHash)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldNotUploadDuringReconciliationBurst_WhenStateUnchanged() {
	// Given
	accountRef, accountID, initialResult, _ := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	}).Times(10)

	stateValidatedAt := time.Now()

	// When: the same Account is reconciled repeatedly during a burst.
	for i := 0; i < 10; i++ {
		result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
			AccountRef: accountRef,
			AccountID:  nauth.AccountID(accountID),
			State: nauth.AccountState{
				ClaimsHash:         initialResult.State.ClaimsHash,
				ObservedClaimsHash: initialResult.State.ClaimsHash,
				ObservedStatus:     domain.NatsAccountStateComplete,
				StateValidatedAt:   stateValidatedAt,
			},
			ClusterTarget: t.clusterTarget,
		})

		t.NoError(err)
		t.NotNil(result)
		t.Equal(initialResult.State.ClaimsHash, result.State.ClaimsHash)
		t.Equal(stateValidatedAt, result.State.StateValidatedAt)
	}

	// Then: the initial upload performed by the helper is the only upload.
	t.natsSysClientMock.AssertNotCalled(t.T(), "Connect", mock.Anything, mock.Anything)
	t.natsSysConnMock.AssertNotCalled(t.T(), "UploadAccountJWT", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldNotUploadDuringRepeatedChildResourceReconciliations_WhenEffectiveStateUnchanged() {
	// Given: an Account whose desired state includes claims derived from a child resource.
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.NatsTestAccountA.AccountID()
	childExportGroups := func() nauth.ExportGroups {
		return nauth.ExportGroups{
			{
				Ref:  "child-export",
				Name: "orders-export",
				Exports: nauth.Exports{
					{
						Name:    "orders",
						Subject: "orders",
						Type:    nauth.ExportTypeStream,
					},
				},
			},
		}
	}
	request := nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
		ExportGroups:  childExportGroups(),
	}

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(_ string) {})
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	initialResult, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)
	t.Require().NoError(err)
	t.Require().NotNil(initialResult)
	t.Require().NotEmpty(initialResult.State.ClaimsHash)
	t.assertAndResetAllMock()

	// When: repeated child-resource reconciliations produce the same effective Account state.
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	}).Times(10)
	stateValidatedAt := time.Now()
	for i := 0; i < 10; i++ {
		repeatedRequest := request
		repeatedRequest.ExportGroups = childExportGroups()
		repeatedRequest.State.ClaimsHash = initialResult.State.ClaimsHash
		repeatedRequest.State.ObservedClaimsHash = initialResult.State.ClaimsHash
		repeatedRequest.State.ObservedStatus = domain.NatsAccountStateComplete
		repeatedRequest.State.StateValidatedAt = stateValidatedAt

		result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, repeatedRequest)

		// Then: every reconciliation keeps the state unchanged and skips NATS validation.
		t.NoError(err)
		t.NotNil(result)
		t.Equal(initialResult.State.ClaimsHash, result.State.ClaimsHash)
		t.Equal(stateValidatedAt, result.State.StateValidatedAt)
	}

	t.natsSysClientMock.AssertNotCalled(t.T(), "Connect", mock.Anything, mock.Anything)
	t.natsSysConnMock.AssertNotCalled(t.T(), "UploadAccountJWT", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldUseConfiguredAccountReconciliationInterval() {
	// Given
	accountRef, accountID, initialResult, existingJWT := t.createExistingAccountForValidation()
	t.unitUnderTest.accountReconciliationInterval = time.Minute

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsSysConnMock.mockLookupAccountState(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		AccountID:  accountID,
		ClaimsHash: initialResult.State.ClaimsHash,
	}, nil)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now().Add(-2 * time.Minute),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.False(result.State.StateValidatedAt.IsZero())
}

func (t *AccountManagerTestSuite) Test_Update_ShouldValidateRemoteAccountState_WhenValidationExpired() {
	// Given
	accountRef, accountID, initialResult, existingJWT := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsSysConnMock.mockLookupAccountState(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		AccountID:  accountID,
		ClaimsHash: initialResult.State.ClaimsHash,
	}, nil)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(initialResult.State.ClaimsHash, result.State.ClaimsHash)
	t.False(result.State.StateValidatedAt.IsZero())
	t.natsSysConnMock.AssertNotCalled(t.T(), "UploadAccountJWT", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldRefreshObservedServerIdentity_WhenValidationExpires() {
	// Given
	accountRef, accountID, initialResult, existingJWT := t.createExistingAccountForValidation()
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsSysConnMock.mockLookupAccountState(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		ServerID:   "server-b",
		AccountID:  accountID,
		ClaimsHash: initialResult.State.ClaimsHash,
	}, nil)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:         initialResult.State.ClaimsHash,
			ObservedServerID:   "server-a",
			ObservedClaimsHash: initialResult.State.ClaimsHash,
			StateValidatedAt:   time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal("server-b", result.State.ObservedServerID)
	t.Equal(initialResult.State.ClaimsHash, result.State.ObservedClaimsHash)
}

func (t *AccountManagerTestSuite) Test_NewAccountManager_ShouldRejectNonPositiveAccountReconciliationInterval() {
	// When
	result, err := newAccountManager(
		t.natsSysClientMock,
		t.natsAccClientMock,
		t.accountIDReaderMock,
		t.secretManagerMock,
		0,
	)

	// Then
	t.Nil(result)
	t.ErrorContains(err, "accountReconciliationInterval must be greater than zero")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldRepairMissingRemoteAccountState_WhenValidationExpired() {
	// Given
	accountRef, accountID, initialResult, _ := t.createExistingAccountForValidation()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, "")
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(_ string) {})
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.False(result.State.StateValidatedAt.IsZero())
}

func (t *AccountManagerTestSuite) Test_Update_ShouldRepairDriftedRemoteAccountState_WhenValidationExpired() {
	// Given
	accountRef, accountID, initialResult, _ := t.createExistingAccountForValidation()
	var uploadedJWT string
	remoteClaims, err := newAccountClaimsBuilder(accountID, nil).
		displayName("drifted-account").
		signingKey(testutil.NatsTestAccountA.Sign.PublicKey).
		build()
	t.Require().NoError(err)
	remoteJWT, err := signAccountJWT(remoteClaims, t.clusterTarget.OperatorSigningKey)
	t.Require().NoError(err)

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, remoteJWT)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { uploadedJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.False(result.State.StateValidatedAt.IsZero())
	t.Require().NotEmpty(uploadedJWT)
	uploadedClaimsHash, err := domain.HashNatsAccountJWTClaims(uploadedJWT)
	t.Require().NoError(err)
	t.Equal(initialResult.State.ClaimsHash, uploadedClaimsHash)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldSurfaceRemoteStateValidationFailure() {
	// Given
	accountRef, accountID, initialResult, _ := t.createExistingAccountForValidation()
	validationErr := fmt.Errorf("NATS lookup unavailable")

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.On("LookupAccountJWT", accountID).Return("", validationErr)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "failed to validate account jwt in NATS")
	t.ErrorIs(err, validationErr)
	t.natsSysConnMock.AssertNotCalled(t.T(), "UploadAccountJWT", mock.Anything)
}

func (t *AccountManagerTestSuite) Test_Update_ShouldReturnUnknownObservation_WhenAccountCompletenessCannotBeEstablished() {
	// Given
	accountRef, accountID, initialResult, existingJWT := t.createExistingAccountForValidation()
	observationErr := fmt.Errorf("ACCOUNTZ is unavailable on connected NATS server")

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsSysConnMock.mockLookupAccountState(domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}, observationErr)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: accountRef,
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:         initialResult.State.ClaimsHash,
			ObservedClaimsHash: initialResult.State.ClaimsHash,
			StateValidatedAt:   time.Now().Add(-testAccountReconciliationInterval - time.Second),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(domain.NatsAccountStateUnknown, result.NatsState.Status)
	t.Equal(observationErr.Error(), result.NatsObservationMessage)
	t.Equal(nauth.AccountValidationUnknown, result.ValidationOutcome)
	t.Equal(initialResult.State.ClaimsHash, result.State.ClaimsHash)
	t.Zero(result.State.StateValidatedAt)
	t.Empty(result.State.ObservedServerID)
	t.Empty(result.State.ObservedClaimsHash)
}

func (t *AccountManagerTestSuite) createExistingAccountForValidation() (domain.NamespacedName, string, *nauth.AccountResult, string) {
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.NatsTestAccountA.AccountID()
	var existingJWT string

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { existingJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Require().NotEmpty(result.State.ClaimsHash)
	t.Require().NotEmpty(existingJWT)
	t.assertAndResetAllMock()

	return accountRef, accountID, result, existingJWT
}

func (t *AccountManagerTestSuite) Test_Update_ShouldUploadNewAccountJWT_WhenOperatorSigningKeyHashChanged() {
	// Given
	var caughtAccountJWT string
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.NatsTestAccountA.AccountID()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) {})
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	initialResult, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})
	t.Require().NoError(err)
	t.Require().NotNil(initialResult)
	t.Require().NotEmpty(initialResult.State.ClaimsHash)
	t.assertAndResetAllMock()

	newOpSignKey := testutil.CreateNatsTestOperatorKey()
	t.clusterTarget.OperatorSigningKey = newOpSignKey.Key

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef: domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:  nauth.AccountID(accountID),
		State: nauth.AccountState{
			ClaimsHash:       initialResult.State.ClaimsHash,
			StateValidatedAt: time.Now(),
		},
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.NotEqual(initialResult.State.ClaimsHash, result.State.ClaimsHash)
	t.Equal(newOpSignKey.PublicKey, result.AccountSignedBy)
	t.NotEmpty(caughtAccountJWT)

	parsedClaims, err := jwt.DecodeAccountClaims(caughtAccountJWT)
	t.NoError(err)
	t.Equal(result.AccountID, parsedClaims.Subject)
	t.Equal(newOpSignKey.PublicKey, parsedClaims.Issuer)
}

func (t *AccountManagerTestSuite) Test_CreateOrUpdate_WithSigningKeys_NewAccount_ShouldIncludeImplicitAndExplicitKeys() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	explicitKey1 := testutil.CreateNatsTestAccountKey().PublicKey
	explicitKey2 := testutil.CreateNatsTestAccountKey().PublicKey

	var capturedSignKP nkeys.KeyPair
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, "")
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, nil)
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(_ string, kp nkeys.KeyPair) {
		capturedSignKP = kp
	})
	var capturedJWT string
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { capturedJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		ClusterTarget: t.clusterTarget,
		SigningKeys:   []string{explicitKey1, explicitKey2},
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)

	signPub, _ := capturedSignKP.PublicKey()
	claims, decodeErr := jwt.DecodeAccountClaims(capturedJWT)
	t.Require().NoError(decodeErr)
	t.ElementsMatch([]string{signPub, explicitKey1, explicitKey2}, claims.SigningKeys.Keys(),
		"JWT must contain implicit signing key plus all explicit signing keys")
}

func (t *AccountManagerTestSuite) Test_CreateOrUpdate_AddSigningKeys_ShouldIncludeImplicitAndExplicitKeysInJWT() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.NatsTestAccountA.AccountID()
	explicitKey := testutil.CreateNatsTestAccountKey().PublicKey

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	var capturedJWT string
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { capturedJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
		SigningKeys:   []string{explicitKey},
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.secretManagerMock.AssertNotCalled(t.T(), "ApplyRootSecret", mock.Anything, mock.Anything, mock.Anything)
	t.secretManagerMock.AssertNotCalled(t.T(), "ApplySignSecret", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	claims, decodeErr := jwt.DecodeAccountClaims(capturedJWT)
	t.Require().NoError(decodeErr)
	t.ElementsMatch(
		[]string{testutil.NatsTestAccountA.Sign.PublicKey, explicitKey},
		claims.SigningKeys.Keys(),
		"JWT must contain implicit signing key plus the explicit signing key",
	)
}

func (t *AccountManagerTestSuite) Test_Create_ImplicitMode_ShouldCreateRootAndSignSecret() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")

	var capturedSignKP nkeys.KeyPair
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, "")
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, nil)
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(_ string, kp nkeys.KeyPair) {
		capturedSignKP = kp
	})
	var capturedJWT string
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { capturedJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		ClusterTarget: t.clusterTarget,
		SigningKeys:   nil, // implicit mode
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)

	signPub, _ := capturedSignKP.PublicKey()
	claims, decodeErr := jwt.DecodeAccountClaims(capturedJWT)
	t.Require().NoError(decodeErr)
	t.Equal([]string{signPub}, claims.SigningKeys.Keys(), "JWT must contain only the implicit signing key")
}

func (t *AccountManagerTestSuite) Test_CreateOrUpdate_RemoveSigningKeys_OnlyImplicitKeyRemainsInJWT() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.NatsTestAccountA.AccountID()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: testutil.NatsTestAccountA.Root.Key,
		Sign: testutil.NatsTestAccountA.Sign.Key,
	})
	var capturedJWT string
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockUploadAccountJWTCatch(func(jwt string) { capturedJWT = jwt })
	t.natsSysConnMock.mockCompleteAccountState()
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    accountRef,
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
		SigningKeys:   nil,
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.secretManagerMock.AssertNotCalled(t.T(), "ApplyRootSecret", mock.Anything, mock.Anything, mock.Anything)
	t.secretManagerMock.AssertNotCalled(t.T(), "ApplySignSecret", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	claims, decodeErr := jwt.DecodeAccountClaims(capturedJWT)
	t.Require().NoError(decodeErr)
	t.Equal([]string{testutil.NatsTestAccountA.Sign.PublicKey}, claims.SigningKeys.Keys(),
		"JWT must contain only the implicit signing key when signingKeyRefs is removed")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenAccountSecretsAreMissing() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := "ACMISSINGACCOUNTID"

	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, accountID)

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "account secrets not found for account ACMISSINGACCOUNTID")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenUpdatingSystemAccount() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := t.sauCreds.AccountID
	account := testutil.CreateNatsTestAccount()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(accountID),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "reconciling system account is not supported")
}

func (t *AccountManagerTestSuite) Test_Update_ShouldFail_WhenAccountClaimsAreInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()
	importAccount := testutil.CreateNatsTestAccount()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, account.AccountID(), &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountRequest{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(account.AccountID()),
		ClusterTarget: t.clusterTarget,
		ImportGroups: nauth.ImportGroups{
			{
				Ref:      "inline",
				Required: true,
				Imports: nauth.Imports{
					{
						AccountID: nauth.AccountID(importAccount.AccountID()),
						Name:      "import-once",
						Subject:   "foo",
						Type:      nauth.ExportTypeService,
					},
					{
						AccountID: nauth.AccountID(importAccount.AccountID()),
						Name:      "import-twice",
						Subject:   "foo",
						Type:      nauth.ExportTypeService,
					},
				},
			},
		},
	})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "failed to include required import group")
	t.ErrorContains(err, "overlapping subject namespace for \"foo\" and \"foo\"")
}

func (t *AccountManagerTestSuite) Test_Import_ShouldSucceed() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()

	existingNatsLimitsSubs := int64(100)
	existingClaims, err := newAccountClaimsBuilder(account.AccountID(), nil).
		natsLimits(&nauth.NatsLimits{Subs: &existingNatsLimitsSubs}).
		signingKey(account.Sign.PublicKey).
		build()
	t.NoError(err, "failed to build existing account claims")
	existingJWT, err := existingClaims.Encode(account.Sign.Key)
	t.NoError(err, "failed to encode existing account JWT")

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, account.AccountID(), &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	})
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock)
	t.natsSysConnMock.mockLookupAccountJWT(account.AccountID(), existingJWT)
	existingClaimsHash, err := domain.HashNatsAccountJWTClaims(existingJWT)
	t.Require().NoError(err)
	t.natsSysConnMock.mockLookupAccountState(domain.NatsAccountState{
		Status:     domain.NatsAccountStateComplete,
		ServerID:   "server-a",
		AccountID:  account.AccountID(),
		ClaimsHash: existingClaimsHash,
	}, nil)
	t.natsSysConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Import(t.ctx, nauth.AccountReference{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(account.AccountID()),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(account.AccountID(), result.AccountID)
	t.Equal(account.Sign.PublicKey, result.AccountSignedBy)
	t.Equal(existingNatsLimitsSubs, *result.Claims.NatsLimits.Subs)
	t.Equal(domain.NatsAccountStateComplete, result.NatsState.Status)
	t.Equal(existingClaimsHash, result.State.ObservedClaimsHash)
}

func (t *AccountManagerTestSuite) Test_FindAccountID_ShouldReturnIDFromAccountSecrets() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, "", &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	}).Once()

	// When
	result, found, err := t.unitUnderTest.FindAccountID(t.ctx, nauth.AccountReference{
		AccountRef: accountRef,
	})

	// Then
	t.NoError(err)
	t.True(found)
	t.Equal(nauth.AccountID(account.AccountID()), result)
}

func (t *AccountManagerTestSuite) Test_FindAccountID_ShouldReturnNotFoundWhenAccountSecretsAreMissing() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, "")

	// When
	result, found, err := t.unitUnderTest.FindAccountID(t.ctx, nauth.AccountReference{
		AccountRef: accountRef,
	})

	// Then
	t.NoError(err)
	t.False(found)
	t.Empty(result)
}

func (t *AccountManagerTestSuite) Test_FindAccountID_ShouldFailWhenAccountSecretsAreInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	t.secretManagerMock.mockGetSecretsFoundError(t.ctx, accountRef, "", fmt.Errorf("invalid account secrets"))

	// When
	result, found, err := t.unitUnderTest.FindAccountID(t.ctx, nauth.AccountReference{
		AccountRef: accountRef,
	})

	// Then
	t.ErrorContains(err, "failed to get account secrets for account ID lookup: invalid account secrets")
	t.False(found)
	t.Empty(result)
}

func (t *AccountManagerTestSuite) Test_Delete_ShouldSucceed() {
	// Given
	var (
		caughtDeleteJWT    string
		caughtNatsAccCreds *domain.NatsUserCreds
	)
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()

	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, account.AccountID(), &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	})

	t.natsAccClientMock.mockConnectMatchingCreds(t.natsURL, func(userCreds domain.NatsUserCreds) bool {
		if userCreds.AccountID == account.AccountID() {
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
	t.secretManagerMock.mockDeleteAll(t.ctx, accountRef, account.AccountID()).Once()

	// When
	err := t.unitUnderTest.Delete(t.ctx, nauth.AccountReference{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(account.AccountID()),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.Require().NoError(err)

	t.Require().NotNil(caughtNatsAccCreds, "expected to connect to NATS account with credentials for account ID "+account.AccountID())
	t.Equal(account.AccountID(), caughtNatsAccCreds.AccountID)
	t.NotEmpty(caughtNatsAccCreds.Creds, "expected credentials for account to be non-empty")
	accUserJWT, err := jwt.ParseDecoratedJWT(caughtNatsAccCreds.Creds)
	t.Require().NoError(err)
	accUserClaims, err := jwt.DecodeUserClaims(accUserJWT)
	t.Require().NoError(err)
	t.Equal(account.AccountID(), accUserClaims.Issuer, "expected account user JWT to be issued by the account root key")
	t.Equal(account.AccountID(), accUserClaims.IssuerAccount)
	t.Equal(jwt.StringList{"$JS.API.>"}, accUserClaims.Pub.Allow)
	t.Equal(jwt.StringList{"_INBOX.>"}, accUserClaims.Sub.Allow)

	t.Require().NotEmpty(caughtDeleteJWT, "expected deletion JWT to be published to NATS")
	deleteClaims, err := jwt.DecodeGeneric(caughtDeleteJWT)
	t.Require().NoError(err, "failed to decode deletion JWT")
	t.Equal([]interface{}{account.AccountID()}, deleteClaims.Data["accounts"])
}

func (t *AccountManagerTestSuite) Test_Delete_ShouldSucceed_WhenAccountSecretsAreMissing() {
	// Given
	var caughtDeleteJWT string
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()

	t.secretManagerMock.mockGetSecretsMissing(t.ctx, accountRef, account.AccountID())
	t.natsSysClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsSysConnMock).Once()
	t.natsSysConnMock.mockDeleteAccountJWTCatch(func(jwt string) { caughtDeleteJWT = jwt }).Once()
	t.natsSysConnMock.mockDisconnect().Once()
	t.secretManagerMock.mockDeleteAll(t.ctx, accountRef, account.AccountID()).Once()

	// When
	err := t.unitUnderTest.Delete(t.ctx, nauth.AccountReference{
		AccountRef:    domain.NewNamespacedName("account-namespace", "account-name"),
		AccountID:     nauth.AccountID(account.AccountID()),
		ClusterTarget: t.clusterTarget,
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotEmpty(caughtDeleteJWT, "expected deletion JWT to be published to NATS")
}

func (t *AccountManagerTestSuite) Test_signAccountJWT_ShouldFailWhenInvalidClaims() {
	// Given
	ac := testutil.CreateNatsTestAccountKey()
	opSign := testutil.CreateNatsTestOperatorKey()
	claims := jwt.NewAccountClaims(ac.PublicKey)

	acOther := testutil.CreateNatsTestAccountKey()
	claims.Imports.Add(&jwt.Import{
		Name:    "import-once",
		Type:    jwt.Service,
		Subject: "foo",
		Account: acOther.PublicKey,
	})
	claims.Imports.Add(&jwt.Import{
		Name:    "import-twice",
		Type:    jwt.Service,
		Subject: "foo",
		Account: acOther.PublicKey,
	})

	// When
	accountJWT, err := signAccountJWT(claims, opSign.Key)

	// Then
	t.Empty(accountJWT)
	t.ErrorContains(err, "account claims validation failed: [overlapping subject namespace for \"foo\" and \"foo\"")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldSucceed() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()

	t.accountIDReaderMock.mockGetAccountID(t.ctx, accountRef, account.AccountID()).Once()
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, account.AccountID(), &Secrets{
		Root: account.Root.Key,
		Sign: account.Sign.Key,
	}).Once()

	user := testutil.CreateNatsTestUserKey()
	claims := jwt.NewUserClaims(user.PublicKey)

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, UserSigningRequest{AccountRef: accountRef, Claims: claims})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(account.AccountID(), result.AccountID)
	t.Equal(account.Sign.PublicKey, result.SignedBy)

	// Verify the JWT is signed with the account's signing key
	parsedClaims, err := jwt.DecodeUserClaims(result.UserJWT)
	t.NoError(err, "failed to decode signed user JWT")
	t.Equal(account.AccountID(), parsedClaims.IssuerAccount)
	t.Equal(account.Sign.PublicKey, parsedClaims.Issuer)
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenAccountIsNotReady() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")

	t.accountIDReaderMock.mockGetAccountIDError(t.ctx, accountRef, domain.ErrAccountNotReady).Once()

	user := testutil.CreateNatsTestUserKey()
	claims := jwt.NewUserClaims(user.PublicKey)

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, UserSigningRequest{AccountRef: accountRef, Claims: claims})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "failed to lookup Account ID for \"account-namespace/account-name\" during user JWT signing: AccountNotReady")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsIssuerAccountDoesNotMatchFoundAccountID() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	account := testutil.CreateNatsTestAccount()

	t.accountIDReaderMock.mockGetAccountID(t.ctx, accountRef, account.AccountID()).Once()

	user := testutil.CreateNatsTestUserKey()
	claims := jwt.NewUserClaims(user.PublicKey)
	claims.IssuerAccount = "some-other-account-id"

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, UserSigningRequest{AccountRef: accountRef, Claims: claims})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "claims issuer account ID some-other-account-id does not match "+
		account.AccountID()+" bound to account \"account-namespace/account-name\" during user JWT signing")
}

func (t *AccountManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsValidationFails() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountID := testutil.AnyNatsTestAccountID()

	t.accountIDReaderMock.mockGetAccountID(t.ctx, accountRef, accountID).Once()

	user := testutil.CreateNatsTestUserKey()
	claims := jwt.NewUserClaims(user.PublicKey)
	claims.Locale = "funky-BUSINESS"

	// When
	result, err := t.unitUnderTest.SignUserJWT(t.ctx, UserSigningRequest{AccountRef: accountRef, Claims: claims})

	// Then
	t.Nil(result)
	t.ErrorContains(err, "claims validation failed during user JWT signing: [could not parse iana time zone by name")
}

/* ****************************************************
* Helpers
*****************************************************/

func (t *AccountManagerTestSuite) verifyAccountResult(result *nauth.AccountResult, caughtAccountJWT string, expectRootKey, expectSignKey nkeys.KeyPair) *jwt.AccountClaims {
	t.Require().NotEmpty(caughtAccountJWT, "caught Account JWT must not be empty")

	rootKeyPublic, err := expectRootKey.PublicKey()
	t.NoError(err, "failed to get public key from expect root key pair")
	signKeyPublic, err := expectSignKey.PublicKey()
	t.NoError(err, "failed to get public key from expect signing key pair")

	t.NotNil(result)
	t.NotEmpty(result.AccountID)
	t.Equal(result.AccountID, rootKeyPublic)
	t.Equal(testutil.NatsTestOperatorA.Sign.PublicKey, result.AccountSignedBy)
	t.NotEmpty(result.State.ClaimsHash)

	accountClaims, err := jwt.DecodeAccountClaims(caughtAccountJWT)
	t.NoError(err, "failed to decode caught account JWT")

	t.Equal(testutil.NatsTestOperatorA.Sign.PublicKey, accountClaims.Issuer)
	t.Equal(result.AccountID, accountClaims.Subject)

	t.Equal([]string{signKeyPublic}, accountClaims.SigningKeys.Keys(), "account claims should contain the expected signing key")

	return accountClaims
}

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

func (m *secretManagerMock) mockGetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string, result *Secrets) *mock.Call {
	return m.On("GetSecrets", ctx, accountRef, accountID).Return(result, true, nil)
}

func (m *secretManagerMock) mockGetSecretsFoundError(ctx context.Context, accountRef domain.NamespacedName, accountID string, err error) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(nil, true, err)
}

func (m *secretManagerMock) mockGetSecretsMissing(ctx context.Context, accountRef domain.NamespacedName, accountID string) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(nil, false, nil)
}

var _ secretManager = (*secretManagerMock)(nil)
