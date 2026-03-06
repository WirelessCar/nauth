package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	opSignKey       nkeys.KeyPair
	opSignKeyPublic string
	sauCreds        ports.NatsUserCreds
	natsURL         string
	clusterConfig   clusterConfig

	nauthAccountResolverMock *AccountResolverMock
	natsClientMock           *NatsClientMock
	natsConnMock             *NatsConnectionMock
	clusterConfigReaderMock  *clusterConfigReaderMock
	secretManagerMock        *secretManagerMock

	unitUnderTest *Manager
}

func (t *ManagerTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.opSignKey, _ = nkeys.CreateOperator()
	t.opSignKeyPublic, _ = t.opSignKey.PublicKey()
	t.sauCreds = ports.NatsUserCreds{
		Creds:     []byte("FAKE_CREDENTIALS"),
		AccountID: "FAKE_SYS_ACCOUNT_ID",
	}
	t.natsURL = "nats://nats:4222"
	t.clusterConfig = clusterConfig{
		NatsURL:            t.natsURL,
		OperatorSigningKey: t.opSignKey,
		SystemAdminCreds:   t.sauCreds,
	}

	t.clusterConfigReaderMock = newClusterConfigReaderMock()
	t.secretManagerMock = newSecretManagerMock()
	t.nauthAccountResolverMock = NewAccountResolverMock()
	t.natsClientMock = NewNatsClientMock()
	t.natsConnMock = NewNatsConnectionMock()

	var err error
	t.unitUnderTest, err = newManager(
		t.natsClientMock,
		t.nauthAccountResolverMock,
		t.clusterConfigReaderMock,
		t.secretManagerMock,
	)
	t.NoError(err)
}

func (t *ManagerTestSuite) TearDownTest() {
	t.clusterConfigReaderMock.AssertExpectations(t.T())
	t.secretManagerMock.AssertExpectations(t.T())
	t.nauthAccountResolverMock.AssertExpectations(t.T())
	t.natsClientMock.AssertExpectations(t.T())
	t.natsConnMock.AssertExpectations(t.T())
}

func TestManager_TestSuite(t *testing.T) {
	suite.Run(t, new(ManagerTestSuite))
}

func (t *ManagerTestSuite) Test_Create_ShouldSucceed() {
	// Given
	var (
		caughtAccountJWT    string
		caughtRootKeyPair   nkeys.KeyPair
		caughtSignAccountID string
		caughtSignKeyPair   nkeys.KeyPair
	)
	var natsLimitsSubs int64 = 100

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, nil, &t.clusterConfig)
	t.secretManagerMock.mockGetSecretsError(t.ctx, "account-namespace", "account-name", "", fmt.Errorf("no secrets found"))
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, "account-namespace", "account-name", func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, "account-namespace", "account-name", func(accountID string, signKeyPair nkeys.KeyPair) {
		caughtSignAccountID = accountID
		caughtSignKeyPair = signKeyPair
	})
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
		},
		Spec: v1alpha1.AccountSpec{
			NatsLimits: &v1alpha1.NatsLimits{
				Subs: &natsLimitsSubs,
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

func (t *ManagerTestSuite) Test_Create_ShouldSucceed_WhenAccountExplicitCluster() {
	// Given
	var (
		caughtAccountJWT    string
		caughtRootKeyPair   nkeys.KeyPair
		caughtSignAccountID string
		caughtSignKeyPair   nkeys.KeyPair
	)

	natsLimitsSubs := int64(100)

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, &v1alpha1.NatsClusterRef{
		Namespace: "account-namespace",
		Name:      "account-namespace-cluster",
	}, &t.clusterConfig)
	t.secretManagerMock.mockGetSecretsError(t.ctx, "account-namespace", "account-name", "", fmt.Errorf("no secrets found"))
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, "account-namespace", "account-name", func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, "account-namespace", "account-name", func(accountID string, signKeyPair nkeys.KeyPair) {
		caughtSignAccountID = accountID
		caughtSignKeyPair = signKeyPair
	})
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Create(t.ctx, &v1alpha1.Account{
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
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, caughtRootKeyPair, caughtSignKeyPair)

	t.Equal(result.AccountID, caughtSignAccountID)
	t.Equal(natsLimitsSubs, jwtClaims.Limits.Subs)
}

func (t *ManagerTestSuite) Test_Create_ShouldSucceed_WhenSecretsAlreadyExist() {
	// Given
	var (
		caughtAccountJWT string
	)
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	var natsLimitsSubs int64 = 100

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, nil, &t.clusterConfig)
	t.secretManagerMock.mockGetSecrets(t.ctx, "account-namespace", "account-name", "", &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.secretManagerMock.mockApplyRootSecret(t.ctx, "account-namespace", "account-name", accountRootKey)
	t.secretManagerMock.mockApplySignSecret(t.ctx, "account-namespace", "account-name", accountID, accountSignKey)
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
		},
		Spec: v1alpha1.AccountSpec{
			NatsLimits: &v1alpha1.NatsLimits{
				Subs: &natsLimitsSubs,
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	jwtClaims := t.verifyAccountResult(result, caughtAccountJWT, accountRootKey, accountSignKey)

	t.Equal(natsLimitsSubs, jwtClaims.Limits.Subs)
}

func (t *ManagerTestSuite) Test_Create_ShouldFail_WhenClusterNotFound() {
	// Given
	t.clusterConfigReaderMock.mockGetClusterConfigError(t.ctx, nil, fmt.Errorf("test cluster not found"))

	// When
	result, err := t.unitUnderTest.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
		},
		Spec: v1alpha1.AccountSpec{},
	})

	// Then
	t.ErrorContains(err, "test cluster not found")
	t.Nil(result)
}

func (t *ManagerTestSuite) Test_Update_ShouldSucceed() {
	// Given
	var (
		caughtAccountJWT string
	)
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, nil, &t.clusterConfig)
	t.secretManagerMock.mockGetSecrets(t.ctx, "account-namespace", "account-name", accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockUploadAccountJWTCatch(func(jwt string) { caughtAccountJWT = jwt })
	t.natsConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Update(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				k8s.LabelAccountID: accountID,
			},
		},
		Spec: v1alpha1.AccountSpec{},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)

	t.verifyAccountResult(result, caughtAccountJWT, accountRootKey, accountSignKey)
}

func (t *ManagerTestSuite) Test_Import_ShouldSucceed() {
	// Given
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	accountSignKeyPublic, _ := accountSignKey.PublicKey()

	existingNatsLimitsSubs := int64(100)
	existingSpec := v1alpha1.AccountSpec{
		NatsLimits: &v1alpha1.NatsLimits{
			Subs: &existingNatsLimitsSubs,
		},
	}
	existingClaims, err := newClaimsBuilder(t.ctx, "Existing Account", existingSpec, accountID, t.nauthAccountResolverMock).
		signingKey(accountSignKeyPublic).
		build()
	t.NoError(err, "failed to build existing account claims")
	existingJWT, err := existingClaims.Encode(accountSignKey)
	t.NoError(err, "failed to encode existing account JWT")

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, nil, &t.clusterConfig)
	t.secretManagerMock.mockGetSecrets(t.ctx, "account-namespace", "account-name", accountID, &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockLookupAccountJWT(accountID, existingJWT)
	t.natsConnMock.mockDisconnect()

	// When
	result, err := t.unitUnderTest.Import(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				k8s.LabelAccountID: accountID,
			},
		},
	})

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(accountID, result.AccountID)
	t.Equal(existingNatsLimitsSubs, *result.Claims.NatsLimits.Subs)
}

func (t *ManagerTestSuite) Test_Delete_ShouldSucceed() {
	// Given
	var (
		caughtDeleteJWT string
	)
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	t.clusterConfigReaderMock.mockGetClusterConfig(t.ctx, nil, &t.clusterConfig)
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockDeleteAccountJWTCatch(func(jwt string) { caughtDeleteJWT = jwt })
	t.natsConnMock.mockDisconnect()
	t.secretManagerMock.mockDeleteAll(t.ctx, "account-namespace", "account-name", accountID)

	// When
	err := t.unitUnderTest.Delete(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				k8s.LabelAccountID: accountID,
			},
		},
	})

	// Then
	t.NoError(err, "failed to delete account")
	t.NotEmpty(caughtDeleteJWT, "expected deletion JWT to be published to NATS")
	deleteClaims, err := jwt.DecodeGeneric(caughtDeleteJWT)
	t.NoError(err, "failed to decode deletion JWT")
	t.Equal([]interface{}{accountID}, deleteClaims.Data["accounts"])
}

/* ****************************************************
* Helpers
*****************************************************/

func (t *ManagerTestSuite) verifyAccountResult(result *controller.AccountResult, caughtAccountJWT string, expectRootKey, expectSignKey nkeys.KeyPair) *jwt.AccountClaims {
	rootKeyPublic, err := expectRootKey.PublicKey()
	t.NoError(err, "failed to get public key from expect root key pair")
	signKeyPublic, err := expectSignKey.PublicKey()
	t.NoError(err, "failed to get public key from expect signing key pair")

	t.NotNil(result)
	t.NotEmpty(result.AccountID)
	t.Equal(result.AccountID, rootKeyPublic)
	t.Equal(t.opSignKeyPublic, result.AccountSignedBy)

	t.NotEmpty(caughtAccountJWT)
	accountClaims, err := jwt.DecodeAccountClaims(caughtAccountJWT)
	t.NoError(err, "failed to decode caught account JWT")

	t.Equal(t.opSignKeyPublic, accountClaims.Issuer)
	t.Equal(result.AccountID, accountClaims.Subject)

	t.Equal([]string{signKeyPublic}, accountClaims.SigningKeys.Keys(), "account claims should contain the expected signing key")

	return accountClaims
}

/* ****************************************************
* clusterConfigResolver Mock
*****************************************************/
type clusterConfigReaderMock struct {
	mock.Mock
}

func newClusterConfigReaderMock() *clusterConfigReaderMock {
	return &clusterConfigReaderMock{}
}

func (m *clusterConfigReaderMock) GetClusterConfig(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef) (*clusterConfig, error) {
	args := m.Called(ctx, accountClusterRef)
	return args.Get(0).(*clusterConfig), args.Error(1)
}

func (m *clusterConfigReaderMock) mockGetClusterConfig(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef, result *clusterConfig) {
	m.On("GetClusterConfig", ctx, accountClusterRef).Return(result, nil)
}

func (m *clusterConfigReaderMock) mockGetClusterConfigError(ctx context.Context, accountClusterRef *v1alpha1.NatsClusterRef, err error) {
	m.On("GetClusterConfig", ctx, accountClusterRef).Return((*clusterConfig)(nil), err)
}

var _ clusterConfigResolver = (*clusterConfigReaderMock)(nil)

/* ****************************************************
* secretManager Mock
*****************************************************/

type secretManagerMock struct {
	mock.Mock
}

func newSecretManagerMock() *secretManagerMock {
	return &secretManagerMock{}
}

func (m *secretManagerMock) ApplyRootSecret(ctx context.Context, namespace, accountName string, rootKeyPair nkeys.KeyPair) error {
	args := m.Called(ctx, namespace, accountName, rootKeyPair)
	return args.Error(0)
}

func (m *secretManagerMock) mockApplyRootSecret(ctx context.Context, namespace, accountName string, rootKeyPair nkeys.KeyPair) {
	m.On("ApplyRootSecret", ctx, namespace, accountName, rootKeyPair).Return(nil)
}

func (m *secretManagerMock) mockApplyRootSecretUnknown(ctx context.Context, namespace, accountName string, catch func(rootKeyPair nkeys.KeyPair)) {
	m.On("ApplyRootSecret", ctx, namespace, accountName, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if catch != nil {
				catch(args.Get(3).(nkeys.KeyPair))
			}
		})
}

func (m *secretManagerMock) ApplySignSecret(ctx context.Context, namespace, accountName, accountID string, signKeyPair nkeys.KeyPair) error {
	args := m.Called(ctx, namespace, accountName, accountID, signKeyPair)
	return args.Error(0)
}

func (m *secretManagerMock) mockApplySignSecret(ctx context.Context, namespace, accountName, accountID string, signKeyPair nkeys.KeyPair) {
	m.On("ApplySignSecret", ctx, namespace, accountName, accountID, signKeyPair).Return(nil)
}

func (m *secretManagerMock) mockApplySignSecretUnknown(ctx context.Context, namespace, accountName string, catch func(accountID string, signKeyPair nkeys.KeyPair)) {
	m.On("ApplySignSecret", ctx, namespace, accountName, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			if catch != nil {
				catch(args.String(3), args.Get(4).(nkeys.KeyPair))
			}
		})
}

func (m *secretManagerMock) DeleteAll(ctx context.Context, namespace, accountName, accountID string) error {
	args := m.Called(ctx, namespace, accountName, accountID)
	return args.Error(0)
}

func (m *secretManagerMock) mockDeleteAll(ctx context.Context, namespace, accountName, accountID string) {
	m.On("DeleteAll", ctx, namespace, accountName, accountID).Return(nil)
}

func (m *secretManagerMock) GetSecrets(ctx context.Context, namespace, accountName, accountID string) (*Secrets, error) {
	args := m.Called(ctx, namespace, accountName, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Secrets), args.Error(1)
}

func (m *secretManagerMock) mockGetSecrets(ctx context.Context, namespace, accountName, accountID string, result *Secrets) {
	m.On("GetSecrets", ctx, namespace, accountName, accountID).Return(result, nil)
}

func (m *secretManagerMock) mockGetSecretsError(ctx context.Context, namespace, accountName, accountID string, err error) {
	m.On("GetSecrets", ctx, namespace, accountName, accountID).Return(nil, err)
}

var _ secretManager = (*secretManagerMock)(nil)
