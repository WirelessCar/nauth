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
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Controller must not depend on other adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
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

	accountManagerMock *AccountManagerMock
	fakeRecorder       *events.FakeRecorder

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
	t.Require().NoError(os.Setenv(EnvOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	t.accountName = scopedTestName(accountBaseName, testName)
	t.accountNamespace = scopedTestName("account", testName)
	t.operatorNamespace = scopedTestName("operator", testName)
	t.accountNamespacedRef = ktypes.NamespacedName{
		Name:      t.accountName,
		Namespace: t.accountNamespace,
	}

	t.accountManagerMock = &AccountManagerMock{}
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewAccountReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountManagerMock,
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
	t.accountManagerMock.AssertExpectations(t.T())
	t.Require().NoError(os.Unsetenv(EnvOperatorVersion))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenCreatingAccount() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	t.accountManagerMock.On("Create", mock.Anything).Return(mockResult, nil).Once()

	// When (expect manager.Create)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(controllerReasonReconciled, c.Reason)
	}
	t.Equal(t.operatorVersion, account.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldFail_WhenCreateFails() {
	// Given
	accountsManagerErr := fmt.Errorf("a test error")

	t.accountManagerMock.On("Create", mock.Anything).Return(nil, accountsManagerErr).Once()

	// When (expect manager.Create)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.True(errors.Is(err, accountsManagerErr))

	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(controllerReasonErrored, c.Reason)
	}
	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, "failed to create the account: a test error")
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotDeleteObservedAccount() {
	// Given
	mockResult := &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &v1alpha1.AccountClaims{},
	}
	// Note: Expect manager.Import during setup only
	t.accountManagerMock.On("Import", mock.Anything).Return(mockResult, nil).Once()

	account := &v1alpha1.Account{}
	err := k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	account.Labels = map[string]string{k8s.LabelManagementPolicy: k8s.LabelManagementPolicyObserveValue}
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
	t.accountManagerMock.AssertExpectations(t.T())

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
	// Note: Expect manager.Create during setup only
	t.accountManagerMock.On("Create", mock.Anything).Return(mockResult, nil).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(account, controllerAccountFinalizer))

	err = k8sClient.Delete(t.ctx, account)
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.False(account.DeletionTimestamp.IsZero())

	// Note: assert mock calls during setup and reset for test case
	t.accountManagerMock.AssertExpectations(t.T())
	t.accountManagerMock.On("Delete", mock.Anything).Return(nil).Once()

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
	// Note: Expect manager.Create during setup only
	t.accountManagerMock.On("Create", mock.Anything).Return(mockResult, nil).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	t.True(controllerutil.ContainsFinalizer(account, controllerAccountFinalizer))

	err = k8sClient.Delete(t.ctx, account)
	t.Require().NoError(err)

	// Note: assert mock calls during setup and reset for test case
	t.accountManagerMock.AssertExpectations(t.T())
	t.accountManagerMock.On("Delete", mock.Anything).Return(deletionErr).Once()

	// When (expect manager.Delete)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.Contains(err.Error(), deletionErr.Error())

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(controllerReasonErrored, c.Reason)
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

	account.Labels = map[string]string{k8s.LabelManagementPolicy: k8s.LabelManagementPolicyObserveValue}
	err = k8sClient.Update(t.ctx, account)
	t.Require().NoError(err)

	t.accountManagerMock.On("Import", mock.Anything).Return(mockResult, nil).Once()

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
	// Note: Expect manager.Create during setup only
	t.accountManagerMock.On("Create", mock.Anything).Return(mockResult, nil).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(EnvOperatorVersion, newOperatorVersion))

	// Note: assert mock calls during setup and reset for test case
	t.accountManagerMock.AssertExpectations(t.T())
	t.accountManagerMock.On("Update", mock.Anything).Return(mockResult, nil).Once()

	// When (expect manager.Update)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(controllerReasonReconciled, c.Reason)
	}
	t.Equal(newOperatorVersion, account.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

type AccountManagerMock struct {
	mock.Mock
}

func (o *AccountManagerMock) Create(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	args := o.Called(state)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*domain.AccountResult), nil
}

func (o *AccountManagerMock) Update(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	args := o.Called(state)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*domain.AccountResult), nil
}

func (o *AccountManagerMock) Import(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	args := o.Called(state)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*domain.AccountResult), nil
}

func (o *AccountManagerMock) Delete(ctx context.Context, desired *v1alpha1.Account) error {
	args := o.Called(desired)
	return args.Error(0)
}
