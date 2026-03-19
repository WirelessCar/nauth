package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
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
	sauCreds        domain.NatsUserCreds
	natsURL         string
	clusterTarget   clusterTarget

	accountReaderMock         *AccountReaderMock
	natsClientMock            *NatsClientMock
	natsConnMock              *NatsConnectionMock
	clusterTargetResolverMock *clusterTargetResolverMock
	secretManagerMock         *secretManagerMock

	unitUnderTest *Manager
}

func (t *ManagerTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.opSignKey, _ = nkeys.CreateOperator()
	t.opSignKeyPublic, _ = t.opSignKey.PublicKey()
	t.sauCreds = domain.NatsUserCreds{
		Creds:     []byte("FAKE_CREDENTIALS"),
		AccountID: "FAKE_SYS_ACCOUNT_ID",
	}
	t.natsURL = "nats://nats:4222"
	t.clusterTarget = clusterTarget{
		NatsURL:            t.natsURL,
		OperatorSigningKey: t.opSignKey,
		SystemAdminCreds:   t.sauCreds,
	}

	t.clusterTargetResolverMock = newClusterTargetResolverMock()
	t.secretManagerMock = newSecretManagerMock()
	t.accountReaderMock = NewAccountReaderMock()
	t.natsClientMock = NewNatsClientMock()
	t.natsConnMock = NewNatsConnectionMock()

	var err error
	t.unitUnderTest, err = newManager(
		t.natsClientMock,
		t.accountReaderMock,
		t.clusterTargetResolverMock,
		t.secretManagerMock,
	)
	t.NoError(err)
}

func (t *ManagerTestSuite) TearDownTest() {
	t.clusterTargetResolverMock.AssertExpectations(t.T())
	t.secretManagerMock.AssertExpectations(t.T())
	t.accountReaderMock.AssertExpectations(t.T())
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
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	var natsLimitsSubs int64 = 100

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsError(t.ctx, accountRef, "", fmt.Errorf("no secrets found"))
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(accountID string, signKeyPair nkeys.KeyPair) {
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

	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	natsLimitsSubs := int64(100)

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, &v1alpha1.NatsClusterRef{
		Namespace: "account-namespace",
		Name:      "account-namespace-cluster",
	}, &t.clusterTarget)
	t.secretManagerMock.mockGetSecretsError(t.ctx, accountRef, "", fmt.Errorf("no secrets found"))
	t.secretManagerMock.mockApplyRootSecretUnknown(t.ctx, accountRef, func(rootKeyPair nkeys.KeyPair) {
		caughtRootKeyPair = rootKeyPair
	})
	t.secretManagerMock.mockApplySignSecretUnknown(t.ctx, accountRef, func(accountID string, signKeyPair nkeys.KeyPair) {
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
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()
	var natsLimitsSubs int64 = 100

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, "", &Secrets{
		Root: accountRootKey,
		Sign: accountSignKey,
	})
	t.secretManagerMock.mockApplyRootSecret(t.ctx, accountRef, accountRootKey)
	t.secretManagerMock.mockApplySignSecret(t.ctx, accountRef, accountID, accountSignKey)
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
	t.clusterTargetResolverMock.mockGetClusterTargetError(t.ctx, nil, fmt.Errorf("test cluster not found"))

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
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()
	accountSignKey, _ := nkeys.CreateAccount()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.secretManagerMock.mockGetSecrets(t.ctx, accountRef, accountID, &Secrets{
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
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
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
	existingClaims, err := newClaimsBuilder(t.ctx, "Existing Account", existingSpec, accountID, t.accountReaderMock).
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
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	t.clusterTargetResolverMock.mockGetClusterTarget(t.ctx, nil, &t.clusterTarget)
	t.natsClientMock.mockConnect(t.natsURL, t.sauCreds, t.natsConnMock)
	t.natsConnMock.mockDeleteAccountJWTCatch(func(jwt string) { caughtDeleteJWT = jwt })
	t.natsConnMock.mockDisconnect()
	t.secretManagerMock.mockDeleteAll(t.ctx, accountRef, accountID)

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

func (t *ManagerTestSuite) Test_signAccountJWT_ShouldFailWhenInvalidClaims() {
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

func (t *ManagerTestSuite) Test_SignUserJWT_ShouldSucceed() {
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
				k8s.LabelAccountID: accountID,
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

func (t *ManagerTestSuite) Test_SignUserJWT_ShouldFailWhenAccountIsNotReady() {
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

func (t *ManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsIssuerAccountDoesNotMatchFoundAccountID() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				k8s.LabelAccountID: accountID,
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

func (t *ManagerTestSuite) Test_SignUserJWT_ShouldFailWhenClaimsValidationFails() {
	// Given
	accountRef := domain.NewNamespacedName("account-namespace", "account-name")
	accountRootKey, _ := nkeys.CreateAccount()
	accountID, _ := accountRootKey.PublicKey()

	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "account-namespace",
			Name:      "account-name",
			Labels: map[string]string{
				k8s.LabelAccountID: accountID,
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

func (m *secretManagerMock) mockApplyRootSecret(ctx context.Context, accountRef domain.NamespacedName, rootKeyPair nkeys.KeyPair) {
	m.On("ApplyRootSecret", ctx, accountRef, rootKeyPair).Return(nil)
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

func (m *secretManagerMock) mockApplySignSecret(ctx context.Context, accountRef domain.NamespacedName, accountID string, signKeyPair nkeys.KeyPair) {
	m.On("ApplySignSecret", ctx, accountRef, accountID, signKeyPair).Return(nil)
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

func (m *secretManagerMock) mockDeleteAll(ctx context.Context, accountRef domain.NamespacedName, accountID string) {
	m.On("DeleteAll", ctx, accountRef, accountID).Return(nil)
}

func (m *secretManagerMock) GetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string) (*Secrets, error) {
	args := m.Called(ctx, accountRef, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Secrets), args.Error(1)
}

func (m *secretManagerMock) mockGetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string, result *Secrets) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(result, nil)
}

func (m *secretManagerMock) mockGetSecretsError(ctx context.Context, accountRef domain.NamespacedName, accountID string, err error) {
	m.On("GetSecrets", ctx, accountRef, accountID).Return(nil, err)
}

var _ secretManager = (*secretManagerMock)(nil)
