/*
Copyright 2026.

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
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/core"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AccountSigningKeyControllerTestSuite struct {
	suite.Suite
	ctx       context.Context
	name      string
	namespace string
	ref       ktypes.NamespacedName

	managerMock   *AccountSigningKeyManagerMock
	unitUnderTest *AccountSigningKeyReconciler
}

func TestAccountSigningKeyController_TestSuite(t *testing.T) {
	suite.Run(t, new(AccountSigningKeyControllerTestSuite))
}

func (t *AccountSigningKeyControllerTestSuite) SetupTest() {
	t.ctx = context.Background()

	testName := t.T().Name()
	t.name = testutil.ScopedTestName("test-ask", testName)
	t.namespace = testutil.ScopedTestName("ns", testName)
	t.ref = ktypes.NamespacedName{Name: t.name, Namespace: t.namespace}

	t.Require().NoError(ensureNamespace(t.ctx, t.namespace))

	t.managerMock = &AccountSigningKeyManagerMock{}
	t.unitUnderTest = NewAccountSigningKeyReconciler(k8sClient, k8sClient.Scheme(), t.managerMock)
}

func (t *AccountSigningKeyControllerTestSuite) TearDownTest() {
	t.managerMock.AssertExpectations(t.T())
}

// --- Managed mode ---

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldGenerateKey_WithDefaultSecretName() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, t.newSigningKey(v1alpha1.AccountSigningKeySpec{})))

	expectedSecretName := t.name + "-ac-sign"
	expectedPublicKey := "AXXXPUBLICKEY"
	t.managerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).
		Return(&nauth.AccountSigningKeyResult{PublicKey: expectedPublicKey, SecretName: expectedSecretName}, nil).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionTrue, conditionReasonReconciled)
	t.Require().Equal(expectedPublicKey, state.Status.PublicKey)
	t.Require().Equal(expectedSecretName, state.Status.SecretName)
	t.Require().Empty(state.Status.ManagementPolicy)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldGenerateKey_WithCustomSecretName() {
	// Given
	customName := "my-sign-secret"
	t.Require().NoError(k8sClient.Create(t.ctx, t.newSigningKey(v1alpha1.AccountSigningKeySpec{SecretName: customName})))

	t.managerMock.On("CreateOrUpdate", mock.Anything,
		mock.MatchedBy(func(req nauth.AccountSigningKeyRequest) bool {
			return req.SecretRef.Name == customName
		})).
		Return(&nauth.AccountSigningKeyResult{PublicKey: "AXXXKEY", SecretName: customName}, nil).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.Require().Equal(customName, state.Status.SecretName)
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionTrue, conditionReasonReconciled)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldRejectConflict_WhenUnownedSecretExists() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, t.newSigningKey(v1alpha1.AccountSigningKeySpec{})))

	t.managerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: secret conflict", core.ErrSigningKeyConflict)).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)
	t.assertCondition(t.getSigningKey().Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonConflict)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldFail_WhenExistingOwnedSecretHasInvalidSeed() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, t.newSigningKey(v1alpha1.AccountSigningKeySpec{})))

	t.managerMock.On("CreateOrUpdate", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: bad seed", core.ErrInvalidSeed)).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonInvalid)
	t.Require().Empty(state.Status.PublicKey)
}

// --- Observe mode ---

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldReadExistingSecret_InObserveMode() {
	// Given
	key := testutil.CreateNatsTestAccountKey()
	secretName := "existing-sign-secret"
	t.Require().NoError(k8sClient.Create(t.ctx, t.newObserveSigningKey(v1alpha1.AccountSigningKeySpec{SecretName: secretName})))

	t.managerMock.On("Import", mock.Anything,
		mock.MatchedBy(func(ref domain.NamespacedName) bool {
			return ref.Name == secretName
		})).
		Return(&nauth.AccountSigningKeyResult{PublicKey: key.PublicKey, SecretName: secretName}, nil).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionTrue, conditionReasonReconciled)
	t.Require().Equal(key.PublicKey, state.Status.PublicKey)
	t.Require().Equal(secretName, state.Status.SecretName)
	t.Require().Equal(k8s.ManagementPolicyObserve, state.Status.ManagementPolicy)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldFail_WhenObservedSecretMissing() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, t.newObserveSigningKey(v1alpha1.AccountSigningKeySpec{SecretName: "does-not-exist"})))

	t.managerMock.On("Import", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: does-not-exist", core.ErrSecretNotFound)).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonNotFound)
	t.Require().Empty(state.Status.PublicKey)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldFail_InObserveMode_WhenSecretNameMissing() {
	// Given: observe label set but spec.secretName left empty.
	// The reconciler must reject this without invoking the manager — observe mode
	// never falls back to the managed default Secret name.
	t.Require().NoError(k8sClient.Create(t.ctx, t.newObserveSigningKey(v1alpha1.AccountSigningKeySpec{})))

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)

	state := t.getSigningKey()
	t.assertCondition(state.Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonInvalid)
	t.Require().Empty(state.Status.PublicKey)
	t.Require().Empty(state.Status.SecretName)
	t.managerMock.AssertNotCalled(t.T(), "Import", mock.Anything, mock.Anything)
	t.managerMock.AssertNotCalled(t.T(), "CreateOrUpdate", mock.Anything, mock.Anything)
}

func (t *AccountSigningKeyControllerTestSuite) Test_Reconcile_ShouldFail_WhenObservedSecretHasInvalidSeed() {
	// Given
	t.Require().NoError(k8sClient.Create(t.ctx, t.newObserveSigningKey(v1alpha1.AccountSigningKeySpec{SecretName: "bad-seed-secret"})))

	t.managerMock.On("Import", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("%w: bad seed", core.ErrInvalidSeed)).Once()

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.ref})

	// Then
	t.Require().NoError(err)
	t.assertCondition(t.getSigningKey().Status.Conditions, conditionTypeReady, metav1.ConditionFalse, conditionReasonInvalid)
}

// --- helpers ---

func (t *AccountSigningKeyControllerTestSuite) newSigningKey(spec v1alpha1.AccountSigningKeySpec) *v1alpha1.AccountSigningKey {
	return &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.name,
			Namespace: t.namespace,
		},
		Spec: spec,
	}
}

func (t *AccountSigningKeyControllerTestSuite) newObserveSigningKey(spec v1alpha1.AccountSigningKeySpec) *v1alpha1.AccountSigningKey {
	return &v1alpha1.AccountSigningKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.name,
			Namespace: t.namespace,
			Labels: map[string]string{
				k8s.LabelManagementPolicy: k8s.ManagementPolicyObserve,
			},
		},
		Spec: spec,
	}
}

func (t *AccountSigningKeyControllerTestSuite) getSigningKey() *v1alpha1.AccountSigningKey {
	t.T().Helper()
	state := &v1alpha1.AccountSigningKey{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.ref, state))
	return state
}

func (t *AccountSigningKeyControllerTestSuite) assertCondition(
	conditions []metav1.Condition,
	condType string,
	wantStatus metav1.ConditionStatus,
	wantReason metav1.StatusReason,
) metav1.Condition {
	t.T().Helper()
	for _, c := range conditions {
		if c.Type == condType {
			t.Require().Equal(
				fmt.Sprintf("%s|%s|%s", condType, wantStatus, wantReason),
				fmt.Sprintf("%s|%s|%s", c.Type, c.Status, c.Reason),
			)
			return c
		}
	}
	t.Failf("condition not found", "condition %q not found in %v", condType, conditions)
	return metav1.Condition{}
}

// AccountSigningKeyManagerMock mocks inbound.AccountSigningKeyManager.
type AccountSigningKeyManagerMock struct {
	mock.Mock
}

func (m *AccountSigningKeyManagerMock) CreateOrUpdate(ctx context.Context, req nauth.AccountSigningKeyRequest) (*nauth.AccountSigningKeyResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nauth.AccountSigningKeyResult), args.Error(1)
}

func (m *AccountSigningKeyManagerMock) Import(ctx context.Context, secretRef domain.NamespacedName) (*nauth.AccountSigningKeyResult, error) {
	args := m.Called(ctx, secretRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nauth.AccountSigningKeyResult), args.Error(1)
}

var _ inbound.AccountSigningKeyManager = (*AccountSigningKeyManagerMock)(nil)
