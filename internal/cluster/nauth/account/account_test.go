package account

import (
	"context"
	"fmt"
	"time"

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
			accountManager    *Manager
			accountGetterMock *AccountGetterMock
			natsClientMock    *NATSClientMock
			secretStorerMock  *SecretStorerMock
		)

		BeforeEach(func() {
			By("creating the account manager")
			accountGetterMock = NewAccountGetterMock()
			natsClientMock = NewNATSClientMock()
			secretStorerMock = NewSecretStorerMock()
			accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock, WithNamespace("nauth"))
		})

		AfterEach(func() {
			secretStorerMock.AssertExpectations(GinkgoT())
			natsClientMock.AssertExpectations(GinkgoT())
		})

		It("creates a new account with primary key", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking secret lookup by account name")
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{
				k8s.LabelAccountName: accountName,
				k8s.LabelManaged:     k8s.LabelManagedValue,
			}).Return(&corev1.SecretList{}, nil).Maybe()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountSign
			}), mock.Anything).Return(nil)

			By("creating a new account")
			result, err := accountManager.Create(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.AccountID).Should(Satisfy(isAccountPubKey))
			Expect(result.AccountSignedBy).ToNot(BeEmpty())
			Expect(result.Claims).ToNot(BeNil())
		})

		It("reuses existing secrets if NATS upload failed in a previous attempt", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer for operator keys")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil).Twice()

			By("mocking the NATS client to fail on first attempt and succeed on second")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil).Twice()
			natsClientMock.On("Disconnect").Return().Twice()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(fmt.Errorf("NATS error")).Once()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()

			var capturedAccountID string
			var capturedAccountSeed []byte
			var capturedAccountSignSeed []byte

			// Expect Apply to be called twice for each secret type (total 4 calls)
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetLabels()[k8s.LabelSecretType] == k8s.SecretTypeAccountRoot
			}), mock.Anything).Run(func(args mock.Arguments) {
				meta := args.Get(2).(v1.ObjectMeta)
				id := meta.GetLabels()[k8s.LabelAccountID]
				if capturedAccountID == "" {
					capturedAccountID = id
				}
				valMap := args.Get(3).(map[string]string)
				capturedAccountSeed = []byte(valMap[k8s.DefaultSecretKeyName])
			}).Return(nil).Twice()

			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetLabels()[k8s.LabelSecretType] == k8s.SecretTypeAccountSign
			}), mock.Anything).Run(func(args mock.Arguments) {
				valMap := args.Get(3).(map[string]string)
				capturedAccountSignSeed = []byte(valMap[k8s.DefaultSecretKeyName])
			}).Return(nil).Twice()

			// Mock GetByLabels for account lookup
			// First call returns empty. We use mock.MatchedBy to ensure it overrides the BeforeEach mock if needed.
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, mock.MatchedBy(func(l map[string]string) bool {
				return l[k8s.LabelAccountName] == accountName && l[k8s.LabelManaged] == k8s.LabelManagedValue
			})).Return(&corev1.SecretList{}, nil).Once()

			By("executing the first attempt")
			_, err := accountManager.Create(ctx, account)
			Expect(err).To(HaveOccurred())
			Expect(capturedAccountID).ToNot(BeEmpty())

			// Mock GetByLabels for second call to return the captured secrets
			By("mocking secret lookup for the second attempt")
			existingSecrets := &corev1.SecretList{
				Items: []corev1.Secret{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      "root",
							Namespace: accountNamespace,
							Labels: map[string]string{
								k8s.LabelAccountID:   capturedAccountID,
								k8s.LabelAccountName: accountName,
								k8s.LabelSecretType:  k8s.SecretTypeAccountRoot,
								k8s.LabelManaged:     k8s.LabelManagedValue,
							},
						},
						Data: map[string][]byte{k8s.DefaultSecretKeyName: capturedAccountSeed},
					},
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      "sign",
							Namespace: accountNamespace,
							Labels: map[string]string{
								k8s.LabelAccountID:   capturedAccountID,
								k8s.LabelAccountName: accountName,
								k8s.LabelSecretType:  k8s.SecretTypeAccountSign,
								k8s.LabelManaged:     k8s.LabelManagedValue,
							},
						},
						Data: map[string][]byte{k8s.DefaultSecretKeyName: capturedAccountSignSeed},
					},
				},
			}
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, mock.MatchedBy(func(l map[string]string) bool {
				return l[k8s.LabelAccountName] == accountName && l[k8s.LabelManaged] == k8s.LabelManagedValue
			})).Return(existingSecrets, nil).Once()

			By("executing the second attempt")
			result, err := accountManager.Create(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.AccountID).To(Equal(capturedAccountID))
		})

		It("fails to create an account with conflicting imports", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking secret lookup by account name")
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{
				k8s.LabelAccountName: accountName,
				k8s.LabelManaged:     k8s.LabelManagedValue,
			}).Return(&corev1.SecretList{}, nil).Maybe()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID := s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountSign
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
			_, err := accountManager.Create(ctx, account)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicting import subject found"))
		})

		It("converts jwt.AccountClaims to nauthv1alpha1.AccountClaims correctly", func() {
			// Build a fully populated jwt.AccountClaims
			kp, err := nkeys.CreateAccount()
			Expect(err).ToNot(HaveOccurred())
			pub, err := kp.PublicKey()
			Expect(err).ToNot(HaveOccurred())

			claims := jwt.NewAccountClaims(pub)
			// Fill limits
			claims.Limits.AccountLimits = jwt.AccountLimits{
				Imports:         3,
				Exports:         4,
				WildcardExports: false,
				Conn:            123,
				LeafNodeConn:    7,
			}
			claims.Limits.NatsLimits = jwt.NatsLimits{
				Subs:    111,
				Data:    222,
				Payload: 333,
			}
			claims.Limits.JetStreamLimits = jwt.JetStreamLimits{
				MemoryStorage:        10,
				DiskStorage:          20,
				Streams:              30,
				Consumer:             40,
				MaxAckPending:        50,
				MemoryMaxStreamBytes: 60,
				DiskMaxStreamBytes:   70,
				MaxBytesRequired:     true,
			}

			// Exports
			claims.Exports = jwt.Exports{
				&jwt.Export{ // stream export
					Name:        "stream-exp",
					Subject:     jwt.Subject("a.>"),
					Type:        jwt.Stream,
					TokenReq:    true,
					Revocations: jwt.RevocationList{"UABC": 12345},
					Advertise:   true,
				},
				&jwt.Export{ // service export
					Name:              "svc-exp",
					Subject:           jwt.Subject("req.svc"),
					Type:              jwt.Service,
					ResponseType:      jwt.ResponseTypeStream,
					ResponseThreshold: 250 * time.Millisecond,
					Latency: &jwt.ServiceLatency{
						Sampling: 42,
						Results:  jwt.Subject("results.latency"),
					},
					AccountTokenPosition: 2,
					AllowTrace:           true,
				},
			}

			// Imports
			claims.Imports = jwt.Imports{
				&jwt.Import{ // stream import
					Name:         "imp-stream",
					Subject:      jwt.Subject("b.>"),
					Account:      "ACCEXP1",
					LocalSubject: jwt.RenamingSubject("local.b.>"),
					Type:         jwt.Stream,
					Share:        false,
					AllowTrace:   false,
				},
				&jwt.Import{ // service import
					Name:         "imp-svc",
					Subject:      jwt.Subject("svc.api"),
					Account:      "ACCEXP2",
					LocalSubject: jwt.RenamingSubject("local.svc.api"),
					Type:         jwt.Service,
					Share:        true,
					AllowTrace:   false,
				},
			}

			// Convert
			out := convertNatsAccountClaims(claims)

			// Assert AccountLimits
			Expect(out.AccountLimits).ToNot(BeNil())
			Expect(*out.AccountLimits.Imports).To(Equal(int64(3)))
			Expect(*out.AccountLimits.Exports).To(Equal(int64(4)))
			Expect(*out.AccountLimits.WildcardExports).To(BeFalse())
			Expect(*out.AccountLimits.Conn).To(Equal(int64(123)))
			Expect(*out.AccountLimits.LeafNodeConn).To(Equal(int64(7)))

			// Assert NatsLimits
			Expect(out.NatsLimits).ToNot(BeNil())
			Expect(*out.NatsLimits.Subs).To(Equal(int64(111)))
			Expect(*out.NatsLimits.Data).To(Equal(int64(222)))
			Expect(*out.NatsLimits.Payload).To(Equal(int64(333)))

			// Assert JetStreamLimits
			Expect(out.JetStreamLimits).ToNot(BeNil())
			Expect(*out.JetStreamLimits.MemoryStorage).To(Equal(int64(10)))
			Expect(*out.JetStreamLimits.DiskStorage).To(Equal(int64(20)))
			Expect(*out.JetStreamLimits.Streams).To(Equal(int64(30)))
			Expect(*out.JetStreamLimits.Consumer).To(Equal(int64(40)))
			Expect(*out.JetStreamLimits.MaxAckPending).To(Equal(int64(50)))
			Expect(*out.JetStreamLimits.MemoryMaxStreamBytes).To(Equal(int64(60)))
			Expect(*out.JetStreamLimits.DiskMaxStreamBytes).To(Equal(int64(70)))
			Expect(out.JetStreamLimits.MaxBytesRequired).To(BeTrue())

			// Assert Exports
			Expect(out.Exports).To(HaveLen(2))
			var streamExp, svcExp *v1alpha1.Export
			for _, e := range out.Exports {
				switch e.Name {
				case "stream-exp":
					streamExp = e
				case "svc-exp":
					svcExp = e
				}
			}
			Expect(streamExp).ToNot(BeNil())
			Expect(string(streamExp.Subject)).To(Equal("a.>"))
			Expect(streamExp.Type).To(Equal(v1alpha1.Stream))
			Expect(streamExp.TokenReq).To(BeTrue())
			Expect(streamExp.Advertise).To(BeTrue())
			Expect(streamExp.Revocations).To(HaveKeyWithValue("UABC", int64(12345)))
			Expect(svcExp).ToNot(BeNil())
			Expect(svcExp.Type).To(Equal(v1alpha1.Service))
			Expect(svcExp.Name).To(Equal("svc-exp"))
			Expect(string(svcExp.Subject)).To(Equal("req.svc"))
			Expect(string(svcExp.ResponseType)).To(Equal(string(jwt.ResponseTypeStream)))
			Expect(svcExp.ResponseThreshold).To(Equal(250 * time.Millisecond))
			Expect(svcExp.Latency).ToNot(BeNil())
			Expect(int(svcExp.Latency.Sampling)).To(Equal(42))
			Expect(string(svcExp.Latency.Results)).To(Equal("results.latency"))
			Expect(svcExp.AccountTokenPosition).To(Equal(uint(2)))
			Expect(svcExp.AllowTrace).To(BeTrue())

			// Assert Imports
			Expect(out.Imports).To(HaveLen(2))
			var impStream, impSvc *v1alpha1.Import
			for _, im := range out.Imports {
				switch im.Name {
				case "imp-stream":
					impStream = im
				case "imp-svc":
					impSvc = im
				}
			}
			Expect(impStream).ToNot(BeNil())
			Expect(string(impStream.Subject)).To(Equal("b.>"))
			Expect(impStream.Account).To(Equal("ACCEXP1"))
			Expect(string(impStream.LocalSubject)).To(Equal("local.b.>"))
			Expect(impStream.Type).To(Equal(v1alpha1.Stream))
			Expect(impSvc).ToNot(BeNil())
			Expect(impSvc.Name).To(Equal("imp-svc"))
			Expect(string(impSvc.Subject)).To(Equal("svc.api"))
			Expect(impSvc.Account).To(Equal("ACCEXP2"))
			Expect(string(impSvc.LocalSubject)).To(Equal("local.svc.api"))
			Expect(impSvc.Type).To(Equal(v1alpha1.Service))
			Expect(impSvc.Share).To(BeTrue())
		})

		It("creates a new account and update it", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking secret lookup by account name")
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{
				k8s.LabelAccountName: accountName,
				k8s.LabelManaged:     k8s.LabelManagedValue,
			}).Return(&corev1.SecretList{}, nil).Maybe()

			var accountID string

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)
			sysAccKP, _ := nkeys.CreateAccount()
			sysAccPubKey, _ := sysAccKP.PublicKey()
			sysUserKP, _ := nkeys.CreateUser()
			sysUserPubKey, _ := sysUserKP.PublicKey()
			sysUserSeed, _ := sysUserKP.Seed()
			sysUserClaims := jwt.NewUserClaims(sysUserPubKey)
			sysUserClaims.IssuerAccount = sysAccPubKey
			sysUserJWT, _ := sysUserClaims.Encode(sysAccKP)
			sysUserCreds := fmt.Sprintf("-----BEGIN NATS USER JWT-----\n%s\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n%s\n------END USER NKEY SEED------\n", sysUserJWT, string(sysUserSeed))
			sysAccCredsLabelMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds}
			sysAccCredsSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: []byte(sysUserCreds)}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, sysAccCredsLabelMock).Return(sysAccCredsSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil)

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(s.GetLabels()[k8s.LabelAccountID]) && secretType == k8s.SecretTypeAccountSign
			}), mock.Anything).Return(nil)

			By("creating a new account")
			result, err := accountManager.Create(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.AccountID).Should(Satisfy(isAccountPubKey))

			// Apply result to account for subsequent operations
			if account.Labels == nil {
				account.Labels = make(map[string]string)
			}
			account.Labels[k8s.LabelAccountID] = result.AccountID

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
								k8s.LabelAccountID:  accountID,
								k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
							},
						},
						Data: map[string][]byte{
							k8s.DefaultSecretKeyName: accountSeed,
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{
								k8s.LabelAccountID:  accountID,
								k8s.LabelSecretType: k8s.SecretTypeAccountSign,
							},
						},
						Data: map[string][]byte{
							k8s.DefaultSecretKeyName: accountSigningSeed,
						},
					},
				},
			}
			accountSecretLabelsMock := map[string]string{
				k8s.LabelAccountID: accountID,
				k8s.LabelManaged:   k8s.LabelManagedValue,
			}
			secretStorerMock.On("GetByLabels", ctx, mock.Anything, accountSecretLabelsMock).Return(secretsList, nil)
			account.Spec.AccountLimits = &v1alpha1.AccountLimits{
				Imports:         ptr.To[int64](10),
				Exports:         ptr.To[int64](10),
				WildcardExports: ptr.To(true),
				Conn:            ptr.To[int64](100),
				LeafNodeConn:    ptr.To[int64](0),
			}
			updateResult, err := accountManager.Update(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResult).ToNot(BeNil())
			Expect(updateResult.AccountID).Should(Satisfy(isAccountPubKey))
		})

		It("updates an existing account with legacy secrets", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking the secret storer")
			operatorKeyPair, _ := nkeys.CreateOperator()
			operatorPublicKey, _ := operatorKeyPair.PublicKey()
			operatorSeed, _ := operatorKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)
			secretStorerMock.On("GetByLabels", mock.Anything, account.GetNamespace(), mock.Anything).Return(&corev1.SecretList{}, nil)
			sysAccKP, _ := nkeys.CreateAccount()
			sysAccPubKey, _ := sysAccKP.PublicKey()
			sysUserKP, _ := nkeys.CreateUser()
			sysUserPubKey, _ := sysUserKP.PublicKey()
			sysUserSeed, _ := sysUserKP.Seed()
			sysUserClaims := jwt.NewUserClaims(sysUserPubKey)
			sysUserClaims.IssuerAccount = sysAccPubKey
			sysUserJWT, _ := sysUserClaims.Encode(sysAccKP)
			sysUserCreds := fmt.Sprintf("-----BEGIN NATS USER JWT-----\n%s\n------END NATS USER JWT------\n\n-----BEGIN USER NKEY SEED-----\n%s\n------END USER NKEY SEED------\n", sysUserJWT, string(sysUserSeed))
			sysAccCredsLabelMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds}
			sysAccCredsSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: []byte(sysUserCreds)}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, sysAccCredsLabelMock).Return(sysAccCredsSecretMock, nil)

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
				k8s.LabelAccountID:       accountPublicKey,
				k8s.LabelAccountSignedBy: operatorPublicKey,
			}
			result, err := accountManager.Update(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.AccountID).Should(Satisfy(isAccountPubKey))
		})

		It("delete an account", func() {
			By("providing an account specification")
			account := GetNewAccount()

			By("mocking secret lookup by account name")
			secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{
				k8s.LabelAccountName: accountName,
				k8s.LabelManaged:     k8s.LabelManagedValue,
			}).Return(&corev1.SecretList{}, nil).Maybe()

			var accountID string

			By("mocking the secret storer")
			operatorSignKeyPair, _ := nkeys.CreateOperator()
			operatorSignSeed, _ := operatorSignKeyPair.Seed()
			operatorSignLabelsMock := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
			operatorSignSecretMock := &corev1.SecretList{
				Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: operatorSignSeed}}},
			}
			secretStorerMock.On("GetByLabels", ctx, nauthNamespace, operatorSignLabelsMock).Return(operatorSignSecretMock, nil)

			By("mocking the NATS client")
			natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
			natsClientMock.On("Disconnect").Return()
			natsClientMock.On("UploadAccountJWT", mock.Anything).Return(nil).Once()
			natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil).Once()

			By("validating that relevant keys for a base account are stored")
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountRoot
			}), mock.Anything).Return(nil)
			secretStorerMock.On("Apply", ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				accountID = s.GetLabels()[k8s.LabelAccountID]
				secretType := s.GetLabels()[k8s.LabelSecretType]
				return s.GetNamespace() == accountNamespace && isAccountPubKey(accountID) && secretType == k8s.SecretTypeAccountSign
			}), mock.Anything).Return(nil)
			secretStorerMock.On("DeleteByLabels", ctx, mock.Anything, mock.MatchedBy(func(s map[string]string) bool {
				return s[k8s.LabelAccountName] == accountName || s[k8s.LabelAccountID] == accountID
			})).Return(nil)

			By("creating a new account")
			result, err := accountManager.Create(ctx, account)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.AccountID).Should(Satisfy(isAccountPubKey))

			// Apply result to account for delete operation
			if account.Labels == nil {
				account.Labels = make(map[string]string)
			}
			account.Labels[k8s.LabelAccountID] = result.AccountID

			By("deleting the account")
			err = accountManager.Delete(ctx, account)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("getOperatorSigningKeyPairFromNatsCluster", func() {
			var (
				clusterNamespace = "cluster-ns"
				cluster          *v1alpha1.NatsCluster
			)

			BeforeEach(func() {
				cluster = &v1alpha1.NatsCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: clusterNamespace,
					},
					Spec: v1alpha1.NatsClusterSpec{
						OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
							Name: "operator-signing-key",
							Key:  "seed",
						},
						SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
							Name: "system-account-creds",
							Key:  "creds",
						},
					},
				}
			})

			It("successfully gets keypair using NatsCluster's namespace and custom key", func() {
				By("creating the account manager with NatsCluster")
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret with custom key 'seed'")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{"seed": string(operatorSeed)}, nil)

				By("mocking the NATS client and deletion")
				natsClientMock.On("EnsureConnected", clusterNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)
				secretStorerMock.On("DeleteByLabels", ctx, accountNamespace, map[string]string{
					k8s.LabelAccountID:   accountPublicKey,
					k8s.LabelAccountName: accountName,
				}).Return(nil)

				err := accountManager.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())
			})

			It("uses default key when secretRef.Key is empty", func() {
				By("creating NatsCluster with empty Key in secretRef")
				clusterWithDefaultKey := &v1alpha1.NatsCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: clusterNamespace,
					},
					Spec: v1alpha1.NatsClusterSpec{
						OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
							Name: "operator-signing-key",
							Key:  "", // Empty key should default to k8s.DefaultSecretKeyName
						},
						SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
							Name: "system-account-creds",
						},
					},
				}
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(clusterWithDefaultKey),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret with default key")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				// When Key is empty, it should use k8s.DefaultSecretKeyName ("default")
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{k8s.DefaultSecretKeyName: string(operatorSeed)}, nil)

				By("mocking the NATS client and deletion")
				natsClientMock.On("EnsureConnected", clusterNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)
				secretStorerMock.On("DeleteByLabels", ctx, accountNamespace, map[string]string{
					k8s.LabelAccountID:   accountPublicKey,
					k8s.LabelAccountName: accountName,
				}).Return(nil)

				err := accountManager.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())
			})

			It("falls back to account namespace when NatsCluster namespace is empty", func() {
				By("creating NatsCluster with empty namespace")
				clusterWithEmptyNS := &v1alpha1.NatsCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "", // Empty namespace
					},
					Spec: v1alpha1.NatsClusterSpec{
						OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
							Name: "operator-signing-key",
							Key:  "seed",
						},
						SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
							Name: "system-account-creds",
						},
					},
				}
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(clusterWithEmptyNS),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key - should use account namespace as fallback")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				secretStorerMock.On("Get", ctx, accountNamespace, "operator-signing-key").
					Return(map[string]string{"seed": string(operatorSeed)}, nil)

				By("mocking the NATS client - should use account namespace as fallback")
				natsClientMock.On("EnsureConnected", accountNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)
				secretStorerMock.On("DeleteByLabels", ctx, accountNamespace, map[string]string{
					k8s.LabelAccountID:   accountPublicKey,
					k8s.LabelAccountName: accountName,
				}).Return(nil)

				err := accountManager.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())
			})

			It("fails when secret lookup returns an error", func() {
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the secret lookup to fail")
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{}, fmt.Errorf("secret not found"))

				err := accountManager.Delete(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("operator signing key secret"))
			})

			It("fails when secret does not contain the expected key", func() {
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the secret to return data without the expected key")
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{"wrong-key": "some-value"}, nil)

				err := accountManager.Delete(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not contain key"))
				Expect(err.Error()).To(ContainSubstring("seed"))
			})

			It("fails when seed is invalid and cannot create keypair", func() {
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the secret to return an invalid seed")
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{"seed": "invalid-seed-data"}, nil)

				err := accountManager.Delete(ctx, account)
				Expect(err).To(HaveOccurred())
				// nkeys.FromSeed returns an error for invalid seeds
			})
		})

		Context("Delete with NatsCluster-based config", func() {
			var (
				clusterNamespace = "cluster-ns"
				cluster          *v1alpha1.NatsCluster
			)

			BeforeEach(func() {
				cluster = &v1alpha1.NatsCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: clusterNamespace,
					},
					Spec: v1alpha1.NatsClusterSpec{
						OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
							Name: "operator-signing-key",
							Key:  "seed",
						},
						SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
							Name: "system-account-creds",
							Key:  "creds",
						},
					},
				}
			})

			It("successfully deletes an account using NatsCluster secretRef", func() {
				By("creating the account manager with NatsCluster")
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret from NatsCluster's secretRef")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{"seed": string(operatorSeed)}, nil)

				By("mocking the NATS client")
				natsClientMock.On("EnsureConnected", clusterNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)

				By("mocking secret cleanup")
				secretStorerMock.On("DeleteByLabels", ctx, accountNamespace, map[string]string{
					k8s.LabelAccountID:   accountPublicKey,
					k8s.LabelAccountName: accountName,
				}).Return(nil)

				By("deleting the account")
				err := accountManager.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())
			})

			It("fails when operator signing key secret lookup fails", func() {
				By("creating the account manager with NatsCluster")
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret to fail")
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{}, fmt.Errorf("secret not found"))

				By("deleting the account should fail")
				err := accountManager.Delete(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("operator signing key"))
			})

			It("fails when NATS DeleteAccountJWT fails", func() {
				By("creating the account manager with NatsCluster")
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(cluster),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret from NatsCluster's secretRef")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				secretStorerMock.On("Get", ctx, clusterNamespace, "operator-signing-key").
					Return(map[string]string{"seed": string(operatorSeed)}, nil)

				By("mocking the NATS client to fail on delete")
				natsClientMock.On("EnsureConnected", clusterNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(fmt.Errorf("NATS delete failed"))

				By("deleting the account should fail")
				err := accountManager.Delete(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("delete account"))
			})

			It("falls back to account namespace when NatsCluster namespace is empty", func() {
				By("creating the account manager with NatsCluster that has empty namespace")
				clusterWithEmptyNS := &v1alpha1.NatsCluster{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "", // Empty namespace
					},
					Spec: v1alpha1.NatsClusterSpec{
						OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
							Name: "operator-signing-key",
							Key:  "seed",
						},
						SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
							Name: "system-account-creds",
							Key:  "creds",
						},
					},
				}
				accountManager = NewManager(accountGetterMock, natsClientMock, secretStorerMock,
					WithNamespace(nauthNamespace),
					WithNatsCluster(clusterWithEmptyNS),
				)

				account := GetNewAccount()
				accountKeyPair, _ := nkeys.CreateAccount()
				accountPublicKey, _ := accountKeyPair.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accountPublicKey}

				By("mocking the operator signing key secret - should use account namespace as fallback")
				operatorKeyPair, _ := nkeys.CreateOperator()
				operatorSeed, _ := operatorKeyPair.Seed()
				// Should fall back to account's namespace when NatsCluster namespace is empty
				secretStorerMock.On("Get", ctx, accountNamespace, "operator-signing-key").
					Return(map[string]string{"seed": string(operatorSeed)}, nil)

				By("mocking the NATS client - should use account namespace as fallback")
				natsClientMock.On("EnsureConnected", accountNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()
				natsClientMock.On("DeleteAccountJWT", mock.Anything).Return(nil)

				By("mocking secret cleanup")
				secretStorerMock.On("DeleteByLabels", ctx, accountNamespace, map[string]string{
					k8s.LabelAccountID:   accountPublicKey,
					k8s.LabelAccountName: accountName,
				}).Return(nil)

				By("deleting the account")
				err := accountManager.Delete(ctx, account)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Import", func() {
			createAccountClaims := func(subject string) string {
				claims := jwt.NewAccountClaims(subject)
				claims.Limits.AccountLimits = jwt.AccountLimits{
					Conn: 77,
				}
				claims.Limits.NatsLimits = jwt.NatsLimits{Subs: 999}
				claims.Limits.JetStreamLimits = jwt.JetStreamLimits{Streams: 9}
				claims.Exports = jwt.Exports{
					&jwt.Export{
						Name:         "ex-svc",
						Subject:      jwt.Subject("svc.req"),
						Type:         jwt.Service,
						ResponseType: jwt.ResponseTypeChunked,
						AllowTrace:   true,
					},
				}
				op, _ := nkeys.CreateOperator()
				jwtStr, _ := claims.Encode(op)
				return jwtStr
			}

			It("successfully imports an account from NATS", func() {
				account := GetNewAccount()

				By("preparing keys")
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()
				opPub, _ := opKP.PublicKey()

				accRootKP, _ := nkeys.CreateAccount()
				accRootSeed, _ := accRootKP.Seed()
				accRootPub, _ := accRootKP.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accRootPub}

				accSignKP, _ := nkeys.CreateAccount()
				accSignSeed, _ := accSignKP.Seed()

				By("mocking the secret storer")
				opSignLabels := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
				opSignSecrets := &corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}
				secretStorerMock.On("GetByLabels", ctx, nauthNamespace, opSignLabels).Return(opSignSecrets, nil)
				accSecrets := &corev1.SecretList{Items: []corev1.Secret{
					{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountRoot, k8s.LabelAccountID: accRootPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accRootSeed}},
					{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountSign, k8s.LabelAccountID: accRootPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accSignSeed}},
				}}
				secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{k8s.LabelAccountID: accRootPub, k8s.LabelManaged: k8s.LabelManagedValue}).Return(accSecrets, nil)

				By("mocking the NATS client")
				accountJWT := createAccountClaims(accRootPub)
				natsClientMock.On("LookupAccountJWT", accRootPub).Return(accountJWT, nil)
				natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()

				By("importing the account from NATS")
				result, err := accountManager.Import(ctx, account)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.AccountSignedBy).To(Equal(opPub))
				Expect(result.AccountID).To(Equal(accRootPub))

				claims := result.Claims
				Expect(claims).ToNot(BeNil())
				Expect(claims.AccountLimits).ToNot(BeNil())
				Expect(claims.AccountLimits.Conn).ToNot(BeNil())
				Expect(*claims.AccountLimits.Conn).To(Equal(int64(77)))
				Expect(claims.NatsLimits).ToNot(BeNil())
				Expect(claims.NatsLimits.Subs).ToNot(BeNil())
				Expect(*claims.NatsLimits.Subs).To(Equal(int64(999)))
				Expect(claims.JetStreamLimits).ToNot(BeNil())
				Expect(claims.JetStreamLimits.Streams).ToNot(BeNil())
				Expect(*claims.JetStreamLimits.Streams).To(Equal(int64(9)))
				Expect(claims.Exports).To(HaveLen(1))
				ex := claims.Exports[0]
				Expect(ex.Name).To(Equal("ex-svc"))
				Expect(string(ex.Subject)).To(Equal("svc.req"))
				Expect(ex.Type).To(Equal(v1alpha1.Service))
				Expect(string(ex.ResponseType)).To(Equal(string(jwt.ResponseTypeChunked)))
				Expect(ex.AllowTrace).To(BeTrue())
			})

			It("fails when account ID is missing", func() {
				account := GetNewAccount()
				// operator signing present so we fail on account ID, not before
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()
				opSignLabels := map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}
				opSignSecrets := &corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}
				secretStorerMock.On("GetByLabels", ctx, nauthNamespace, opSignLabels).Return(opSignSecrets, nil)

				_, err := accountManager.Import(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("account ID"))
			})

			It("fails when account root secret is not found", func() {
				account := GetNewAccount()
				accKP, _ := nkeys.CreateAccount()
				accPub, _ := accKP.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accPub}
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()
				secretStorerMock.On("GetByLabels", ctx, nauthNamespace, map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}).Return(&corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}, nil)
				// return two secrets but none of them root
				accSignKP, _ := nkeys.CreateAccount()
				accSignSeed, _ := accSignKP.Seed()
				accSecrets := &corev1.SecretList{Items: []corev1.Secret{
					{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: "other", k8s.LabelAccountID: accPub}}},
					{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountSign, k8s.LabelAccountID: accPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accSignSeed}},
				}}
				secretStorerMock.On("GetByLabels", ctx, accountNamespace, map[string]string{k8s.LabelAccountID: accPub, k8s.LabelManaged: k8s.LabelManagedValue}).Return(accSecrets, nil)

				_, err := accountManager.Import(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("existing account root secret not found"))
			})

			It("fails when account signing secret is not found", func() {
				account := GetNewAccount()

				By("preparing keys")
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()
				accKP, _ := nkeys.CreateAccount()
				accRootSeed, _ := accKP.Seed()
				accRootPub, _ := accKP.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accRootPub}

				By("mocking the secret storer")
				secretStorerMock.
					On("GetByLabels", ctx, nauthNamespace, map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}).
					Return(&corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}, nil)
				secretStorerMock.
					On("GetByLabels", ctx, accountNamespace, map[string]string{k8s.LabelAccountID: accRootPub, k8s.LabelManaged: k8s.LabelManagedValue}).
					Return(&corev1.SecretList{Items: []corev1.Secret{
						// return two secrets but none of them sign
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: "other", k8s.LabelAccountID: accRootPub}}},
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountRoot, k8s.LabelAccountID: accRootPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accRootSeed}},
					}}, nil)

				_, err := accountManager.Import(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("existing account signing secret not found"))
			})

			It("fails when account root secret does not match account ID", func() {
				account := GetNewAccount()

				By("preparing keys")
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()

				accKP, _ := nkeys.CreateAccount()
				accPub, _ := accKP.PublicKey()
				accSignSeed, _ := accKP.Seed()
				account.Labels = map[string]string{k8s.LabelAccountID: accPub}

				otherAccKP, _ := nkeys.CreateAccount()
				otherAccRootSeed, _ := otherAccKP.Seed()

				By("mocking the secret storer")
				secretStorerMock.
					On("GetByLabels", ctx, nauthNamespace, map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}).
					Return(&corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}, nil)
				secretStorerMock.
					On("GetByLabels", ctx, accountNamespace, map[string]string{k8s.LabelAccountID: accPub, k8s.LabelManaged: k8s.LabelManagedValue}).
					Return(&corev1.SecretList{Items: []corev1.Secret{
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountRoot, k8s.LabelAccountID: "WRONG"}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: otherAccRootSeed}},
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountSign, k8s.LabelAccountID: "WRONG"}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accSignSeed}},
					}}, nil)

				_, err := accountManager.Import(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("match"))
			})

			It("fails when NATS has no user with such ID", func() {
				account := GetNewAccount()

				By("preparing keys")
				opKP, _ := nkeys.CreateOperator()
				opSeed, _ := opKP.Seed()

				accKP, _ := nkeys.CreateAccount()
				accRootSeed, _ := accKP.Seed()
				accRootPub, _ := accKP.PublicKey()
				account.Labels = map[string]string{k8s.LabelAccountID: accRootPub}

				accSignKP, _ := nkeys.CreateAccount()
				accSignSeed, _ := accSignKP.Seed()

				By("mocking the secret storer")
				secretStorerMock.
					On("GetByLabels", ctx, nauthNamespace, map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign}).
					Return(&corev1.SecretList{Items: []corev1.Secret{{Data: map[string][]byte{k8s.DefaultSecretKeyName: opSeed}}}}, nil)
				secretStorerMock.
					On("GetByLabels", ctx, accountNamespace, map[string]string{k8s.LabelAccountID: accRootPub, k8s.LabelManaged: k8s.LabelManagedValue}).
					Return(&corev1.SecretList{Items: []corev1.Secret{
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountRoot, k8s.LabelAccountID: accRootPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accRootSeed}},
						{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{k8s.LabelSecretType: k8s.SecretTypeAccountSign, k8s.LabelAccountID: accRootPub}}, Data: map[string][]byte{k8s.DefaultSecretKeyName: accSignSeed}},
					}}, nil)

				By("mocking the NATS client")
				natsClientMock.On("LookupAccountJWT", accRootPub).Return("", nil)
				natsClientMock.On("EnsureConnected", nauthNamespace).Return(nil)
				natsClientMock.On("Disconnect").Return()

				_, err := accountManager.Import(ctx, account)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("jwt"))
			})
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
