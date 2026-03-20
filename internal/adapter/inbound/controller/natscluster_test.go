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

	. "github.com/onsi/ginkgo/v2" // TODO: [#183] Replace Ginkgo tests with Testify
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/WirelessCar/nauth/api/v1alpha1"
)

const (
	natsClusterBaseName  = "test-nats-cluster"
	natsClusterNamespace = "test-nats-cluster-namespace"
)

var _ = Describe("NatsCluster Controller", func() {
	Context("When reconciling a NatsCluster", func() {
		var (
			natsClusterManagerMock *NatsClusterManagerMock
			natsClusterName        string
			testIndex              int
			resourceName           ktypes.NamespacedName
			controllerReconciler   *NatsClusterReconciler
			fakeRecorder           *events.FakeRecorder
			operatorVersion        string
		)

		ctx := context.Background()

		BeforeEach(func() {
			operatorVersion = testOperatorVersion
			_ = os.Setenv(EnvOperatorVersion, operatorVersion)

			natsClusterManagerMock = &NatsClusterManagerMock{}

			testIndex += 1
			natsClusterName = fmt.Sprintf("%s-%d", natsClusterBaseName, testIndex)
			resourceName = ktypes.NamespacedName{
				Name:      natsClusterName,
				Namespace: natsClusterNamespace,
			}

			fakeRecorder = events.NewFakeRecorder(5)
			controllerReconciler = NewNatsClusterReconciler(
				k8sClient,
				k8sClient.Scheme(),
				natsClusterManagerMock,
				fakeRecorder,
			)

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: natsClusterNamespace}}
			_ = k8sClient.Create(ctx, ns)

			cluster := &v1alpha1.NatsCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      natsClusterName,
					Namespace: natsClusterNamespace,
				},
				Spec: v1alpha1.NatsClusterSpec{
					URL:                             "nats://my-cluster:4222",
					OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
					SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		})

		AfterEach(func() {
			natsClusterManagerMock.AssertExpectations(GinkgoT())
			_ = os.Unsetenv(EnvOperatorVersion)
		})

		It("should successfully reconcile the NatsCluster", func() {
			natsClusterManagerMock.On("Validate", mock.Anything).Return(nil).Once()

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: resourceName})
			Expect(err).NotTo(HaveOccurred())

			cluster := &v1alpha1.NatsCluster{}
			Expect(k8sClient.Get(ctx, resourceName, cluster)).To(Succeed())
			Expect(cluster.Status.OperatorVersion).To(Equal(operatorVersion))
			Expect(cluster.Status.ObservedGeneration).To(Equal(cluster.Generation))
			Expect(cluster.Status.ReconcileTimestamp.IsZero()).To(BeFalse())
			for _, c := range cluster.Status.Conditions {
				Expect(c.Status).To(Equal(metav1.ConditionTrue))
				Expect(c.Reason).To(Equal(controllerReasonReconciled))
			}
			Expect(fakeRecorder.Events).To(BeEmpty())
		})

		It("should fail to reconcile the NatsCluster", func() {
			validateErr := fmt.Errorf("a test error")
			natsClusterManagerMock.On("Validate", mock.Anything).Return(validateErr).Once()

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: resourceName})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(validateErr.Error()))

			cluster := &v1alpha1.NatsCluster{}
			Expect(k8sClient.Get(ctx, resourceName, cluster)).To(Succeed())
			for _, c := range cluster.Status.Conditions {
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
				Expect(c.Reason).To(Equal(controllerReasonErrored))
			}
			Expect(fakeRecorder.Events).To(HaveLen(1))
			Expect(<-fakeRecorder.Events).To(ContainSubstring("failed to validate NatsCluster: a test error"))
		})

		It("should skip reconciliation when generation and operator version are unchanged", func() {
			natsClusterManagerMock.On("Validate", mock.Anything).Return(nil).Once()

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: resourceName})
			Expect(err).NotTo(HaveOccurred())

			cluster := &v1alpha1.NatsCluster{}
			Expect(k8sClient.Get(ctx, resourceName, cluster)).To(Succeed())
			Expect(cluster.Status.ObservedGeneration).To(Equal(cluster.Generation))

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: resourceName})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

type NatsClusterManagerMock struct {
	mock.Mock
}

func (m *NatsClusterManagerMock) Validate(ctx context.Context, state *v1alpha1.NatsCluster) error {
	args := m.Called(state)
	return args.Error(0)
}
