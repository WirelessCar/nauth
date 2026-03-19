package user

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var userName = "user"

const (
	accountName      = "test-account"
	accountNamespace = "default"
)

var _ = Describe("User manager", func() {
	Context("When handling NATS user resources", func() {
		var (
			ctx               = context.Background()
			userManager       *UserManager
			userJWTSignerMock *UserJWTSignerMock
			secretClientMock  *SecretClientMock
		)

		BeforeEach(func() {
			By("creating the user manager")
			secretClientMock = NewSecretClientMock()
			userJWTSignerMock = NewUserJWTSignerMock()
			userManager = NewUserManager(userJWTSignerMock, secretClientMock)
		})

		AfterEach(func() {
			secretClientMock.AssertExpectations(GinkgoT())
			userJWTSignerMock.AssertExpectations(GinkgoT())
		})

		It("creates a new user belonging to the correct account", func() {
			By("providing a fake existing account and signing key")
			accountRoot, _ := nkeys.CreateAccount()
			accountID, _ := accountRoot.PublicKey()
			accountSign, _ := nkeys.CreateAccount()
			accountSignPub, _ := accountSign.PublicKey()

			By("providing a user specification")
			user := GetNewUser()
			var subsLimit int64 = 43
			user.Spec.NatsLimits.Subs = &subsLimit

			By("mocking user signing")
			userJWTSignerMock.mockSignUserJWT(ctx, domain.NewNamespacedName(accountNamespace, accountName), func(claims *jwt.UserClaims) *SignedUserJWT {
				Expect(claims.IssuerAccount).To(BeEmpty())
				claims.IssuerAccount = accountID
				userJWT, err := claims.Encode(accountSign)
				Expect(err).NotTo(HaveOccurred())
				return &SignedUserJWT{
					UserJWT:   userJWT,
					AccountID: accountID,
					SignedBy:  accountSignPub,
				}
			})

			By("User credentials are stored")
			secretClientMock.mockApply(ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string"))

			err := userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
			Expect(user.GetLabels()[k8s.LabelUserAccountID]).To(Equal(accountID))
			Expect(user.GetLabels()[k8s.LabelUserSignedBy]).To(Equal(accountSignPub))
			Expect(user.Status.Claims.NatsLimits.Subs).To(Equal(&subsLimit))
		})

		It("updates an existing user", func() {
			By("providing a fake existing account and signing key")
			accountRoot, _ := nkeys.CreateAccount()
			accountID, _ := accountRoot.PublicKey()
			accountSign, _ := nkeys.CreateAccount()
			accountSignPub, _ := accountSign.PublicKey()

			By("providing a user specification bound to the existing account")
			user := GetNewUser()
			var subsLimit int64 = 43
			user.Spec.NatsLimits.Subs = &subsLimit
			user.Labels = make(map[string]string)
			user.Labels[k8s.LabelUserID] = "fake-prev-user-pub-key"
			user.Labels[k8s.LabelUserAccountID] = accountID
			user.Labels[k8s.LabelUserSignedBy] = "fake-prev-sign-pub-key"

			By("mocking user signing")
			userJWTSignerMock.mockSignUserJWT(ctx, domain.NewNamespacedName(accountNamespace, accountName), func(claims *jwt.UserClaims) *SignedUserJWT {
				Expect(claims.IssuerAccount).To(Equal(accountID))
				userJWT, err := claims.Encode(accountSign)
				Expect(err).NotTo(HaveOccurred())
				return &SignedUserJWT{
					UserJWT:   userJWT,
					AccountID: accountID,
					SignedBy:  accountSignPub,
				}
			})

			By("User credentials are stored")
			secretClientMock.mockApply(ctx, mock.Anything, mock.MatchedBy(func(s v1.ObjectMeta) bool {
				return s.GetName() == user.GetUserSecretName() && s.GetNamespace() == accountNamespace
			}), mock.AnythingOfType("map[string]string"))

			err := userManager.CreateOrUpdate(ctx, user)

			Expect(err).ToNot(HaveOccurred())
			Expect(user.GetLabels()).ToNot(BeNil())
			Expect(user.GetLabels()[k8s.LabelUserID]).Should(Satisfy(isUserPubKey))
			Expect(user.GetLabels()[k8s.LabelUserAccountID]).To(Equal(accountID))
			Expect(user.GetLabels()[k8s.LabelUserSignedBy]).To(Equal(accountSignPub))
			Expect(user.Status.Claims.NatsLimits.Subs).To(Equal(&subsLimit))
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
