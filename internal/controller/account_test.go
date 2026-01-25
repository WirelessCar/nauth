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
	"os"

	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	k8err "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
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
			resolverMock          *ResolverMock
			providerMock          *ProviderMock
			accountName           string
			testIndex             int
			accountNamespacedName ktypes.NamespacedName
			controllerReconciler  *AccountReconciler
			fakeRecorder          *events.FakeRecorder
			operatorVersion       string
		)

		ctx := context.Background()

		BeforeEach(func() {
			operatorVersion = "0.0-SNAPSHOT"
			_ = os.Setenv(EnvOperatorVersion, operatorVersion)

			providerMock = &ProviderMock{}
			resolverMock = &ResolverMock{}
			resolverMock.On("ResolveForAccount", mock.Anything, mock.Anything).Return(providerMock, nil)

			testIndex += 1
			accountName = fmt.Sprintf("%s-%d", accountBaseName, testIndex)
			accountNamespacedName = ktypes.NamespacedName{
				Name:      accountName,
				Namespace: accountNamespace,
			}

			By("setting up the controller")
			fakeRecorder = events.NewFakeRecorder(5)
			controllerReconciler = NewAccountReconciler(
				k8sClient,
				k8sClient.Scheme(),
				resolverMock,
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
			account := &nauthv1alpha1.Account{}
			err := k8sClient.Get(ctx, accountNamespacedName, account)
			if err != nil && k8err.IsNotFound(err) {
				account := &nauthv1alpha1.Account{
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
			providerMock.AssertExpectations(GinkgoT())
			_ = os.Unsetenv(EnvOperatorVersion)
		})

		Context("Account create reconciliation", func() {
			It("should successfully reconcile the account", func() {
				By("Reconciling the created account")

				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				account := &nauthv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(controllerReasonReconciled))
				}
				Expect(account.Status.OperatorVersion).To(Equal(operatorVersion))

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})

			It("should fail to reconcile the account", func() {
				By("Failing to reconcile the account")

				accountsManagerErr := fmt.Errorf("a test error")
				providerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(nil, accountsManagerErr).Once()
				account := &nauthv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, accountsManagerErr)).To(BeTrue())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(controllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring("failed to create the account: a test error"))
			})
		})

		Context("Account delete reconciliation", func() {
			It("should not remove account from manager in observe mode", func() {
				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("ImportAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()

				account := &nauthv1alpha1.Account{}
				err := k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())

				account.Labels = map[string]string{
					k8s.LabelManagementPolicy: k8s.LabelManagementPolicyObserveValue,
				}
				err = k8sClient.Update(ctx, account)
				Expect(err).ToNot(HaveOccurred())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

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
			It("should successfully remove the account marked for deletion", func() {
				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("DeleteAccount", mock.Anything, mock.Anything).Return(nil).Once()
				account := &nauthv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(account, controllerAccountFinalizer)).To(BeTrue())

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
				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("DeleteAccount", mock.Anything, mock.Anything).Return(deletionErr).Once()
				account := &nauthv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(account, controllerAccountFinalizer)).To(BeTrue())

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
					Expect(c.Reason).To(Equal(controllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring(deletionErr.Error()))

				// Account is not deleted
				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Account observe reconciliation", func() {
			It("should import account in observe mode", func() {
				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("ImportAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()

				account := &nauthv1alpha1.Account{}
				err := k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).ToNot(HaveOccurred())

				account.Labels = map[string]string{
					k8s.LabelManagementPolicy: k8s.LabelManagementPolicyObserveValue,
				}
				err = k8sClient.Update(ctx, account)
				Expect(err).ToNot(HaveOccurred())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Account update reconciliation", func() {
			It("should successfully reconcile the account when the operator version change", func() {
				By("Reconciling the created account")

				mockResult := &cluster.AccountResult{
					AccountID:       accountPublicKey,
					AccountSignedBy: "OPERATOR_SIGNING_KEY",
					Claims:          &nauthv1alpha1.AccountClaims{},
				}
				providerMock.On("CreateAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("UpdateAccount", mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				account := &nauthv1alpha1.Account{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				// Reconcile again to verify same ObservedGeneration and Generation
				newOperatorVersion := "1.1-SNAPSHOT"
				_ = os.Setenv(EnvOperatorVersion, newOperatorVersion)
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: accountNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, accountNamespacedName, account)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range account.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(controllerReasonReconciled))
				}
				Expect(account.Status.OperatorVersion).To(Equal(newOperatorVersion))

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})
		})
	})
})

// MOCKS

type ResolverMock struct {
	mock.Mock
}

func (r *ResolverMock) ResolveForAccount(ctx context.Context, account *nauthv1alpha1.Account) (cluster.Provider, error) {
	args := r.Called(ctx, account)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(cluster.Provider), args.Error(1)
}

type ProviderMock struct {
	mock.Mock
}

func (p *ProviderMock) CreateAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	args := p.Called(ctx, account)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*cluster.AccountResult), nil
}

func (p *ProviderMock) UpdateAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	args := p.Called(ctx, account)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*cluster.AccountResult), nil
}

func (p *ProviderMock) ImportAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	args := p.Called(ctx, account)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*cluster.AccountResult), nil
}

func (p *ProviderMock) DeleteAccount(ctx context.Context, account *nauthv1alpha1.Account) error {
	args := p.Called(ctx, account)
	return args.Error(0)
}

func (p *ProviderMock) CreateOrUpdateUser(ctx context.Context, user *nauthv1alpha1.User) (*cluster.UserResult, error) {
	args := p.Called(ctx, user)
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}
	if args.Get(0) == nil {
		return nil, nil
	}
	return args.Get(0).(*cluster.UserResult), nil
}

func (p *ProviderMock) DeleteUser(ctx context.Context, user *nauthv1alpha1.User) error {
	args := p.Called(ctx, user)
	return args.Error(0)
}
