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

	natsClusterManagerMock *NatsClusterManagerMock
	fakeRecorder           *events.FakeRecorder

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

	t.natsClusterManagerMock = &NatsClusterManagerMock{}
	t.fakeRecorder = events.NewFakeRecorder(5)
	t.unitUnderTest = NewNatsClusterReconciler(
		k8sClient,
		k8sClient.Scheme(),
		t.natsClusterManagerMock,
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
	t.natsClusterManagerMock.AssertExpectations(t.T())
	t.Require().NoError(os.Unsetenv(envOperatorVersion))
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldSucceed() {
	// Given
	t.natsClusterManagerMock.On("Validate", mock.Anything).Return(nil).Once()

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
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldFail() {
	// Given
	validateErr := fmt.Errorf("a test error")
	t.natsClusterManagerMock.On("Validate", mock.Anything).Return(validateErr).Once()

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
}

func (t *NatsClusterControllerTestSuite) Test_Reconcile_ShouldSkip_WhenGenerationAndOperatorVersionAreUnchanged() {
	// Given
	// Note: Expect manager.Validate during setup only
	t.natsClusterManagerMock.On("Validate", mock.Anything).Return(nil).Once()

	_, err := t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	t.Require().NoError(err)

	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, t.resourceName, cluster))
	t.Equal(cluster.Generation, cluster.Status.ObservedGeneration)

	// Note: assert mock calls during setup and reset for test case
	t.natsClusterManagerMock.AssertExpectations(t.T())

	// When (expect no manager calls)
	_, err = t.unitUnderTest.Reconcile(t.ctx, reconcile.Request{NamespacedName: t.resourceName})

	// Then
	t.NoError(err)
}

type NatsClusterManagerMock struct {
	mock.Mock
}

func (m *NatsClusterManagerMock) Validate(ctx context.Context, state *v1alpha1.NatsCluster) error {
	args := m.Called(state)
	return args.Error(0)
}
