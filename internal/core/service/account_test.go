package service

import (
	"context"
	"fmt"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	accountName      = "test-account"
	accountNamespace = "default"
	nauthNamespace   = "nauth"
	unlimitedLimit   = -1
)

var _ = Describe("Account manager", func() {
	Context("When handling NATS account resources", func() {
		var (
			ctx               = context.Background()
			accountManager    *AccountManager
			accountGetterMock *AccountGetterMock
			natsClientMock    *NATSClientMock
			secretStorerMock  *SecretStorerMock
		)

		BeforeEach(func() {
			By("creating the account manager")
			accountGetterMock = NewAccountGetterMock()
			natsClientMock = NewNATSClientMock()
			secretStorerMock = NewSecretStorerMock()
			accountManager = NewAccountManager(accountGetterMock, natsClientMock, secretStorerMock, WithNamespace("nauth"))
		})

		AfterEach(func() {
			secretStorerMock.AssertExpectations(GinkgoT())
			natsClientMock.AssertExpectations(GinkgoT())
		})

		It("creates a new account with primary key", func() {
			By("providing an account specification")

			account := GetNewAccount()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{domain.LabelSecretType: domain.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{domain.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountSign
			}), mock.Anything).Return(nil)

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountID]).Should(Satisfy(isAccountPubKey))
		})

		It("fails to create an account with conflicting imports", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{domain.LabelSecretType: domain.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{domain.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountSign
			}), mock.Anything).Return(nil)

			By("providing an account with conflicting imports")

			accountGetterMock.On("Get", ctx, "Account1", "default").Return(*GetExistingAccount(), nil)
			accountGetterMock.On("Get", ctx, "Account2", "default").Return(*GetExistingAccount(), nil)

			account.Spec.Imports = v1alpha1.Imports{
				{
					Name:       "Import1",
					Subject:    "subject.duplicate",
					AccountRef: v1alpha1.AccountRef{Name: "Account1", Namespace: "default"},
				},
				{
					Name:       "Import2",
					Subject:    "subject.duplicate",
					AccountRef: v1alpha1.AccountRef{Name: "Account2", Namespace: "default"},
				},
			}

			By("ensuring conflict detection during account processing")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicting import subject found"))
		})

		It("creates a new account and update it", func() {
			By("providing an account specification")
			account := GetNewAccount()
			var accountID string

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{domain.LabelSecretType: domain.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{domain.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(s.GetLabels()[domain.LabelAccountID]) && secretType == domain.SecretTypeAccountSign
			}), mock.Anything).Return(nil)

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountID]).Should(Satisfy(isAccountPubKey))

			By("updating account")
			accountKeyPair, _ := nkeys.CreateAccount()
			accountSeed, _ := accountKeyPair.Seed()

			accountSigningKeyPair, _ := nkeys.CreateAccount()
			accountSigningSeed, _ := accountSigningKeyPair.Seed()
			secretsList := &corev1.SecretList{
				Items: []corev1.Secret{
					{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								domain.LabelSecretType: domain.SecretTypeAccountRoot,
							},
						},
						Data: map[string][]byte{
							domain.DefaultSecretKeyName: []byte(accountSeed),
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								domain.LabelSecretType: domain.SecretTypeAccountSign,
							},
						},
						Data: map[string][]byte{
							domain.DefaultSecretKeyName: []byte(accountSigningSeed),
						},
					},
				},
			}
			accountSecretLabelsMock := map[string]string{
				domain.LabelAccountID: accountID,
				domain.LabelManaged:   domain.LabelManagedValue,
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, mock.Anything, accountSecretLabelsMock).Return(secretsList, nil)
			account.Spec.AccountLimits = &v1alpha1.AccountLimits{
				Imports:         ptr.To[int64](10),
				Exports:         ptr.To[int64](10),
				WildcardExports: ptr.To(true),
				Conn:            ptr.To[int64](100),
				LeafNodeConn:    ptr.To[int64](0),
			}
			err = accountManager.UpdateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountID]).Should(Satisfy(isAccountPubKey))
		})

		It("updates an existing account with legacy secrets", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorPublicKey, _ := operatorKeyPair.PublicKey()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{domain.LabelSecretType: domain.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{domain.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)
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

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("updating account")
			account.Spec.AccountLimits = &v1alpha1.AccountLimits{
				Imports:         ptr.To[int64](10),
				Exports:         ptr.To[int64](10),
				WildcardExports: ptr.To(true),
				Conn:            ptr.To[int64](100),
				LeafNodeConn:    ptr.To[int64](0),
			}
			account.Labels = map[string]string{
				domain.LabelAccountID:       accountPublicKey,
				domain.LabelAccountSignedBy: operatorPublicKey,
			}
			err := accountManager.UpdateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountID]).Should(Satisfy(isAccountPubKey))
		})

		It("delete an account", func() {
			By("providing an account specification")
			account := GetNewAccount()
			var accountID string

			By("mocking the secret storer")
			operatorSignKeyPair, _ := nkeys.CreateOperator()
			operatorSignSeed, _ := operatorSignKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{domain.LabelSecretType: domain.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{domain.DefaultSecretKeyName: operatorSignSeed}}},
			}
			secretStorerMock.On("GetSecretsByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()
			natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil).Once()

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[domain.LabelAccountID]
				secretType := s.GetLabels()[domain.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == domain.SecretTypeAccountSign
			}), mock.Anything).Return(nil)
			secretStorerMock.On("DeleteSecretsByLabels", ctx, mock.Anything, mock.MatchedBy(func(s map[string]string) bool {
				return s[domain.LabelAccountID] == accountID
			}), mock.Anything).Return(nil)

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountID]).Should(Satisfy(isAccountPubKey))

			By("deleting the account")
			err = accountManager.DeleteAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

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
	account := GetNewAccount()
	account.Status = v1alpha1.AccountStatus{
		SigningKey: v1alpha1.KeyInfo{
			Name: "OPERATORSIGNPUBKEY",
		},
		Conditions: []v1.Condition{
			{
				Type:   types.ControllerTypeReady,
				Status: v1.ConditionTrue,
			},
		},
	}
	return account
}

func GetNotReadyAccount() *v1alpha1.Account {
	account := GetNewAccount()
	account.Status = v1alpha1.AccountStatus{
		SigningKey: v1alpha1.KeyInfo{
			Name: "OPERATORSIGNPUBKEY",
		},
		Conditions: []v1.Condition{
			{
				Type:               types.ControllerTypeReady,
				Status:             v1.ConditionFalse,
				Reason:             "AccountNotReady",
				LastTransitionTime: v1.Now(),
			},
		},
	}
	return account
}
