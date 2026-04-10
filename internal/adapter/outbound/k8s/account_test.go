package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	t.accountRef = domain.NewNamespacedName(testNamespace, sanitizeTestName(t.T().Name()))
	t.Require().NoError(t.accountRef.Validate())
	t.unitUnderTest = NewAccountClient(k8sClient)
	t.Require().NoError(cleanAccount(t.ctx, t.accountRef))
	t.Require().NoError(createAccount(t.ctx, t.accountRef))
}

func (t *AccountClientTestSuite) TearDownTest() {
	t.Require().NoError(cleanAccount(t.ctx, t.accountRef))
}

func (t *AccountClientTestSuite) Test_Get_ShouldFail_WhenAccountIsNotReady() {
	fetchedAccount, err := t.unitUnderTest.Get(t.ctx, t.accountRef)

	t.Error(err)
	t.Nil(fetchedAccount)
}

func (t *AccountClientTestSuite) Test_Get_ShouldSucceed_WhenAccountIsReady() {
	t.Require().NoError(accountIsReady(t.ctx, t.accountRef))

	fetchedAccount, err := t.unitUnderTest.Get(t.ctx, t.accountRef)

	t.NoError(err)
	t.NotNil(fetchedAccount)
	t.Equal(t.accountRef.Name, fetchedAccount.Name)
}

func createAccount(ctx context.Context, accountRef domain.NamespacedName) error {
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      accountRef.Name,
			Namespace: accountRef.Namespace,
		},
	}

	return k8sClient.Create(ctx, account)
}

func accountIsReady(ctx context.Context, accountRef domain.NamespacedName) error {
	key := client.ObjectKey{Namespace: accountRef.Namespace, Name: accountRef.Name}
	account := &v1alpha1.Account{}

	err := k8sClient.Get(ctx, key, account)
	if err != nil {
		return err
	}

	account.SetLabel(v1alpha1.AccountLabelAccountID, "account-id")

	return k8sClient.Update(ctx, account)
}

func cleanAccount(ctx context.Context, accountRef domain.NamespacedName) error {
	account := &v1alpha1.Account{}

	key := client.ObjectKey{Namespace: accountRef.Namespace, Name: accountRef.Name}
	if err := k8sClient.Get(ctx, key, account); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return k8sClient.Delete(ctx, account)
}
