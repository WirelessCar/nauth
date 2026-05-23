package core

import (
	"context"
	"errors"
	"testing"

	k8s "github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccountSigningKeyManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	secretClientMock *SecretClientMock
	owner            metav1.Object

	unitUnderTest *AccountSigningKeyManager
}

func TestAccountSigningKeyManager_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountSigningKeyManagerTestSuite))
}

func (t *AccountSigningKeyManagerTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.secretClientMock = NewSecretClientMock()
	t.owner = &metav1.ObjectMeta{Name: "test-ask", UID: "test-uid"}
	t.unitUnderTest = NewAccountSigningKeyManager(t.secretClientMock)
}

func (t *AccountSigningKeyManagerTestSuite) TearDownTest() {
	t.secretClientMock.AssertExpectations(t.T())
}

func (t *AccountSigningKeyManagerTestSuite) secretRef() domain.NamespacedName {
	return domain.NewNamespacedName("test-ns", "test-ask-ac-sign")
}

// --- CreateOrUpdate ---

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldGenerateAndStoreKey_WhenSecretMissing() {
	// Given
	t.secretClientMock.mockGetNotFound(t.secretRef())

	var capturedSeed string
	t.secretClientMock.On("Apply", mock.Anything, t.owner, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedSeed = args.Get(3).(map[string]string)[k8s.DefaultSecretKeyName]
		}).
		Return(nil).Once()

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().NoError(err)
	t.Require().NotEmpty(result.PublicKey)
	t.Require().Equal(t.secretRef().Name, result.SecretName)
	t.Require().NotEmpty(capturedSeed, "seed must have been written to the secret")
}

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldApplyCorrectLabels_WhenCreatingSecret() {
	// Given
	t.secretClientMock.mockGetNotFound(t.secretRef())

	t.secretClientMock.On("Apply", mock.Anything, t.owner,
		mock.MatchedBy(func(meta metav1.ObjectMeta) bool {
			return meta.Labels[k8s.LabelManaged] == k8s.LabelManagedValue
		}),
		mock.Anything).Return(nil).Once()

	// When
	_, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().NoError(err)
}

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldReadExistingKey_WhenSecretOwnedByThis() {
	// Given
	key := testutil.CreateNatsTestAccountKey()
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: string(key.Seed),
	})
	t.secretClientMock.mockIsOwnedBy(t.secretRef(), t.owner, true)

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().NoError(err)
	t.Require().Equal(key.PublicKey, result.PublicKey)
	t.Require().Equal(t.secretRef().Name, result.SecretName)
}

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenSecretNotOwnedByThis() {
	// Given
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: "some-seed",
	})
	t.secretClientMock.mockIsOwnedBy(t.secretRef(), t.owner, false)

	// When
	_, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().ErrorIs(err, ErrSigningKeyConflict)
}

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenExistingSecretHasInvalidSeed() {
	// Given
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: "not-a-valid-nkey-seed",
	})
	t.secretClientMock.mockIsOwnedBy(t.secretRef(), t.owner, true)

	// When
	_, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
}

func (t *AccountSigningKeyManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenExistingSecretMissesSeedKey() {
	// Given
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		"wrong-key": "value",
	})
	t.secretClientMock.mockIsOwnedBy(t.secretRef(), t.owner, true)

	// When
	_, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.AccountSigningKeyRequest{
		SecretRef: t.secretRef(),
		Owner:     t.owner,
	})

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
}

// --- Import ---

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldReturnPublicKey_WhenSecretExists() {
	// Given
	key := testutil.CreateNatsTestAccountKey()
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: string(key.Seed),
	})

	// When
	result, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().NoError(err)
	t.Require().Equal(key.PublicKey, result.PublicKey)
	t.Require().Equal(t.secretRef().Name, result.SecretName)
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenSecretMissing() {
	// Given
	t.secretClientMock.mockGetNotFound(t.secretRef())

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, ErrSecretNotFound)
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenSecretHasInvalidSeed() {
	// Given
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: "not-a-valid-nkey-seed",
	})

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenSecretMissesSeedKey() {
	// Given
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		"wrong-key": "value",
	})

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenGetErrors() {
	// Given
	getErr := errors.New("k8s unavailable")
	t.secretClientMock.On("Get", mock.Anything, t.secretRef()).Return(nil, false, getErr)

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, getErr)
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenSecretHoldsUserSeed() {
	// Given: the referenced Secret holds a syntactically valid nkey seed of the
	// wrong type (user). Account signing keys must be account-type public keys.
	userKey := testutil.CreateNatsTestUserKey()
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: string(userKey.Seed),
	})

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
	t.Contains(err.Error(), userKey.PublicKey, "error should name the non-account public key derived from the seed")
}

func (t *AccountSigningKeyManagerTestSuite) Test_Import_ShouldFail_WhenSecretHoldsOperatorSeed() {
	// Given
	opKey := testutil.CreateNatsTestOperatorKey()
	t.secretClientMock.mockGet(t.ctx, t.secretRef(), map[string]string{
		k8s.DefaultSecretKeyName: string(opKey.Seed),
	})

	// When
	_, err := t.unitUnderTest.Import(t.ctx, t.secretRef())

	// Then
	t.Require().ErrorIs(err, ErrInvalidSeed)
	t.Contains(err.Error(), opKey.PublicKey, "error should name the non-account public key derived from the seed")
}
