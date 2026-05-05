package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccountClientTestSuite struct {
	suite.Suite
	ctx        context.Context
	accountRef domain.NamespacedName

	unitUnderTest *AccountClient
}

func TestAccountClient_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountClientTestSuite))
}

func (t *AccountClientTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.accountRef = domain.NewNamespacedName(scopedTestName("ns", t.T().Name()), sanitizeTestName(t.T().Name()))
	t.Require().NoError(t.accountRef.Validate())

	t.unitUnderTest = NewAccountClient(k8sClient)

	t.Require().NoError(ensureNamespace(t.ctx, t.accountRef.Namespace))
}

func (t *AccountClientTestSuite) Test_Get_ShouldSucceed_WhenAccountIsReady() {
	// Given
	accountID := t.newAccountID()
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountRef.Name,
			Namespace: t.accountRef.Namespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}))

	// When
	result, err := t.unitUnderTest.Get(t.ctx, t.accountRef)

	t.Require().NoError(err)
	t.Require().NotNil(result)
	t.Equal(t.accountRef.Name, result.Name)
	t.Equal(accountID, result.GetLabel(v1alpha1.AccountLabelAccountID))
}

func (t *AccountClientTestSuite) Test_Get_ShouldFail_WhenAccountIsNotReady() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountRef.Name,
			Namespace: t.accountRef.Namespace,
		},
	}))

	// When
	result, err := t.unitUnderTest.Get(t.ctx, t.accountRef)

	// Then
	t.ErrorIs(err, domain.ErrAccountNotReady)
	t.Nil(result)
}

func (t *AccountClientTestSuite) Test_Get_ShouldFail_WhenAccountIsNotFound() {
	// When
	result, err := t.unitUnderTest.Get(t.ctx, t.accountRef)

	// Then
	t.Nil(result)
	t.ErrorIs(err, domain.ErrAccountNotFound)
}

func (t *AccountClientTestSuite) Test_GetAccountID_ShouldSucceed() {
	// Given
	accountID := t.newAccountID()
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountRef.Name,
			Namespace: t.accountRef.Namespace,
			Labels: map[string]string{
				string(v1alpha1.AccountLabelAccountID): accountID,
			},
		},
	}))

	// When
	result, err := t.unitUnderTest.GetAccountID(t.ctx, t.accountRef)

	// Then
	t.NoError(err)
	t.Equal(nauth.AccountID(accountID), result)
}

func (t *AccountClientTestSuite) Test_GetAccountID_ShouldFail_WhenAccountIsNotReady() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountRef.Name,
			Namespace: t.accountRef.Namespace,
		},
	}))

	// When
	result, err := t.unitUnderTest.GetAccountID(t.ctx, t.accountRef)

	// Then
	t.ErrorIs(err, domain.ErrAccountNotReady)
	t.Empty(result)
}

func (t *AccountClientTestSuite) Test_GetAccountID_ShouldFail_WhenAccountIsNotFound() {
	// When
	result, err := t.unitUnderTest.GetAccountID(t.ctx, t.accountRef)

	// Then
	t.ErrorIs(err, domain.ErrAccountNotFound)
	t.Empty(result)
}

func (t *AccountClientTestSuite) Test_GetAccountID_ShouldFail_WhenAccountReferenceIsInvalid() {
	// Given
	accountRef := domain.NewNamespacedName("", "asdf")

	// When
	result, err := t.unitUnderTest.GetAccountID(t.ctx, accountRef)

	// Then
	t.ErrorIs(err, domain.ErrBadRequest)
	t.Empty(result)
}

// Helpers

func (t *AccountClientTestSuite) newAccountID() string {
	account, _ := nkeys.CreateAccount()
	publicKey, _ := account.PublicKey()
	return publicKey
}
