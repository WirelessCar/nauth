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
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
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

	accountManagerMock *AccountManagerMock
	fakeRecorder       *events.FakeRecorder

	accountNamespacedRef ktypes.NamespacedName
	accountName          string
	accountNamespace     string
	operatorNamespace    string
	operatorVersion      string

	unitUnderTest            *AccountReconciler
	withExperimentalFeatures func(features *ExperimentalFeatures)
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
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewAccountReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountManagerMock,
		t.fakeRecorder,
		&ExperimentalFeatures{},
	)
	t.withExperimentalFeatures = func(features *ExperimentalFeatures) {
		t.unitUnderTest.features = features
	}

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
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenCreatingAccount() {
	// Given
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
		ClaimsHash:      "CLAIMS_HASH",
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
	t.Equal("CLAIMS_HASH", account.Status.ClaimsHash)
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
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
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
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
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
	t.accountManagerMock.AssertExpectations(t.T())
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
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
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
	t.accountManagerMock.AssertExpectations(t.T())
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
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
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
	mockResult := &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	account := &v1alpha1.Account{}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(envOperatorVersion, newOperatorVersion))

	// Note: assert mock calls during setup and reset for test case
	t.accountManagerMock.AssertExpectations(t.T())
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()

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
	t.withExperimentalFeatures(&ExperimentalFeatures{
		AccountExportEnabled: true,
	})

	// Given
	accountKey, _ := nkeys.CreateAccount()
	accountID, _ := accountKey.PublicKey()
	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
	}
	// Note: Expect manager.CreateOrUpdate during setup only
	var spyAccountResourcesInit nauth.AccountResources
	t.accountManagerMock.mockCreateOrUpdateSpy(t.ctx, func(resources nauth.AccountResources) {
		spyAccountResourcesInit = resources
	}, mockResult).Once()
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.accountNamespace,
			Name:      t.accountName,
		},
	}

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})
	t.Require().NoError(err)
	t.Require().NotNil(spyAccountResourcesInit)
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(account), account))
	t.Require().Equal(accountID, account.GetLabel(v1alpha1.AccountLabelAccountID))

	// Note: assert mock calls during setup and reset for test case
	t.accountManagerMock.AssertExpectations(t.T())
	export1 := t.createExport(domain.Namespace(t.accountNamespace), "export-1", accountID, t.anyExportClaim(10))
	t.createExport("ns-other", "export-2", accountID, t.anyExportClaim(20))
	export3 := t.createExport(domain.Namespace(t.accountNamespace), "export-3", accountID, nil) // Not expected into manager
	t.createExport(domain.Namespace(t.accountNamespace), "export-4", "ANOTHERACCOUNT", t.anyExportClaim(40))
	export5 := t.createExport(domain.Namespace(t.accountNamespace), "export-5", accountID, t.anyExportClaim(50))
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(resources nauth.AccountResources) (*nauth.AccountResult, error) {
		adoptions := nauth.NewAccountAdoptions()
		t.Require().Equalf(2, len(resources.ExportGroups), "expected 2 export groups: export-1 and export-5")
		for _, exportGroup := range resources.ExportGroups {
			t.Require().NoError(adoptions.Exports.Add(nauth.AdoptionResult{
				Ref: exportGroup.Ref,
			}))
		}
		return &nauth.AccountResult{
			AccountID:       accountID,
			AccountSignedBy: "OPERATOR_SIGNING_KEY",
			Claims:          &nauth.AccountClaims{},
			Adoptions:       adoptions,
		}, nil
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
	expectAdoptions := &v1alpha1.AccountAdoptions{
		Exports: []v1alpha1.AccountAdoption{
			{
				Name:               export1.Name,
				UID:                export1.UID,
				ObservedGeneration: export1.Generation,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					Reason:                         conditionReasonOK,
					Message:                        "Adopted",
					DesiredClaimObservedGeneration: &export1.Status.DesiredClaim.ObservedGeneration,
				},
			},
			{
				Name:               export3.Name,
				UID:                export3.UID,
				ObservedGeneration: export3.Generation,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:  metav1.ConditionFalse,
					Reason:  conditionReasonNOK,
					Message: "Adoption pending: no desired claim",
				},
			},
			{
				Name:               export5.Name,
				UID:                export5.UID,
				ObservedGeneration: export5.Generation,
				Status: v1alpha1.AccountAdoptionStatus{
					Status:                         metav1.ConditionTrue,
					Reason:                         conditionReasonOK,
					Message:                        "Adopted",
					DesiredClaimObservedGeneration: &export5.Status.DesiredClaim.ObservedGeneration,
				},
			},
		},
	}
	t.Require().Equal(expectAdoptions, account.Status.Adoptions)
}

func (t *AccountControllerTestSuite) anyExportClaim(observedGeneration int64) *v1alpha1.AccountExportClaim {
	subject := v1alpha1.Subject(fmt.Sprintf("foo.%d.>", observedGeneration))
	return &v1alpha1.AccountExportClaim{
		ObservedGeneration: observedGeneration,
		Rules: []v1alpha1.AccountExportRule{
			{
				Subject: subject,
				Type:    v1alpha1.Stream,
			},
		},
	}
}

// Helpers

func (t *AccountControllerTestSuite) createExport(namespace domain.Namespace, name string, accountID string, claim *v1alpha1.AccountExportClaim) *v1alpha1.AccountExport {
	namespaceResource := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: string(namespace)},
	}
	if err := k8sClient.Get(t.ctx, client.ObjectKeyFromObject(namespaceResource), namespaceResource); err != nil {
		t.Require().NoError(k8sClient.Create(t.ctx, namespaceResource))
	}

	initial := &v1alpha1.AccountExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: string(namespace),
			Labels: map[string]string{
				string(v1alpha1.AccountExportLabelAccountID): accountID,
			},
		},
		Spec: v1alpha1.AccountExportSpec{
			AccountName: "account-name",
			Rules: []v1alpha1.AccountExportRule{
				{
					Name:    "rule-name",
					Subject: "foo.*",
					Type:    v1alpha1.Stream,
				},
			},
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, initial))

	// Set status
	created := &v1alpha1.AccountExport{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(initial), created))
	status := v1alpha1.AccountExportStatus{
		DesiredClaim: claim,
	}
	created.Status = status
	t.Require().NoError(k8sClient.Status().Update(t.ctx, created))

	// Verify
	result := &v1alpha1.AccountExport{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(created), result))
	t.Require().Equal(status, result.Status)

	return result
}

/* ****************************************************
* inbound.AccountManager Mock
*****************************************************/
type AccountManagerMock struct {
	mock.Mock
}

func (o *AccountManagerMock) CreateOrUpdate(ctx context.Context, resources nauth.AccountResources) (*nauth.AccountResult, error) {
	args := o.Called(ctx, resources)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*nauth.AccountResult), nil
}

func (o *AccountManagerMock) mockCreateOrUpdate(ctx interface{}, resources interface{}, result *nauth.AccountResult) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(result, nil)
	return call
}

func (o *AccountManagerMock) mockCreateOrUpdateFn(ctx interface{}, resources interface{}, fn func(resources nauth.AccountResources) (*nauth.AccountResult, error)) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Run(func(args mock.Arguments) {
		result, err := fn(args.Get(1).(nauth.AccountResources))
		call.Return(result, err)
	})
	return call
}

func (o *AccountManagerMock) mockCreateOrUpdateSpy(ctx interface{}, resourcesCallback func(resources nauth.AccountResources), result *nauth.AccountResult) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, mock.Anything)
	call.Run(func(args mock.Arguments) {
		resourcesCallback(args.Get(1).(nauth.AccountResources))
	})
	call.Return(result, nil)
	return call
}

func (o *AccountManagerMock) mockCreateOrUpdateError(ctx interface{}, resources interface{}, err error) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(nil, err)
	return call
}

func (o *AccountManagerMock) Import(ctx context.Context, state *v1alpha1.Account) (*nauth.AccountResult, error) {
	args := o.Called(ctx, state)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*nauth.AccountResult), nil
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

func (o *AccountManagerMock) mockImport(ctx interface{}, state interface{}, result *nauth.AccountResult) *mock.Call {
	call := o.On("Import", ctx, state)
	call.Return(result, nil)
	return call
}

var _ inbound.AccountManager = (*AccountManagerMock)(nil)
