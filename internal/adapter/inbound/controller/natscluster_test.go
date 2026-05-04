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
	"fmt"
	"os"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type NatsClusterControllerTestSuite struct {
	suite.Suite
	ctx context.Context

	managerMock  *ClusterManagerMock
	resolverMock *ClusterResolverMock
	fakeRecorder *events.FakeRecorder

	resourceName    ktypes.NamespacedName
	operatorVersion string

	unitUnderTest *NatsClusterReconciler
}

func TestNatsClusterController_TestSuite(t *testing.T) {
	suite.Run(t, new(NatsClusterControllerTestSuite))
}

func (t *NatsClusterControllerTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.operatorVersion = testOperatorVersion
	t.Require().NoError(os.Setenv(envOperatorVersion, t.operatorVersion))

	testName := t.T().Name()
	namespace := scopedTestName("natscluster", testName)
	name := scopedTestName("test-nats-cluster", testName)
	t.resourceName = ktypes.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	t.managerMock = &ClusterManagerMock{}
	t.resolverMock = &ClusterResolverMock{}
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewNatsClusterReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.managerMock,
		t.resolverMock,
		t.fakeRecorder,
	)

	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.NatsClusterSpec{
			URL:                             "nats://my-cluster:4222",
			OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
		},
	}))
}

func (t *NatsClusterControllerTestSuite) TearDownTest() {
	t.managerMock.AssertExpectations(t.T())
	t.resolverMock.AssertExpectations(t.T())
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldSucceed() {
	// Given
	target := t.anyClusterTarget()
	t.resolverMock.mockResolveClusterTarget(&target, nil)

	var targetSpied *nauth.ClusterTarget
	t.managerMock.mockValidateSpy(func(target nauth.ClusterTarget) error {
		targetSpied = &target
		return nil
	})

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	// Then
	t.NoError(err)

	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.resourceName, cluster))
	t.Equal(t.operatorVersion, cluster.Status.OperatorVersion)
	t.Equal(cluster.Generation, cluster.Status.ObservedGeneration)
	t.False(cluster.Status.ReconcileTimestamp.IsZero())
	for _, c := range cluster.Status.Conditions {
		t.Equal(metav1.ConditionTrue, c.Status)
		t.Equal(conditionReasonReconciled, c.Reason)
	}
	t.Empty(t.fakeRecorder.Events)
	t.Require().NotNil(targetSpied, "expected manager.Validate to be called with a ClusterTarget")
	t.Equal(target, *targetSpied)
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldFail_WhenValidationFails() {
	// Given
	validateErr := fmt.Errorf("a test error")
	target := t.anyClusterTarget()
	t.resolverMock.mockResolveClusterTarget(&target, nil)
	var targetSpied *nauth.ClusterTarget
	t.managerMock.mockValidateSpy(func(target nauth.ClusterTarget) error {
		targetSpied = &target
		return validateErr
	})

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	// Then
	t.Error(err)
	t.Contains(err.Error(), validateErr.Error())

	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.resourceName, cluster))
	for _, c := range cluster.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}
	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, "failed to validate NatsCluster: a test error")
	t.Require().NotNil(targetSpied, "expected manager.Validate to be called with a ClusterTarget")
	t.Equal(target, *targetSpied)
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldFail_WhenResolveTargetFails() {
	// Given
	resolveErr := fmt.Errorf("a test error")
	t.resolverMock.mockResolveClusterTarget(nil, resolveErr)

	// When
	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	// Then
	t.Error(err)
	t.Contains(err.Error(), resolveErr.Error())

	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.resourceName, cluster))
	for _, c := range cluster.Status.Conditions {
		t.Equal(metav1.ConditionFalse, c.Status)
		t.Equal(conditionReasonErrored, c.Reason)
	}
	t.Len(t.fakeRecorder.Events, 1)
	t.Contains(<-t.fakeRecorder.Events, "failed to resolve NatsCluster target: a test error")
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldSkip_WhenGenerationAndOperatorVersionAreUnchanged() {
	// Given
	// Note: Expect during setup only
	target := t.anyClusterTarget()
	t.resolverMock.mockResolveClusterTarget(&target, nil)
	t.managerMock.mockValidate(nil)

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	t.Require().NoError(err)

	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.resourceName, cluster))
	t.Equal(cluster.Generation, cluster.Status.ObservedGeneration)

	// Note: assert mock calls during setup and reset for test case
	t.managerMock.AssertExpectations(t.T())

	// When (expect no manager calls)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	// Then
	t.NoError(err)
}

// Helpers

func (t *NatsClusterControllerTestSuite) anyClusterTarget() nauth.ClusterTarget {
	return nauth.ClusterTarget{NatsURL: fmt.Sprintf("nats://%s.my-cluster:4222", shortHash(t.T().Name()))}
}

type ClusterManagerMock struct {
	mock.Mock
}

func (m *ClusterManagerMock) Validate(ctx context.Context, target nauth.ClusterTarget) error {
	args := m.Called(ctx, target)
	return args.Error(0)
}

func (m *ClusterManagerMock) mockValidate(err error) {
	m.On("Validate", mock.Anything, mock.Anything).Return(err).Once()
}

func (m *ClusterManagerMock) mockValidateSpy(spy func(target nauth.ClusterTarget) error) {
	call := m.On("Validate", mock.Anything, mock.Anything).Once()
	call.Run(func(args mock.Arguments) { call.Return(spy(args.Get(1).(nauth.ClusterTarget))) })
}

var _ inbound.ClusterManager = (*ClusterManagerMock)(nil)

type ClusterResolverMock struct {
	mock.Mock
}

func (m *ClusterResolverMock) ResolveClusterTarget(ctx context.Context, cluster *v1alpha1.NatsCluster) (*nauth.ClusterTarget, error) {
	args := m.Called(ctx, cluster)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nauth.ClusterTarget), args.Error(1)
}

func (m *ClusterResolverMock) mockResolveClusterTarget(result *nauth.ClusterTarget, err error) {
	m.On("ResolveClusterTarget", mock.Anything, mock.Anything).Return(result, err).Once()
}

var _ ClusterResolver = (*ClusterResolverMock)(nil)
