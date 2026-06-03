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
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AccountControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	accountManagerMock *accountManagerMock
	clusterManagerMock *clusterManagerMock
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
	t.Require().NoError(os.Setenv(envOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	t.accountName = testutil.ScopedTestName("test-resource", testName)
	t.accountNamespace = testutil.ScopedTestName("account", testName)
	t.operatorNamespace = testutil.ScopedTestName("operator", testName)
	t.accountNamespacedRef = ktypes.NamespacedName{
		Name:      t.accountName,
		Namespace: t.accountNamespace,
	}

	t.accountManagerMock = &accountManagerMock{}
	t.clusterManagerMock = &clusterManagerMock{}
	accountClient := k8s.NewAccountClient(k8sClient)
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewAccountReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountManagerMock,
		t.clusterManagerMock,
		accountClient,
		t.fakeRecorder,
	)

	t.Require().NoError(ensureNamespace(t.ctx, t.operatorNamespace))
	t.Require().NoError(ensureNamespace(t.ctx, t.accountNamespace))
}

func (t *AccountControllerTestSuite) TearDownTest() {
	t.accountManagerMock.AssertExpectations(t.T())
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

type accountOption func(account *v1alpha1.Account)

func (t *AccountControllerTestSuite) defaultAccount(options ...accountOption) *v1alpha1.Account {
	account := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.accountName,
			Namespace: t.accountNamespace,
		},
	}
	for _, o := range options {
		o(account)
	}
	return account
}
func (t *AccountControllerTestSuite) setupAccount(account *v1alpha1.Account) {
	initial := &v1alpha1.Account{
		ObjectMeta: account.ObjectMeta,
		Spec:       account.Spec,
	}

	t.Require().NoError(k8sClient.Create(t.ctx, initial))

	accountRef := ktypes.NamespacedName{
		Name:      account.Name,
		Namespace: account.Namespace,
	}
	updated := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, accountRef, updated))

	updated.Status = account.Status
	t.Require().NoError(k8sClient.Status().Update(t.ctx, updated))

	verify := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, accountRef, verify))
	t.Require().Equal(account.Spec, verify.Spec)
	t.Require().Equal(account.Status, verify.Status)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSetFinalizer() {
	// Given
	t.setupAccount(t.defaultAccount())

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Contains(account.Finalizers, finalizerAccount)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenCreatingAccount() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       testutil.AnyNatsTestAccountID(),
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
		ClaimsHash:      "CLAIMS_HASH",
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
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
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)

	accountsManagerErr := fmt.Errorf("a test error")
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
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

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldBootstrap_WhenNewAccountRequestFails() {
	// Given
	importLimit := int64(3)
	streamLimit := int64(5)
	subLimit := int64(7)
	unlimited := int64(-1)
	wildcardExports := true
	maxBytesRequired := false
	exportAccountName := "export-account"
	accountID := testutil.AnyNatsTestAccountID()

	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Name = exportAccountName
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.Spec.AccountLimits = &v1alpha1.AccountLimits{
				Imports:         &importLimit,
				Exports:         &unlimited,
				WildcardExports: &wildcardExports,
				Conn:            &unlimited,
				LeafNodeConn:    &unlimited,
			}
			account.Spec.JetStreamLimits = &v1alpha1.JetStreamLimits{
				MemoryStorage:        &unlimited,
				DiskStorage:          &unlimited,
				Streams:              &streamLimit,
				Consumer:             &unlimited,
				MaxAckPending:        &unlimited,
				MemoryMaxStreamBytes: &unlimited,
				DiskMaxStreamBytes:   &unlimited,
				MaxBytesRequired:     &maxBytesRequired,
			}
			account.Spec.NatsLimits = &v1alpha1.NatsLimits{
				Subs:    &subLimit,
				Data:    &unlimited,
				Payload: &unlimited,
			}
			account.Spec.Imports = v1alpha1.Imports{
				{
					AccountRef: v1alpha1.AccountRef{
						Name:      exportAccountName,
						Namespace: t.accountNamespace,
					},
					Name:    "stream-import",
					Subject: "foo.>",
					Type:    v1alpha1.Stream,
				},
			}
		}),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(request nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Empty(request.ExportGroups)
		t.Empty(request.ImportGroups)
		t.Equal(&nauth.AccountLimits{
			Imports:         &importLimit,
			Exports:         &unlimited,
			WildcardExports: &wildcardExports,
			Conn:            &unlimited,
			LeafNodeConn:    &unlimited,
		}, request.AccountLimits)
		t.Equal(&nauth.JetStreamLimits{
			MemoryStorage:        &unlimited,
			DiskStorage:          &unlimited,
			Streams:              &streamLimit,
			Consumer:             &unlimited,
			MaxAckPending:        &unlimited,
			MemoryMaxStreamBytes: &unlimited,
			DiskMaxStreamBytes:   &unlimited,
			MaxBytesRequired:     &maxBytesRequired,
		}, request.JetStreamLimits)
		t.Equal(&nauth.NatsLimits{
			Subs:    &subLimit,
			Data:    &unlimited,
			Payload: &unlimited,
		}, request.NatsLimits)

		return &nauth.AccountResult{
			AccountID:       accountID,
			AccountSignedBy: "OPERATOR_SIGNING_KEY",
			Claims: &nauth.AccountClaims{
				AccountLimits:   request.AccountLimits,
				JetStreamLimits: request.JetStreamLimits,
				NatsLimits:      request.NatsLimits,
			},
			ClaimsHash: "BOOTSTRAP_CLAIMS_HASH",
			Adoptions:  nauth.NewAccountAdoptions(),
		}, nil
	}).Once()

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.Equal(requeueImmediately, result.RequeueAfter)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Equal(accountID, account.GetLabel(v1alpha1.AccountLabelAccountID))
	t.Equal("OPERATOR_SIGNING_KEY", account.GetLabel(v1alpha1.AccountLabelSignedBy))
	t.Equal("BOOTSTRAP_CLAIMS_HASH", account.Status.ClaimsHash)
	t.Equal(t.operatorVersion, account.Status.OperatorVersion)

	c := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(c)
	t.Equal(metav1.ConditionFalse, c.Status)
	t.Equal(conditionReasonNotReady, c.Reason)
	t.Contains(c.Message, "Account bootstrapped with limits only")
	t.Contains(c.Message, "failed to create account request")
	t.Empty(t.fakeRecorder.Events)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotBootstrap_WhenExistingAccountRequestFails() {
	// Given
	exportAccountName := "export-account"
	accountID := testutil.AnyNatsTestAccountID()

	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Name = exportAccountName
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
			account.Spec.Imports = v1alpha1.Imports{
				{
					AccountRef: v1alpha1.AccountRef{
						Name:      exportAccountName,
						Namespace: t.accountNamespace,
					},
					Name:    "stream-import",
					Subject: "foo.>",
					Type:    v1alpha1.Stream,
				},
			}
		}),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().Error(err)
	t.ErrorIs(err, domain.ErrAccountNotReady)
	t.accountManagerMock.AssertNotCalled(t.T(), "CreateOrUpdate", mock.Anything, mock.Anything)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))

	c := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(c)
	t.Equal(metav1.ConditionFalse, c.Status)
	t.Equal(conditionReasonErrored, c.Reason)
	t.Require().Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, "failed to create account request")
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldFail_WhenChangingNatsCluster() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelNatsClusterID, "natscluster1")
		}),
	)

	target := createDummyClusterTarget()
	target.UID = "natscluster2"
	t.clusterManagerMock.mockGetClusterTarget(target, nil)

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.Equal(err.Error(), "account already bound to cluster with uid: natscluster1")

	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	c := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Equal(metav1.ConditionFalse, c.Status)
	t.Equal(conditionReasonErrored, c.Reason)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotDeleteObservedAccount() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelManagementPolicy, v1alpha1.AccountManagementPolicyObserve)
			account.SetLabel(v1alpha1.AccountLabelAccountID, testutil.AnyNatsTestAccountID())
		}),
	)

	// Delete it (to set deletion timestamp)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NoError(k8sClient.Delete(t.ctx, account))

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When (expect no manager calls, especially not manager.Delete)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.accountManagerMock.AssertNotCalled(t.T(), "Delete", mock.Anything, mock.Anything)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().Error(err)
	t.True(k8err.IsNotFound(err))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldDeleteAccountMarkedForDeletion() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, testutil.AnyNatsTestAccountID())
		}),
	)

	// Delete it (to set deletion timestamp)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NoError(k8sClient.Delete(t.ctx, account))

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockDelete(t.ctx, mock.Anything, nil).Once()

	// When (expect manager.Delete)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().Error(err)
	t.True(k8err.IsNotFound(err))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldDeleteAccountMarkedForDeletion_WhenAccountIDCanBeFound() {
	// Given
	accountID := nauth.AccountID(testutil.AnyNatsTestAccountID())
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NoError(k8sClient.Delete(t.ctx, account))

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockFindAccountID(t.ctx, mock.Anything, accountID, true, nil).Once()
	t.accountManagerMock.On("Delete", t.ctx, mock.MatchedBy(func(reference nauth.AccountReference) bool {
		return reference.AccountID == accountID
	})).Return(nil).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().Error(err)
	t.True(k8err.IsNotFound(err))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldRemoveFinalizer_WhenDeletingAccountWithoutManagedState() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
		}),
	)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NoError(k8sClient.Delete(t.ctx, account))

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockFindAccountID(t.ctx, mock.Anything, "", false, nil).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.accountManagerMock.AssertNotCalled(t.T(), "Delete", mock.Anything, mock.Anything)

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().Error(err)
	t.True(k8err.IsNotFound(err))
}

func createDummyClusterTarget() *nauth.ClusterTarget {
	sauCreds := domain.NatsUserCreds{
		Creds:     []byte("FAKE_CREDENTIALS"),
		AccountID: "FAKE_SYS_ACCOUNT_ID",
	}
	opSignKey := domain.NatsOperatorSigningKey(testutil.CreateNatsTestOperator().Sign.Key)
	clusterTarget, _ := nauth.NewClusterTarget(
		"UID",
		"nats://nats-cluster:4222",
		sauCreds,
		opSignKey,
	)
	return clusterTarget
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldFail_WhenDeleteFails() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, testutil.AnyNatsTestAccountID())
		}),
	)

	deletionErr := fmt.Errorf("Unable to delete account")
	// Delete it (to set deletion timestamp)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NoError(k8sClient.Delete(t.ctx, account))

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockDelete(t.ctx, mock.Anything, deletionErr).Once()

	// When (expect manager.Delete)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Error(err)
	t.Contains(err.Error(), deletionErr.Error())

	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)
	for _, c := range account.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}
	t.Require().Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, deletionErr.Error())
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldImportObservedAccount() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelManagementPolicy, v1alpha1.AccountManagementPolicyObserve)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
	}
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockImport(t.ctx, mock.Anything, mockResult).Once()

	// When (expect manager.Import)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenOperatorVersionChanges() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	newOperatorVersion := "1.1-SNAPSHOT"
	t.Require().NoError(os.Setenv(envOperatorVersion, newOperatorVersion))

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When (expect manager.CreateOrUpdate)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)

	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	c := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Equal(metav1.ConditionTrue, c.Status)
	t.Equal(conditionReasonReconciled, c.Reason)

	t.Equal(newOperatorVersion, account.Status.OperatorVersion)
	t.Empty(t.fakeRecorder.Events)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSucceed_WhenAccountExportsExist() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	t.accountManagerMock.AssertExpectations(t.T())
	export1 := t.createExport(domain.Namespace(t.accountNamespace), "export-1", accountID, t.anyExportClaim(10))
	_ = t.createExport("ns-other", "export-2", accountID, t.anyExportClaim(20))
	export3 := t.createExport(domain.Namespace(t.accountNamespace), "export-3", accountID, nil) // Not expected into manager
	_ = t.createExport(domain.Namespace(t.accountNamespace), "export-4", "ANOTHERACCOUNT", t.anyExportClaim(40))
	export5 := t.createExport(domain.Namespace(t.accountNamespace), "export-5", accountID, t.anyExportClaim(50))
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(request nauth.AccountRequest) (*nauth.AccountResult, error) {
		adoptions := nauth.NewAccountAdoptions()
		t.Require().Equalf(2, len(request.ExportGroups), "expected 2 export groups: export-1 and export-5")
		for _, exportGroup := range request.ExportGroups {
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
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))

	c := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Equal(metav1.ConditionTrue, c.Status)
	t.Equal(conditionReasonReconciled, c.Reason)

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
					Message:                        conditionMessageAdopted,
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
					Message:                        conditionMessageAdopted,
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
type accountManagerMock struct {
	mock.Mock
}

func (o *accountManagerMock) CreateOrUpdate(ctx context.Context, request nauth.AccountRequest) (*nauth.AccountResult, error) {
	args := o.Called(ctx, request)
	result := args.Get(0)
	if result != nil {
		return result.(*nauth.AccountResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (o *accountManagerMock) mockCreateOrUpdate(ctx interface{}, resources interface{}, result *nauth.AccountResult) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(result, nil)
	return call
}

func (o *accountManagerMock) mockCreateOrUpdateFn(ctx interface{}, resources interface{}, fn func(request nauth.AccountRequest) (*nauth.AccountResult, error)) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Run(func(args mock.Arguments) {
		result, err := fn(args.Get(1).(nauth.AccountRequest))
		call.Return(result, err)
	})
	return call
}

func (o *accountManagerMock) mockCreateOrUpdateError(ctx interface{}, resources interface{}, err error) *mock.Call {
	call := o.On("CreateOrUpdate", ctx, resources)
	call.Return(nil, err)
	return call
}

func (o *accountManagerMock) Import(ctx context.Context, reference nauth.AccountReference) (*nauth.AccountResult, error) {
	args := o.Called(ctx, reference)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*nauth.AccountResult), nil
}

func (o *accountManagerMock) FindAccountID(ctx context.Context, reference nauth.AccountReference) (nauth.AccountID, bool, error) {
	args := o.Called(ctx, reference)
	return args.Get(0).(nauth.AccountID), args.Bool(1), args.Error(2)
}

func (o *accountManagerMock) mockFindAccountID(ctx interface{}, state interface{}, result nauth.AccountID, found bool, err error) *mock.Call {
	call := o.On("FindAccountID", ctx, state)
	call.Return(result, found, err)
	return call
}

func (o *accountManagerMock) Delete(ctx context.Context, reference nauth.AccountReference) error {
	args := o.Called(ctx, reference)
	return args.Error(0)
}

func (o *accountManagerMock) mockDelete(ctx interface{}, state interface{}, err error) *mock.Call {
	call := o.On("Delete", ctx, state)
	call.Return(err)
	return call
}

func (o *accountManagerMock) mockImport(ctx interface{}, state interface{}, result *nauth.AccountResult) *mock.Call {
	call := o.On("Import", ctx, state)
	call.Return(result, nil)
	return call
}

var _ inbound.AccountManager = (*accountManagerMock)(nil)
