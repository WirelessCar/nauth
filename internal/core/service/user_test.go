package service

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	userName = "user"
)

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
			accountKeyPair, _ := nkeys.CreateAccount()
			accountSeed, _ := accountKeyPair.Seed()
			secretStorerMock.secrets[account.GetAccountSignSecretName()] = map[string]string{domain.DefaultSecretKeyName: string(accountSeed)}
			secretStorerMock.On("GetSecret", ctx, accountNamespace, account.GetAccountSignSecretName()).Return(nil)

			By("User credentials are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, accountNamespace, user.GetUserSecretName(), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserId]).Should(Satisfy(isUserPubKey))
		})
		It("creates a new user and update settigs", func() {
			account := GetExistingAccount()
			user := GetNewUser()

			By("providing a user specification without any specific configuration")
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil).Twice()

			By("mocking preexisting account keys & CR")
			accountKeyPair, _ := nkeys.CreateAccount()
			accountSeed, _ := accountKeyPair.Seed()
			secretStorerMock.secrets[account.GetAccountSignSecretName()] = map[string]string{domain.DefaultSecretKeyName: string(accountSeed)}
			secretStorerMock.On("GetSecret", ctx, accountNamespace, account.GetAccountSignSecretName()).Return(nil)

			By("User credentials are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, accountNamespace, user.GetUserSecretName(), mock.AnythingOfType("map[string]string")).Return(nil).Twice()

			err := userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.Status.Claims.AccountName).Should(Equal(user.Spec.AccountName))
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserId]).Should(Satisfy(isUserPubKey))

			user.Spec.NatsLimits = &v1alpha1.NatsLimits{
				Subs:    ptr.To[int64](100),
				Data:    ptr.To[int64](1024),
				Payload: ptr.To[int64](256),
			}

			err = userManager.CreateOrUpdateUser(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[domain.LabelUserId]).Should(Satisfy(isUserPubKey))
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
