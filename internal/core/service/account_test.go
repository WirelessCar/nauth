package service

import (
	"context"

	"github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
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
			secretStorerMock.secrets["operator-op-sign"] = map[string]string{domain.DefaultSecretKeyName: string(operatorSeed)}
			secretStorerMock.On("GetSecret", ctx, nauthNamespace, "operator-op-sign").Return(nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace, "operator-sau-creds").Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, accountNamespace, mock.Anything, mock.Anything).Return(nil).Twice()

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountId]).Should(Satisfy(isAccountPubKey))
		})

		It("creates a new account and update it", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			secretStorerMock.secrets["operator-op-sign"] = map[string]string{domain.DefaultSecretKeyName: string(operatorSeed)}
			secretStorerMock.On("GetSecret", ctx, nauthNamespace, "operator-op-sign").Return(nil)
			secretStorerMock.On("GetSecret", ctx, accountNamespace, mock.Anything).Return(nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace, "operator-sau-creds").Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, accountNamespace, mock.Anything, mock.Anything).Return(nil).Twice()

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountId]).Should(Satisfy(isAccountPubKey))

			By("updating account")
			account.Spec.AccountLimits = &v1alpha1.AccountLimits{
				Imports:         ptr.To[int64](10),
				Exports:         ptr.To[int64](10),
				WildcardExports: ptr.To[bool](true),
				Conn:            ptr.To[int64](100),
				LeafNodeConn:    ptr.To[int64](0),
			}
			err = accountManager.UpdateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountId]).Should(Satisfy(isAccountPubKey))
		})

		It("delete an account", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer")
			operatorSignKeyPair, _ := nkeys.CreateOperator()
			operatorSignSeed, _ := operatorSignKeyPair.Seed()
			secretStorerMock.secrets["operator-op-sign"] = map[string]string{domain.DefaultSecretKeyName: string(operatorSignSeed)}
			secretStorerMock.On("GetSecret", ctx, nauthNamespace, "operator-op-sign").Return(nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace, "operator-sau-creds").Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()
			natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil).Once()

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("ApplySecret", ctx, mock.Anything, accountNamespace, mock.Anything, mock.Anything).Return(nil).Twice()

			By("creating a new account")
			err := accountManager.CreateAccount(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(account.GetLabels()).ToNot(BeNil())
			Expect(account.GetLabels()[domain.LabelAccountId]).Should(Satisfy(isAccountPubKey))

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
