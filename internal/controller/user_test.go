package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/system"
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

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

var _ = Describe("User Controller", func() {
	Context("When reconciling a user", func() {
		const (
			userBaseName    = "test-user"
			accountBaseName = "test-account"
			userNamespace   = "test-user-namespace"
			accountID       = "ATESTACCOUNTID123"
		)

		// Suite context variables
		var (
			resolverMock         *ResolverMock
			providerMock         *ProviderMock
			controllerReconciler *UserReconciler
			userNamespacedName   ktypes.NamespacedName
			accountName          string
			fakeRecorder         *record.FakeRecorder
			testIndex            int
			operatorVersion      string
		)

		ctx := context.Background()

		BeforeEach(func() {
			fmt.Printf("ENV OV=%s\n", os.Getenv(EnvOperatorVersion))
			operatorVersion = "0.0-SNAPSHOT"
			_ = os.Setenv(EnvOperatorVersion, operatorVersion)

			providerMock = &ProviderMock{}
			resolverMock = &ResolverMock{}
			resolverMock.On("ResolveForAccount", mock.Anything, mock.Anything).Return(providerMock, nil)

			testIndex += 1
			userName := fmt.Sprintf("%s-%d", userBaseName, testIndex)
			accountName = fmt.Sprintf("%s-%d", accountBaseName, testIndex)
			userNamespacedName = ktypes.NamespacedName{
				Name:      userName,
				Namespace: userNamespace,
			}

			By("setting up the controller")
			fakeRecorder = record.NewFakeRecorder(5)
			controllerReconciler = NewUserReconciler(
				k8sClient,
				k8sClient.Scheme(),
				resolverMock,
				fakeRecorder,
			)

			By("ensuring the namespace exists")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: userNamespacedName.Namespace,
				},
			}
			_ = k8sClient.Create(ctx, ns)

			By("creating the account for the user")
			account := &natsv1alpha1.Account{
				ObjectMeta: metav1.ObjectMeta{
					Name:      accountName,
					Namespace: userNamespace,
					Labels: map[string]string{
						k8s.LabelAccountID: accountID,
					},
				},
			}
			err := k8sClient.Create(ctx, account)
			if err != nil && !k8err.IsAlreadyExists(err) {
				Expect(err).ToNot(HaveOccurred())
			}

			By("creating the custom user for the Kind User")
			user := &natsv1alpha1.User{}
			err = k8sClient.Get(ctx, userNamespacedName, user)
			if err != nil && k8err.IsNotFound(err) {
				user := &natsv1alpha1.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: userNamespace,
					},
					Spec: natsv1alpha1.UserSpec{
						AccountName: accountName,
					},
				}
				Expect(k8sClient.Create(ctx, user)).To(Succeed())
			}
		})

		AfterEach(func() {
			providerMock.AssertExpectations(GinkgoT())
			_ = os.Unsetenv(EnvOperatorVersion)
		})

		Context("User create/ update reconciliation", func() {
			It("should successfully reconcile the user", func() {
				By("Reconciling the created user")

				mockResult := &system.UserResult{
					UserID:       "UTESTUSERID123",
					UserSignedBy: "ACCOUNT_SIGNING_KEY",
					Claims:       &natsv1alpha1.UserClaims{},
				}
				providerMock.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				user := &natsv1alpha1.User{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range user.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(controllerReasonReconciled))
				}
				Expect(user.Status.OperatorVersion).To(Equal(operatorVersion))

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})

			It("should fail when trying to create a user and provider fails", func() {
				By("Not able to reconcile the created user due to provider error")

				providerErr := fmt.Errorf("provider error")
				providerMock.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(nil, providerErr).Once()

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(providerErr.Error()))

				user := &natsv1alpha1.User{}
				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range user.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(controllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring(providerErr.Error()))
			})
		})

		Context("User delete reconciliation", func() {
			It("should successfully remove the user marked for deletion", func() {
				mockResult := &system.UserResult{
					UserID:       "UTESTUSERID123",
					UserSignedBy: "ACCOUNT_SIGNING_KEY",
					Claims:       &natsv1alpha1.UserClaims{},
				}
				providerMock.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("DeleteUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
				user := &natsv1alpha1.User{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(user, controllerUserFinalizer)).To(BeTrue())

				err = k8sClient.Delete(ctx, user)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).ToNot(HaveOccurred())
				Expect(user.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				for _, c := range user.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(controllerReasonReconciled))
				}

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).To(HaveOccurred())
				Expect(k8err.IsNotFound(err)).To(BeTrue())

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})

			It("should fail to remove the user when delete client fails", func() {
				userDeleteError := fmt.Errorf("unable to remove the user")
				mockResult := &system.UserResult{
					UserID:       "UTESTUSERID123",
					UserSignedBy: "ACCOUNT_SIGNING_KEY",
					Claims:       &natsv1alpha1.UserClaims{},
				}
				providerMock.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("DeleteUser", mock.Anything, mock.Anything, mock.Anything).Return(userDeleteError).Once()
				user := &natsv1alpha1.User{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).ToNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(user, controllerUserFinalizer)).To(BeTrue())

				err = k8sClient.Delete(ctx, user)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).ToNot(HaveOccurred())
				Expect(user.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(userDeleteError.Error()))

				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).ToNot(HaveOccurred())
				for _, c := range user.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(controllerReasonErrored))
				}

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(HaveLen(1))
				Expect(<-fakeRecorder.Events).To(ContainSubstring(userDeleteError.Error()))

				// User is not deleted
				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("User update reconciliation", func() {
			It("should successfully reconcile the user", func() {
				By("Reconciling the created user")

				mockResult := &system.UserResult{
					UserID:       "UTESTUSERID123",
					UserSignedBy: "ACCOUNT_SIGNING_KEY",
					Claims:       &natsv1alpha1.UserClaims{},
				}
				providerMock.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				providerMock.On("UpdateUser", mock.Anything, mock.Anything, mock.Anything).Return(mockResult, nil).Once()
				user := &natsv1alpha1.User{}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				// Reconcile again to verify same ObservedGeneration and Generation
				newOperatorVersion := "1.1-SNAPSHOT"
				_ = os.Setenv(EnvOperatorVersion, newOperatorVersion)
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: userNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, userNamespacedName, user)
				Expect(err).NotTo(HaveOccurred())

				for _, c := range user.Status.Conditions {
					Expect(c.Status).To(Equal(metav1.ConditionTrue))
					Expect(c.Reason).To(Equal(controllerReasonReconciled))
				}
				Expect(user.Status.OperatorVersion).To(Equal(newOperatorVersion))

				By("Asserting the recorded events match the condition message")
				Expect(fakeRecorder.Events).To(BeEmpty())
			})
		})
	})
})
