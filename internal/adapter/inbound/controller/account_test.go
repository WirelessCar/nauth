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
	"time"

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

const testAccountReconciliationInterval = time.Minute

func TestAccountNatsCompleteCondition(t *testing.T) {
	tests := []struct {
		name       string
		state      domain.NatsAccountState
		wantStatus metav1.ConditionStatus
	}{
		{
			name: "unknown state remains unknown even when the hash matches",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateUnknown,
				ClaimsHash: "desired-hash",
			},
			wantStatus: metav1.ConditionUnknown,
		},
		{
			name: "incomplete state is false when the hash matches",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateIncomplete,
				ClaimsHash: "desired-hash",
			},
			wantStatus: metav1.ConditionFalse,
		},
		{
			name: "complete state is false when the hash differs",
			state: domain.NatsAccountState{
				Status:     domain.NatsAccountStateComplete,
				ClaimsHash: "observed-hash",
			},
			wantStatus: metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := accountNatsCompleteCondition(&nauth.AccountResult{
				State:     nauth.AccountState{ClaimsHash: "desired-hash"},
				NatsState: &tt.state,
			})
			if condition.Status != tt.wantStatus {
				t.Fatalf("expected status %q, got %q", tt.wantStatus, condition.Status)
			}
		})
	}
}

func TestIncompleteAccountMessage(t *testing.T) {
	desiredImport := &nauth.Import{
		AccountID: "EXPORT_ACCOUNT",
		Subject:   "hello.world",
		Type:      nauth.ExportTypeStream,
	}
	tests := []struct {
		name           string
		state          domain.NatsAccountState
		desiredImports nauth.Imports
		wantMessage    string
	}{
		{
			name: "reports desired import omitted by ACCOUNTZ without inferring a cause",
			state: domain.NatsAccountState{
				Status: domain.NatsAccountStateIncomplete,
			},
			desiredImports: nauth.Imports{desiredImport},
			wantMessage:    "NATS did not report desired imports: EXPORT_ACCOUNT -> hello.world (stream)",
		},
		{
			name: "matches an observed import with its default local subject",
			state: domain.NatsAccountState{
				Status: domain.NatsAccountStateIncomplete,
				Imports: []domain.NatsAccountImport{{
					AccountID:    "EXPORT_ACCOUNT",
					Subject:      "hello.world",
					LocalSubject: "hello.world",
					Type:         "stream",
				}},
			},
			desiredImports: nauth.Imports{desiredImport},
			wantMessage:    "NATS reports the Account as incomplete",
		},
		{
			name: "reports an observed invalid import without also calling it missing",
			state: domain.NatsAccountState{
				Status: domain.NatsAccountStateIncomplete,
				Imports: []domain.NatsAccountImport{{
					AccountID: "EXPORT_ACCOUNT",
					Subject:   "hello.world",
					Type:      "stream",
					Invalid:   true,
				}},
			},
			desiredImports: nauth.Imports{desiredImport},
			wantMessage:    "NATS reports invalid imports: EXPORT_ACCOUNT -> hello.world (stream)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := incompleteAccountMessage(&tt.state, tt.desiredImports); got != tt.wantMessage {
				t.Fatalf("incompleteAccountMessage() = %q, want %q", got, tt.wantMessage)
			}
		})
	}
}

func TestAccountRequeueAfterValidationOutcome(t *testing.T) {
	tests := []struct {
		name              string
		validationOutcome nauth.AccountValidationOutcome
		shortRequeue      bool
	}{
		{
			name:              "pending validation",
			validationOutcome: nauth.AccountValidationPending,
			shortRequeue:      true,
		},
		{
			name:              "unknown validation",
			validationOutcome: nauth.AccountValidationUnknown,
			shortRequeue:      true,
		},
		{
			name:              "ready validation",
			validationOutcome: nauth.AccountValidationReady,
		},
	}

	reconciler := &AccountReconciler{accountReconciliationInterval: testAccountReconciliationInterval}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconciler.requeueAfterValidation(tt.validationOutcome)
			if tt.shortRequeue {
				if got != requeuePendingAccountValidation {
					t.Fatalf("expected requeue after %s, got %s", requeuePendingAccountValidation, got)
				}
				return
			}
			minimum := testAccountReconciliationInterval * 9 / 10
			maximum := testAccountReconciliationInterval * 11 / 10
			if got < minimum || got > maximum {
				t.Fatalf("expected requeue between %s and %s, got %s", minimum, maximum, got)
			}
		})
	}
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
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = t.newAccountReconciler(false)

	t.Require().NoError(ensureNamespace(t.ctx, t.operatorNamespace))
	t.Require().NoError(ensureNamespace(t.ctx, t.accountNamespace))
}

func (t *AccountControllerTestSuite) newAccountReconciler(allowAccountNatsClusterRebind bool) *AccountReconciler {
	accountClient := k8s.NewAccountClient(k8sClient)
	return NewAccountReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.accountManagerMock,
		t.clusterManagerMock,
		accountClient,
		t.fakeRecorder,
		testAccountReconciliationInterval,
		allowAccountNatsClusterRebind,
	)
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

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldBootstrap_WhenCreatingAccount() {
	// Given
	importLimit := int64(3)
	streamLimit := int64(5)
	subLimit := int64(7)
	unlimited := int64(-1)
	wildcardExports := true
	maxBytesRequired := false
	accountID := testutil.AnyNatsTestAccountID()

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
						Name:      "export-account",
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
		}, nil
	}).Once()

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.Equal(requeueImmediately, result.RequeueAfter)

	account := &v1alpha1.Account{}
	err = k8sClient.Get(t.ctx, t.accountNamespacedRef, account)
	t.Require().NoError(err)

	t.Equal(accountID, account.GetLabel(v1alpha1.AccountLabelAccountID))
	t.Equal("OPERATOR_SIGNING_KEY", account.GetLabel(v1alpha1.AccountLabelSignedBy))
	t.Equal(createDummyClusterTarget().UID, account.GetLabel(v1alpha1.AccountLabelNatsClusterID))
	t.Empty(account.Status.ClaimsHash)
	t.Empty(account.Status.OperatorVersion)
	t.Nil(meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady))
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
	t.Contains(<-t.fakeRecorder.Events, "failed to bootstrap account: a test error")
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

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldAllowChangingNatsCluster_WhenConfigured() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelNatsClusterID, "natscluster1")
		}),
	)
	t.unitUnderTest = t.newAccountReconciler(true)

	target := createDummyClusterTarget()
	target.UID = "natscluster2"
	t.clusterManagerMock.mockGetClusterTarget(target, nil)
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, &nauth.AccountResult{
		AccountID:       "account-id",
		AccountSignedBy: "operator-signing-key",
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)

	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Equal("natscluster2", account.GetLabel(v1alpha1.AccountLabelNatsClusterID))
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotDeleteObservedAccount() {
	// Given
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabel(k8s.LabelManagementPolicy), k8s.ManagementPolicyObserve)
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
			account.SetLabel(v1alpha1.AccountLabel(k8s.LabelManagementPolicy), k8s.ManagementPolicyObserve)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims:          &nauth.AccountClaims{},
		State: nauth.AccountState{
			ClaimsHash:         "claims-hash",
			ObservedServerID:   "server-a",
			ObservedClaimsHash: "claims-hash",
			ObservedStatus:     domain.NatsAccountStateComplete,
			StateValidatedAt:   time.Now(),
		},
		ValidationOutcome: nauth.AccountValidationReady,
		NatsState: &domain.NatsAccountState{
			Status:     domain.NatsAccountStateComplete,
			ServerID:   "server-a",
			AccountID:  accountID,
			ClaimsHash: "claims-hash",
		},
	}
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockImport(t.ctx, mock.Anything, mockResult).Once()

	// When (expect manager.Import)
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.NoError(err)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NotNil(account.Status.Nats)
	t.Equal("server-a", account.Status.Nats.ObservedServerID)
	t.Equal("claims-hash", account.Status.Nats.ObservedClaimsHash)
	t.assertAccountCondition(account.Status.Conditions, conditionTypeNatsAccountComplete, metav1.ConditionTrue, conditionReasonOK)
	t.assertAccountCondition(account.Status.Conditions, conditionTypeReady, metav1.ConditionTrue, conditionReasonReconciled)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldBeReadyAndRecordNATSStateValidatedAt_WhenManagerConfirmsValidation() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		State: nauth.AccountState{
			ClaimsHash:         "claims-hash",
			ObservedServerID:   "server-a",
			ObservedClaimsHash: "claims-hash",
			ObservedStatus:     domain.NatsAccountStateComplete,
			StateValidatedAt:   time.Now(),
		},
		ValidationOutcome: nauth.AccountValidationReady,
		NatsState: &domain.NatsAccountState{
			Status:     domain.NatsAccountStateComplete,
			ServerID:   "server-a",
			AccountID:  accountID,
			ClaimsHash: "claims-hash",
		},
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	minimum := testAccountReconciliationInterval * 9 / 10
	maximum := testAccountReconciliationInterval * 11 / 10
	t.True(result.RequeueAfter >= minimum, "requeue interval should be at least %s, got %s", minimum, result.RequeueAfter)
	t.True(result.RequeueAfter <= maximum, "requeue interval should be at most %s, got %s", maximum, result.RequeueAfter)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NotNil(account.Status.Nats)
	t.Equal("server-a", account.Status.Nats.ObservedServerID)
	t.Equal("claims-hash", account.Status.Nats.ObservedClaimsHash)
	t.False(account.Status.Nats.StateValidatedAt.IsZero())
	condition := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Require().NotNil(condition)
	t.Equal(metav1.ConditionTrue, condition.Status)
	t.Equal(conditionReasonReconciled, condition.Reason)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldNotBeReady_WhenNATSAccountIsIncomplete() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		Claims: &nauth.AccountClaims{
			Imports: nauth.Imports{{
				AccountID: "missing-export-account",
				Subject:   "not-exported.>",
				Type:      nauth.ExportTypeStream,
			}},
		},
		State: nauth.AccountState{
			ClaimsHash:         "claims-hash",
			ObservedServerID:   "server-a",
			ObservedClaimsHash: "claims-hash",
			ObservedStatus:     domain.NatsAccountStateComplete,
			StateValidatedAt:   time.Now(),
		},
		ValidationOutcome: nauth.AccountValidationPending,
		NatsState: &domain.NatsAccountState{
			Status:     domain.NatsAccountStateIncomplete,
			ServerID:   "server-a",
			AccountID:  accountID,
			ClaimsHash: "claims-hash",
			Imports: []domain.NatsAccountImport{{
				AccountID: "export-account",
				Subject:   "allowed.>",
				Type:      "stream",
				Invalid:   true,
			}},
		},
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.Equal(requeuePendingAccountValidation, result.RequeueAfter)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.assertAccountCondition(account.Status.Conditions, conditionTypeNatsAccountComplete, metav1.ConditionFalse, conditionReasonNotReady)
	t.assertAccountCondition(account.Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotReady)
	condition := meta.FindStatusCondition(account.Status.Conditions, conditionTypeNatsAccountComplete)
	t.Contains(condition.Message, "export-account -> allowed.> (stream)")
	t.Contains(condition.Message, "NATS did not report desired imports: missing-export-account -> not-exported.> (stream)")
	t.NotContains(condition.Message, "not exported")
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSetUnknownReadiness_WhenNATSAccountObservationIsInconclusive() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:              accountID,
		AccountSignedBy:        "OPERATOR_SIGNING_KEY",
		State:                  nauth.AccountState{ClaimsHash: "claims-hash"},
		ValidationOutcome:      nauth.AccountValidationUnknown,
		NatsState:              &domain.NatsAccountState{Status: domain.NatsAccountStateUnknown},
		NatsObservationMessage: "ACCOUNTZ is unavailable on connected NATS server \"server-a\" version \"2.0.0\"; account state is Unknown",
	}
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, mockResult).Once()
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	t.Equal(requeuePendingAccountValidation, result.RequeueAfter)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	natsCondition := meta.FindStatusCondition(account.Status.Conditions, conditionTypeNatsAccountComplete)
	t.Equal(metav1.ConditionUnknown, natsCondition.Status)
	t.Equal(mockResult.NatsObservationMessage, natsCondition.Message)
	readyCondition := meta.FindStatusCondition(account.Status.Conditions, conditionTypeReady)
	t.Equal(metav1.ConditionUnknown, readyCondition.Status)
	t.Equal(conditionReasonUnknown, readyCondition.Reason)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldPreserveNATSStateValidatedAt_WhenManagerSkipsValidation() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	acceptedAt := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
			account.Status.ClaimsHash = "claims-hash"
			account.Status.Nats = &v1alpha1.AccountNatsStatus{
				ObservedServerID:   "server-a",
				ObservedClaimsHash: "claims-hash",
				StateValidatedAt:   acceptedAt,
			}
			account.Status.Conditions = []metav1.Condition{
				{Type: conditionTypeNatsAccountComplete, Status: metav1.ConditionTrue, Reason: conditionReasonOK, LastTransitionTime: metav1.NewTime(time.Now().Truncate(time.Second))},
			}
		}),
	)

	mockResult := &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		State: nauth.AccountState{
			ClaimsHash:         "claims-hash",
			ObservedServerID:   "server-a",
			ObservedClaimsHash: "claims-hash",
			StateValidatedAt:   acceptedAt.Time,
		},
	}
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(request nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Equal("claims-hash", request.State.ClaimsHash)
		t.Equal(acceptedAt.Time, request.State.StateValidatedAt)
		return mockResult, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NotNil(account.Status.Nats)
	t.Equal(acceptedAt, account.Status.Nats.StateValidatedAt)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldPreserveNATSStateValidatedAt_WhenManagerFails() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	validatedAt := metav1.NewTime(time.Now().Add(-time.Minute).Truncate(time.Second))
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
			account.Status.Nats = &v1alpha1.AccountNatsStatus{StateValidatedAt: validatedAt}
		}),
	)

	managerErr := errors.New("NATS state validation failed")
	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateError(t.ctx, mock.Anything, managerErr).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().ErrorIs(err, managerErr)
	account := &v1alpha1.Account{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.accountNamespacedRef, account))
	t.Require().NotNil(account.Status.Nats)
	t.Equal(validatedAt, account.Status.Nats.StateValidatedAt)
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
		State:           nauth.AccountState{ClaimsHash: "claims-hash"},
		NatsState: &domain.NatsAccountState{
			Status:     domain.NatsAccountStateComplete,
			ServerID:   "server-a",
			AccountID:  accountID,
			ClaimsHash: "claims-hash",
		},
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

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldUseConfiguredAccountReconciliationInterval() {
	// Given
	accountID := testutil.AnyNatsTestAccountID()
	t.setupAccount(
		t.defaultAccount(func(account *v1alpha1.Account) {
			account.Finalizers = append(account.Finalizers, finalizerAccount)
			account.SetLabel(v1alpha1.AccountLabelAccountID, accountID)
		}),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdate(t.ctx, mock.Anything, &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: "OPERATOR_SIGNING_KEY",
		State: nauth.AccountState{
			ClaimsHash:         "claims-hash",
			ObservedClaimsHash: "claims-hash",
			ObservedStatus:     domain.NatsAccountStateComplete,
			StateValidatedAt:   time.Now(),
		},
		ValidationOutcome: nauth.AccountValidationReady,
	}).Once()

	// When
	result, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
	minimum := testAccountReconciliationInterval * 9 / 10
	maximum := testAccountReconciliationInterval * 11 / 10
	t.True(result.RequeueAfter >= minimum, "requeue interval should be at least %s, got %s", minimum, result.RequeueAfter)
	t.True(result.RequeueAfter <= maximum, "requeue interval should be at most %s, got %s", maximum, result.RequeueAfter)
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
			State:           nauth.AccountState{ClaimsHash: "claims-hash"},
			NatsState: &domain.NatsAccountState{
				Status:     domain.NatsAccountStateComplete,
				ServerID:   "server-a",
				AccountID:  accountID,
				ClaimsHash: "claims-hash",
			},
			Adoptions: adoptions,
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

func (t *AccountControllerTestSuite) assertAccountCondition(conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus, reason string) {
	condition := meta.FindStatusCondition(conditions, conditionType)
	t.Require().NotNil(condition)
	t.Equal(status, condition.Status)
	t.Equal(reason, condition.Reason)
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

// ---- AccountSigningKey tests ----

func (t *AccountControllerTestSuite) createReadySigningKeyInNamespace(namespace, name, publicKey string) *v1alpha1.AccountSigningKey {
	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	ask := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, ask))

	created := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), created))
	created.Status = v1alpha1.AccountSigningKeyStatus{
		PublicKey: publicKey,
		Conditions: []metav1.Condition{
			{Type: conditionTypeReady, Status: metav1.ConditionTrue, Reason: conditionReasonOK, LastTransitionTime: metav1.Now()},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, created))

	result := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), result))
	return result
}

func (t *AccountControllerTestSuite) createReadySigningKey(name, publicKey string) *v1alpha1.AccountSigningKey {
	ask := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.accountNamespace,
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, ask))

	created := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), created))
	created.Status = v1alpha1.AccountSigningKeyStatus{
		PublicKey: publicKey,
		Conditions: []metav1.Condition{
			{Type: conditionTypeReady, Status: metav1.ConditionTrue, Reason: conditionReasonOK, LastTransitionTime: metav1.Now()},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, created))

	result := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), result))
	return result
}

func (t *AccountControllerTestSuite) createNotReadySigningKey(name string) *v1alpha1.AccountSigningKey {
	ask := &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: t.accountNamespace,
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, ask))

	created := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), created))
	created.Status = v1alpha1.AccountSigningKeyStatus{
		Conditions: []metav1.Condition{
			{Type: conditionTypeReady, Status: metav1.ConditionFalse, Reason: conditionReasonReconciling, LastTransitionTime: metav1.Now()},
		},
	}
	t.Require().NoError(k8sClient.Status().Update(t.ctx, created))

	result := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKeyFromObject(ask), result))
	return result
}

func (t *AccountControllerTestSuite) accountWithSigningKeyRefs(refs ...string) accountOption {
	return func(account *v1alpha1.Account) {
		out := make([]v1alpha1.AccountSigningKeyRef, 0, len(refs))
		for _, r := range refs {
			out = append(out, v1alpha1.AccountSigningKeyRef{Kind: v1alpha1.AccountSigningKeyRefKindAccountSigningKey, Name: r})
		}
		account.Spec.SigningKeyRefs = out
		account.SetLabel(v1alpha1.AccountLabelAccountID, testutil.AnyNatsTestAccountID())
	}
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldPassNilSigningKeys_WhenSigningKeyRefsIsNilOrEmpty() {
	// Given — no signingKeyRefs set. An empty slice is identical to nil after k8s API serialisation
	// (the field has omitempty), so a single test covers both cases.
	t.setupAccount(
		t.defaultAccount(func(a *v1alpha1.Account) {
			a.Finalizers = append(a.Finalizers, finalizerAccount)
			// Spec.SigningKeyRefs is nil (zero value)
		}),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Empty(req.SigningKeys, "implicit mode: SigningKeys must be nil/empty")
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldPassSigningKeysToRequest_WhenSingleRefReady() {
	// Given
	signingKey := testutil.CreateNatsTestAccountKey()
	t.createReadySigningKey("signing-key-1", signingKey.PublicKey)
	t.setupAccount(
		t.defaultAccount(
			func(a *v1alpha1.Account) { a.Finalizers = append(a.Finalizers, finalizerAccount) },
			t.accountWithSigningKeyRefs("signing-key-1"),
		),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Equal([]string{signingKey.PublicKey}, req.SigningKeys)
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldPassSigningKeysToRequest_WhenMultipleRefsReady() {
	// Given
	key1 := testutil.CreateNatsTestAccountKey()
	key2 := testutil.CreateNatsTestAccountKey()
	t.createReadySigningKey("signing-key-a", key1.PublicKey)
	t.createReadySigningKey("signing-key-b", key2.PublicKey)
	t.setupAccount(
		t.defaultAccount(
			func(a *v1alpha1.Account) { a.Finalizers = append(a.Finalizers, finalizerAccount) },
			t.accountWithSigningKeyRefs("signing-key-a", "signing-key-b"),
		),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Equal([]string{key1.PublicKey, key2.PublicKey}, req.SigningKeys)
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldResolveCrossNamespaceSigningKeyRef() {
	// Given an AccountSigningKey in a different namespace referenced explicitly via ref.namespace.
	sharedNamespace := testutil.ScopedTestName("shared", t.T().Name())
	signingKey := testutil.CreateNatsTestAccountKey()
	t.createReadySigningKeyInNamespace(sharedNamespace, "shared-key", signingKey.PublicKey)

	t.setupAccount(
		t.defaultAccount(
			func(a *v1alpha1.Account) {
				a.Finalizers = append(a.Finalizers, finalizerAccount)
				a.Spec.SigningKeyRefs = []v1alpha1.AccountSigningKeyRef{
					{Kind: v1alpha1.AccountSigningKeyRefKindAccountSigningKey, Name: "shared-key", Namespace: sharedNamespace},
				}
				a.SetLabel(v1alpha1.AccountLabelAccountID, testutil.AnyNatsTestAccountID())
			},
		),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Equal([]string{signingKey.PublicKey}, req.SigningKeys)
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSkipMissingSigningKeyRef() {
	// Given — no AccountSigningKey resource created. Mirrors AccountImport/AccountExport
	// behavior: a sub-resource that is absent (or not yet Ready) is silently skipped and
	// the Account reconciles with whatever keys are available.
	t.setupAccount(
		t.defaultAccount(
			func(a *v1alpha1.Account) { a.Finalizers = append(a.Finalizers, finalizerAccount) },
			t.accountWithSigningKeyRefs("missing-key"),
		),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Empty(req.SigningKeys)
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_Reconcile_ShouldSkipNotReadySigningKeyRef() {
	// Given — signing key exists but Ready=False. Same model as missing: silently skipped
	// and the Account reconciles. The AccountSigningKey watch will re-enqueue once Ready.
	t.createNotReadySigningKey("not-ready-key")
	t.setupAccount(
		t.defaultAccount(
			func(a *v1alpha1.Account) { a.Finalizers = append(a.Finalizers, finalizerAccount) },
			t.accountWithSigningKeyRefs("not-ready-key"),
		),
	)

	t.clusterManagerMock.mockGetClusterTarget(createDummyClusterTarget(), nil)
	t.accountManagerMock.mockCreateOrUpdateFn(t.ctx, mock.Anything, func(req nauth.AccountRequest) (*nauth.AccountResult, error) {
		t.Require().Empty(req.SigningKeys)
		return &nauth.AccountResult{AccountID: testutil.AnyNatsTestAccountID(), Claims: &nauth.AccountClaims{}}, nil
	}).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.accountNamespacedRef})

	// Then
	t.Require().NoError(err)
}

func (t *AccountControllerTestSuite) Test_ResolveSigningKeyRefs_ShouldRejectUnsupportedKind() {
	// Defense in depth: the CRD enum already rejects unknown kinds at admission, but the
	// controller validates again so a stale/handwritten spec can never silently fetch the
	// wrong resource type. The unit-level call here bypasses admission to exercise it.
	_, err := t.unitUnderTest.resolveSigningKeyRefs(t.ctx, t.accountNamespace, []v1alpha1.AccountSigningKeyRef{
		{Kind: "AccountSigningKeyProvider", Name: "irrelevant"},
	})
	t.Require().Error(err)
	t.ErrorIs(err, errInvalidSigningKeyRefKind)
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
