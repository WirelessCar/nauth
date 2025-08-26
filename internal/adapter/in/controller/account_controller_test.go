/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
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

	"k8s.io/client-go/tools/record"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	natsv1alpha1 "github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
)

const (
	accountBaseName          = "test-resource"
	accountNamespace         = "test-namespace"
	accountOperatorNamespace = "nauth-account-system"
	accountPublicKey         = "ACSOMETHINGKEY"
)

var _ = Describe("Account Controller", func() {
	Context("When reconciling an account", func() {
		// Suite context variables
		var (
			accountManagerMock    *AccountManagerMock
			accountName           string
			testIndex             int
			accountNamespacedName ktypes.NamespacedName
			controllerReconciler  *AccountReconciler
			fakeRecorder          *record.FakeRecorder
		)

		ctx := context.Background()

		BeforeEach(func() {
			accountManagerMock = &AccountManagerMock{}

			testIndex += 1
			accountName = fmt.Sprintf("%s-%d", accountBaseName, testIndex)
			accountNamespacedName = ktypes.NamespacedName{
				Name:      accountName,
				Namespace: accountNamespace,
			}

			By("setting up the controller")
			fakeRecorder = record.NewFakeRecorder(5)
			controllerReconciler = NewAccountReconciler(
				k8sClient,
				k8sClient.Scheme(),
				accountManagerMock,
				fakeRecorder,
			)

			By("ensuring the namespace exists")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: accountOperatorNamespace,
				},
			}
			_ = k8sClient.Create(ctx, ns)

			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: accountNamespace,
				},
			}
			_ = k8sClient.Create(ctx, ns)

			By("creating the custom account for the Kind Account")
			account := &natsv1alpha1.Account{}
			err := k8sClient.Get(ctx, accountNamespacedName, account)
			if err != nil && k8err.IsNotFound(err) {
				account := &natsv1alpha1.Account{
					ObjectMeta: metav1.ObjectMeta{
						Name:      accountName,
						Namespace: accountNamespace,
					},
				}
				Expect(k8sClient.Create(ctx, account)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			accountManagerMock.AssertExpectations(GinkgoT())
		})

		Context("Account create reconciliation", func() {
			It("should successfully reconcile the account", func() {
				By("Reconciling the created account")

				accountManagerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(nil).Once()
				account := &natsv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(types.ControllerReasonReconciled))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})

			It("should fail to reconcile the account", func() {
				By("Failing to reconcile the account")

				accountsManagerErr := fmt.Errorf("a test error")
				accountManagerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(accountsManagerErr).Once()
				account := &natsv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, accountsManagerErr)).To(BeTrue())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(types.ControllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring("failed to create the account: a test error"))
			})
		})

		Context("Account delete reconciliation", func() {
			It("should successfully remove the account marked for deletion", func() {
				accountManagerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(nil).Once()
				accountManagerMock.On("DeleteAccount", mock.Anything, mock.Anything).Return(nil).Once()
				account := &natsv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(account, types.ControllerAccountFinalizer)).To(BeTrue())

				err = k8sClient.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(account.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).To(HaveOccurred())
				Expect(k8err.IsNotFound(err)).To(BeTrue())
			})

			It("should fail to remove the account when delete client fails", func() {
				deletionErr := fmt.Errorf("Unable to delete account")
				accountManagerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(nil).Once()
				accountManagerMock.On("DeleteAccount", mock.Anything, mock.Anything).Return(deletionErr).Once()
				account := &natsv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(account, types.ControllerAccountFinalizer)).To(BeTrue())

				err = k8sClient.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(deletionErr.Error()))

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(types.ControllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring(deletionErr.Error()))

				// Account is not deleted
				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

// MOCKS

type AccountManagerMock struct {
	mock.Mock
}

// CreateAccount implements AccountManager.
func (o *AccountManagerMock) CreateAccount(ctx context.Context, state *natsv1alpha1.Account) error {

	state.Labels = map[string]string{
		domain.LabelAccountId: accountPublicKey,
	}

	state.Status = natsv1alpha1.AccountStatus{
		SigningKey: natsv1alpha1.KeyInfo{
			Name: accountPublicKey,
		},
	}

	args := o.Called(state)
	return args.Error(0)
}

// CreateAccount implements AccountManager.
func (o *AccountManagerMock) UpdateAccount(ctx context.Context, state *natsv1alpha1.Account) error {

	state.Labels = map[string]string{
		domain.LabelAccountId: accountPublicKey,
	}

	state.Status = natsv1alpha1.AccountStatus{
		SigningKey: natsv1alpha1.KeyInfo{
			Name: accountPublicKey,
		},
	}

	args := o.Called(state)
	return args.Error(0)
}

// DeleteAccount implements AccountManager.
func (o *AccountManagerMock) DeleteAccount(ctx context.Context, desired *natsv1alpha1.Account) error {
	args := o.Called(desired)
	return args.Error(0)
}

// RefreshState implements AccountManager.
func (o *AccountManagerMock) RefreshState(ctx context.Context, observed *types.Account, desired *natsv1alpha1.Account) error {
	args := o.Called(observed, desired)
	return args.Error(0)
}
