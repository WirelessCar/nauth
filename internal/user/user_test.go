package user

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
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
	accountName      = "test-account"
	accountNamespace = "default"
	unlimitedLimit   = -1
)

var _ = Describe("User manager", func() {
	Context("When handling NATS user resources", func() {
		var (
			ctx               = context.Background()
			userManager       *Manager
			accountGetterMock *AccountGetterMock
			secretStorerMock  *SecretStorerMock
			natsClientMock    *NATSClientMock
		)

		BeforeEach(func() {
			By("creating the user manager")
			secretStorerMock = NewSecretStorerMock()
			accountGetterMock = NewAccountGetterMock()
			natsClientMock = NewNATSClientMock()
			userManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock, WithNamespace(accountNamespace))
		})

		AfterEach(func() {
			secretStorerMock.AssertExpectations(GinkgoT())
			accountGetterMock.AssertExpectations(GinkgoT())
			natsClientMock.AssertExpectations(GinkgoT())
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
							k8s.DefaultSecretKeyName: accountSigningSeed,
						},
					},
				},
			}
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, mock.Anything).Return(secretsList, nil)

			By("User credentials are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
		})

		It("creates a new user from an account with legacy secrets", func() {
			By("providing a user specification")
			user := GetNewUser()

			account := GetExistingAccount()

			By("mocking the secret storer")
			secretStorerMock.On("GetByLabels", mock.Anything, account.GetNamespace(), mock.Anything).Return(&corev1.SecretList{}, nil)

			accountKeyPair, _ := nkeys.CreateAccount()
			accountPublicKey, _ := accountKeyPair.PublicKey()
			accountSeed, _ := accountKeyPair.Seed()
			accountSecretValueMock := map[string]string{k8s.DefaultSecretKeyName: string(accountSeed)}
			accountSecretNameMock := fmt.Sprintf(k8s.DeprecatedSecretNameAccountRootTemplate, account.GetName())
			secretStorerMock.On("Get", mock.Anything, account.GetNamespace(), accountSecretNameMock).Return(accountSecretValueMock, nil)
			accountSecretLabelsMock := map[string]string{
				k8s.LabelAccountID:  accountPublicKey,
				k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			secretStorerMock.On("Label", mock.Anything, account.GetNamespace(), accountSecretNameMock, accountSecretLabelsMock).Return(nil)

			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			accountSigningSecretValueMock := map[string]string{k8s.DefaultSecretKeyName: string(accountSigningSeed)}
			accountSigningSecretNameMock := fmt.Sprintf(k8s.DeprecatedSecretNameAccountSignTemplate, account.GetName())
			secretStorerMock.On("Get", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock).Return(accountSigningSecretValueMock, nil)
			accountSigningSecretLabelsMock := map[string]string{
				k8s.LabelAccountID:  accountPublicKey,
				k8s.LabelSecretType: k8s.SecretTypeAccountSign,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			secretStorerMock.On("Label", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock, accountSigningSecretLabelsMock).Return(nil)

			By("mocking existing account")
			account.Status.SigningKey = v1alpha1.KeyInfo{
				Name: accountSigningPublicKey,
			}
			account.Labels = map[string]string{
				k8s.LabelAccountID: accountPublicKey,
			}
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil)

			By("mock storing user credentials")
			secretStorerMock.On("Apply", mock.Anything, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
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
							k8s.DefaultSecretKeyName: accountSigningSeed,
						},
					},
				},
			}
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, mock.Anything).Return(secretsList, nil)

			By("User credentials are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))

			user.Spec.NatsLimits = &v1alpha1.NatsLimits{
				Subs:    ptr.To[int64](100),
				Data:    ptr.To[int64](1024),
				Payload: ptr.To[int64](256),
			}

			err = userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
			Expect(user.Status.Claims.NatsLimits.Subs).Should(Equal(user.Spec.NatsLimits.Subs))
			Expect(user.Status.Claims.NatsLimits.Data).Should(Equal(user.Spec.NatsLimits.Data))
			Expect(user.Status.Claims.NatsLimits.Payload).Should(Equal(user.Spec.NatsLimits.Payload))
		})

		It("creates a new scopped user from an account with legacy secrets, then delete it", func() {
			By("providing a user specification")
			user := GetNewScoppedUser()

			account := GetExistingAccount()

			By("mocking the secret storer")

			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, account.GetNamespace(), operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			secretStorerMock.On("GetByLabels", mock.Anything, account.GetNamespace(), mock.Anything).Return(&corev1.SecretList{}, nil)

			accountKeyPair, _ := nkeys.CreateAccount()
			accountPublicKey, _ := accountKeyPair.PublicKey()
			accountSeed, _ := accountKeyPair.Seed()
			accountSecretValueMock := map[string]string{k8s.DefaultSecretKeyName: string(accountSeed)}
			accountSecretNameMock := fmt.Sprintf(k8s.DeprecatedSecretNameAccountRootTemplate, account.GetName())
			secretStorerMock.On("Get", mock.Anything, account.GetNamespace(), accountSecretNameMock).Return(accountSecretValueMock, nil)
			accountSecretLabelsMock := map[string]string{
				k8s.LabelAccountID:  accountPublicKey,
				k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			secretStorerMock.On("Label", mock.Anything, account.GetNamespace(), accountSecretNameMock, accountSecretLabelsMock).Return(nil)

			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			accountSigningSecretValueMock := map[string]string{k8s.DefaultSecretKeyName: string(accountSigningSeed)}
			accountSigningSecretNameMock := fmt.Sprintf(k8s.DeprecatedSecretNameAccountSignTemplate, account.GetName())
			secretStorerMock.On("Get", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock).Return(accountSigningSecretValueMock, nil)
			accountSigningSecretLabelsMock := map[string]string{
				k8s.LabelAccountID:  accountPublicKey,
				k8s.LabelSecretType: k8s.SecretTypeAccountSign,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			secretStorerMock.On("Label", mock.Anything, account.GetNamespace(), accountSigningSecretNameMock, accountSigningSecretLabelsMock).Return(nil)
			claims := jwt.NewAccountClaims(accountPublicKey)
			token, _ := claims.Encode(accountSigningKeyPair)

			By("mocking existing account")
			account.Status.SigningKey = v1alpha1.KeyInfo{
				Name: accountSigningPublicKey,
			}
			account.Labels = map[string]string{
				k8s.LabelAccountID: accountPublicKey,
			}
			accountGetterMock.On("Get", ctx, accountName, accountNamespace).Return(*account, nil)

			By("mock storing account WT")
			secretStorerMock.On("Apply", mock.Anything, mock.Anything, mock.Anything, mock.AnythingOfType("map[string]string")).Return(nil)

			By("mock getting the account JWT")
			natsClientMock.On("LookupAccountJWT", mock.AnythingOfType("string")).Return(token, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", accountNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			// By("mock storing user credentials")
			// secretStorerMock.On("Apply", mock.Anything, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
			// 	return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			// }), mock.AnythingOfType("map[string]string")).Return(nil)

			err := userManager.CreateOrUpdate(ctx, user)

			By("mock deleting secrets")
			secretStorerMock.On("Delete", ctx, accountNamespace, mock.AnythingOfType("string")).Return(nil)
			errDel := userManager.Delete(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(errDel).ToNot(HaveOccurred())

			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
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

func GetNewScoppedUser() *v1alpha1.User {
	return &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      userName,
			Namespace: accountNamespace,
		},
		Spec: v1alpha1.UserSpec{
			UseSigningKey: true,
			AccountName:   accountName,
			Permissions:   nil,
			UserLimits:    nil,
			NatsLimits:    nil,
		},
	}
}

func GetNewAccount() *v1alpha1.Account {
	return &v1alpha1.Account{
		ObjectMeta: v1.ObjectMeta{
			Name:      accountName,
			Namespace: accountNamespace,
		},
		Spec: v1alpha1.AccountSpec{
			JetStreamLimits: &v1alpha1.JetStreamLimits{
				MemoryStorage: ptr.To[int64](unlimitedLimit),
				DiskStorage:   ptr.To[int64](unlimitedLimit),
				Consumer:      ptr.To[int64](unlimitedLimit),
			},
		},
	}
}

func GetExistingAccount() *v1alpha1.Account {
	const ControllerTypeReady = "Ready"
	account := GetNewAccount()
	account.Status = v1alpha1.AccountStatus{
		SigningKey: v1alpha1.KeyInfo{
			Name: "OPERATORSIGNPUBKEY",
		},
		Conditions: []v1.Condition{
			{
				Type:   ControllerTypeReady,
				Status: v1.ConditionTrue,
			},
		},
	}
	return account
}
