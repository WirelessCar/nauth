package service

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/core/domain"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var userName = "user"

const (
	accountNameThatIsNotReady = "test-account-not-ready"
)

var _ = Describe("User manager", func() {
	Context("When handling NATS user resources", func() {
		var (
			ctx               = context.Background()
			userManager       *UserManager
			accountGetterMock *AccountGetterMock
			secretStorerMock  *SecretStorerMock
		)

		BeforeEach(func() {
			By("creating the user manager")
			secretStorerMock = NewSecretStorerMock()
			accountGetterMock = NewAccountGetterMock()
			userManager = NewUserManager(accountGetterMock, secretStorerMock)
		})

		AfterEach(func() {
			secretStorerMock.AssertExpectations(GinkgoT())
			accountGetterMock.AssertExpectations(GinkgoT())
		})

		It("creates a new user belonging to the correct account", func() {
			account := GetExistingAccount()
			user := GetNewUser()

			By("providing a user specification without any specific configuration")
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil)

			By("mocking preexisting account keys & CR")
			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			secretsList := &corev1.SecretList{
				Items: []corev1.Secret{
					{
						Data: map[string][]byte{
							domain.DefaultSecretKeyName: accountSigningSeed,
						},
					},
				},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, accountNamespace, mock.Anything).Return(secretsList, nil)

			By("User credentials are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserID]).Should(Satisfy(isUserPubKey))
		})

		It("creates a new user from an account with legacy secrets", func() {
			By("providing a user specification")
			user := GetNewUser()

			account := GetExistingAccount()

			By("mocking the secret storer")
			secretStorerMock.On("GetSecretsByLabels", mock.Anything, account.GetNamespace(), mock.Anything).Return(&corev1.SecretList{}, nil)

			accountKeyPair, _ := nkeys.CreateAccount()
			accountPublicKey, _ := accountKeyPair.PublicKey()
			accountSeed, _ := accountKeyPair.Seed()
			accountSecretValueMock := map[string]string{domain.DefaultSecretKeyName: string(accountSeed)}
			accountSecretNameMock := fmt.Sprintf(domain.DeprecatedSecretNameAccountRootTemplate, account.GetName())
			secretStorerMock.On("GetSecret", mock.Anything, account.GetNamespace(), accountSecretNameMock).Return(accountSecretValueMock, nil)
			accountSecretLabelsMock := map[string]string{
				domain.LabelAccountID:  accountPublicKey,
				domain.LabelSecretType: domain.SecretTypeAccountRoot,
				domain.LabelManaged:    domain.LabelManagedValue,
			}
			secretStorerMock.On("LabelSecret", mock.Anything, account.GetNamespace(), accountSecretNameMock, accountSecretLabelsMock).Return(nil)

			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			accountSigningSecretValueMock := map[string]string{domain.DefaultSecretKeyName: string(accountSigningSeed)}
			accountSigningSecretNameMock := fmt.Sprintf(domain.DeprecatedSecretNameAccountSignTemplate, account.GetName())
			secretStorerMock.On("GetSecret", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock).Return(accountSigningSecretValueMock, nil)
			accountSigningSecretLabelsMock := map[string]string{
				domain.LabelAccountID:  accountPublicKey,
				domain.LabelSecretType: domain.SecretTypeAccountSign,
				domain.LabelManaged:    domain.LabelManagedValue,
			}
			secretStorerMock.On("LabelSecret", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock, accountSigningSecretLabelsMock).Return(nil)

			By("mocking existing account")
			account.Status.SigningKey = v1alpha1.KeyInfo{
				Name: accountSigningPublicKey,
			}
			account.Labels = map[string]string{
				domain.LabelAccountID: accountPublicKey,
			}
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil)

			By("mock storing user credentials")
			secretStorerMock.On("ApplySecret", mock.Anything, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserID]).Should(Satisfy(isUserPubKey))
		})

		It("creates a new user and update settigs", func() {
			account := GetExistingAccount()
			user := GetNewUser()

			By("providing a user specification without any specific configuration")
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil).Twice()

			By("mocking preexisting account keys & CR")
			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			secretsList := &corev1.SecretList{
				Items: []corev1.Secret{
					{
						Data: map[string][]byte{
							domain.DefaultSecretKeyName: accountSigningSeed,
						},
					},
				},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, accountNamespace, mock.Anything).Return(secretsList, nil)

			By("User credentials are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.Status.Claims.AccountName).Should(Equal(user.Spec.AccountName))
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserID]).Should(Satisfy(isUserPubKey))

			user.Spec.NatsLimits = &v1alpha1.NatsLimits{
				Subs:    ptr.To[int64](100),
				Data:    ptr.To[int64](1024),
				Payload: ptr.To[int64](256),
			}

			err = userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserID]).Should(Satisfy(isUserPubKey))
			Expect(user.Status.Claims.AccountName).Should(Equal(user.Spec.AccountName))
			Expect(user.Status.Claims.NatsLimits.Subs).Should(Equal(user.Spec.NatsLimits.Subs))
			Expect(user.Status.Claims.NatsLimits.Data).Should(Equal(user.Spec.NatsLimits.Data))
			Expect(user.Status.Claims.NatsLimits.Payload).Should(Equal(user.Spec.NatsLimits.Payload))
		})
	})
})

func GetNewUser() *v1alpha1.User {
	return &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      userName,
			Namespace: accountNamespace,
		},
		Spec: v1alpha1.UserSpec{
			AccountName: accountName,
			Permissions: &v1alpha1.Permissions{
				Pub: v1alpha1.Permission{},
				Sub: v1alpha1.Permission{},
			},
			UserLimits: &v1alpha1.UserLimits{},
			NatsLimits: &v1alpha1.NatsLimits{},
		},
	}
}

func GetNewUserWithNotReadyAccount() *v1alpha1.User {
	return &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      userName,
			Namespace: accountNamespace,
		},
		Spec: v1alpha1.UserSpec{
			AccountName: accountNameThatIsNotReady,
			Permissions: &v1alpha1.Permissions{
				Pub: v1alpha1.Permission{},
				Sub: v1alpha1.Permission{},
			},
			UserLimits: &v1alpha1.UserLimits{},
			NatsLimits: &v1alpha1.NatsLimits{},
		},
	}
}
