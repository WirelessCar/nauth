/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	accountBaseName  = "test-resource"
	accountPublicKey = "ACSOMETHINGKEY"
)

type AccountControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	accountManagerMock      *AccountManagerMock
	accountExportReaderMock *accountExportReaderMock
	fakeRecorder            *events.FakeRecorder

	accountNamespacedRef ktypes.NamespacedName
	accountName          string
	accountNamespace     string
	operatorNamespace    string
	operatorVersion      string

	unitUnderTest *AccountReconciler
}

func TestAccountController_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountControllerTestSuite))
}

func (t *AccountControllerTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.operatorVersion = testOperatorVersion
	t.Require().NoError(os.Setenv(envOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	t.accountName = scopedTestName(accountBaseName, testName)
	t.accountNamespace = scopedTestName("account", testName)
	t.operatorNamespace = scopedTestName("operator", testName)
	t.accountNamespacedRef = ktypes.NamespacedName{
		Name:      t.accountName,
		Namespace: t.accountNamespace,
	}

	t.accountManagerMock = &AccountManagerMock{}
	t.accountExportReaderMock = &accountExportReaderMock{}
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewAccountReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountManagerMock,
		t.accountExportReaderMock,
		t.fakeRecorder,
	)

	t.Require().NoError(ensureNamespace(t.ctx, t.operatorNamespace))
	t.Require().NoError(ensureNamespace(t.ctx, t.accountNamespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountName,
			Namespace: t.accountNamespace,
		},
	}))
}

func (t *AccountControllerTestSuite) TearDownTest() {
	t.assertAllMockExpectations()
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

func (t *AccountControllerTestSuite) assertAllMockExpectations() {
	t.accountManagerMock.AssertExpectations(t.T())
	t.accountExportReaderMock.AssertExpectations(t.T())
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenCreatingAccount() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Equal(t.operatorVersion, account.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldFail_WhenCreateOrUpdateFails() {
	// Given
	accountsManagerErr := fmt.Errorf("a test error")

	t.accountManagerMock.mockCreateOrUpdateError(t.ctx, mock.Anything, accountsManagerErr).Once()

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.True(errors.Is(err, accountsManagerErr))

	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}
	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, "failed to apply account: a test error")
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotDeleteObservedAccount() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.Import during setup only
	t.accountManagerMock.mockImport(t.ctx, mock.Anything, mockResult).Once()

	account := &v1alpha1.Account{}
	err := k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	account.Labels = map[string]string{string(v1alpha1.AccountLabelManagementPolicy): v1alpha1.AccountManagementPolicyObserve}
	err = k8sClient.Update(t.ctx, account)
	t.Require().NoError(err)

	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	err = k8sClient.Delete(t.ctx, account)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.False(account.DeletionTimestamp.IsZero())

	// Note: assert mock calls during setup and reset for test case
	t.assertAllMockExpectations()

	// When (expect no manager calls, especially not manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Error(err)
	t.True(k8err.IsNotFound(err))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldDeleteAccountMarkedForDeletion() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(account, finalizerAccount))

	err = k8sClient.Delete(t.ctx, account)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.False(account.DeletionTimestamp.IsZero())

	// Note: assert mock calls during setup and reset for test case
	t.assertAllMockExpectations()
	t.accountManagerMock.mockDelete(t.ctx, mock.Anything, nil).Once()

	// When (expect manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Error(err)
	t.True(k8err.IsNotFound(err))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldFail_WhenDeleteFails() {
	// Given
	deletionErr := fmt.Errorf("Unable to delete account")
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(account, finalizerAccount))

	err = k8sClient.Delete(t.ctx, account)
	t.Require().NoError(err)

	// Note: assert mock calls during setup and reset for test case
	t.assertAllMockExpectations()
	t.accountManagerMock.mockDelete(t.ctx, mock.Anything, deletionErr).Once()

	// When (expect manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.Contains(err.Error(), deletionErr.Error())

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}
	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, deletionErr.Error())
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldImportObservedAccount() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}

	account := &v1alpha1.Account{}
	err := k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	account.Labels = map[string]string{
		string(v1alpha1.AccountLabelManagementPolicy): v1alpha1.AccountManagementPolicyObserve}
	err = k8sClient.Update(t.ctx, account)
	t.Require().NoError(err)

	t.accountManagerMock.mockImport(t.ctx, mock.Anything, mockResult).Once()

	// When (expect manager.Import)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenOperatorVersionChanges() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(envOperatorVersion, newOperatorVersion))

	// Note: assert mock calls during setup and reset for test case
	t.assertAllMockExpectations()
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.accountExportReaderMock.mockFindByAccountID(t.ctx, mock.Anything, mock.Anything, &v1alpha1.AccountExportList{}).Once()

	// When (expect manager.CreateOrUpdate)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Equal(newOperatorVersion, account.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenAccountExportsExist() {
	// Given
	accountKey, _ := nkeys.CreateAccount()
	accountID, _ := accountKey.PublicKey()
	mockResult := &domain.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	var accountResources0 domain.AccountResources
	t.accountManagerMock.mockCreateOrUpdateSpy(t.ctx, func(resources domain.AccountResources) {
		accountResources0 = resources
	}, mockResult).Once()
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.accountNamespace,
			Name:      t.accountName,
		},
	}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)
	t.Require().NotNil(accountResources0)
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(account), account))
	t.Require().Equal(accountID, account.GetLabel(v1alpha1.AccountLabelAccountID))

	// Note: assert mock calls during setup and reset for test case
	t.assertAllMockExpectations()
	var accountResources1 domain.AccountResources
	t.accountManagerMock.mockCreateOrUpdateSpy(t.ctx, func(resources domain.AccountResources) {
		accountResources1 = resources
	}, mockResult).Once()
	t.accountExportReaderMock.mockFindByAccountID(t.ctx, mock.Anything, mock.Anything, &v1alpha1.AccountExportList{
		Items: []v1alpha1.AccountExport{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.accountNamespace,
					Name:      "export-1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: t.accountNamespace,
					Name:      "export-2",
				},
			},
		},
	}).Once()

	// When (expect manager.CreateOrUpdate)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Empty(t.fakeRecorder.Events)
	t.Require().NotNil(accountResources1)
	t.Equal(2, len(accountResources1.Exports))
}

/* ****************************************************
* inbound.AccountManager Mock
*****************************************************/
type AccountManagerMock struct {
	mock.Mock
}

func (o *AccountManagerMock) CreateOrUpdate(ctx context.Context, resources domain.AccountResources) (*domain.AccountResult, error) {
	args := o.Called(ctx, resources)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*domain.AccountResult), nil
}

func (o *AccountManagerMock) mockCreateOrUpdate(ctx interface{}, resources interface{}, result *domain.AccountResult) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(result, nil)
	return call
}

func (o *AccountManagerMock) mockCreateOrUpdateSpy(ctx interface{}, resourcesCallback func(resources domain.AccountResources), result *domain.AccountResult) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, mock.Anything)
	call.Run(func(args mock.Arguments) {
		resourcesCallback(args.Get(1).(domain.AccountResources))
	})
	call.Return(result, nil)
	return call
}

func (o *AccountManagerMock) mockCreateOrUpdateError(ctx interface{}, resources interface{}, err error) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(nil, err)
	return call
}

func (o *AccountManagerMock) Import(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	args := o.Called(ctx, state)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*domain.AccountResult), nil
}

func (o *AccountManagerMock) Delete(ctx context.Context, state *v1alpha1.Account) error {
	args := o.Called(ctx, state)
	return args.Error(0)
}

func (o *AccountManagerMock) mockDelete(ctx interface{}, state interface{}, err error) *mock.Call {
	call := o.On("Delete", ctx, state)
	call.Return(err)
	return call
}

func (o *AccountManagerMock) mockImport(ctx interface{}, state interface{}, result *domain.AccountResult) *mock.Call {
	call := o.On("Import", ctx, state)
	call.Return(result, nil)
	return call
}

var _ inbound.AccountManager = (*AccountManagerMock)(nil)

/* ****************************************************
* outbound.AccountExportReader Mock
*****************************************************/
type accountExportReaderMock struct {
	mock.Mock
}

func (m *accountExportReaderMock) FindByAccountID(ctx context.Context, namespace domain.Namespace, accountID string) (*v1alpha1.AccountExportList, error) {
	args := m.Called(ctx, namespace, accountID)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*v1alpha1.AccountExportList), nil
}

func (m *accountExportReaderMock) mockFindByAccountID(ctx interface{}, namespace interface{}, accountID interface{}, result *v1alpha1.AccountExportList) *mock.Call {
	call := m.On("FindByAccountID", ctx, namespace, accountID)
	call.Return(result, nil)
	return call
}

var _ outbound.AccountExportReader = (*accountExportReaderMock)(nil)
