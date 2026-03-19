package user

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
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

type UserManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	userJWTSignerMock *UserJWTSignerMock
	secretClientMock  *SecretClientMock

	unitUnderTest *UserManager
}

func (t *UserManagerTestSuite) SetupTest() {
	t.ctx = context.Background()

	t.userJWTSignerMock = NewUserJWTSignerMock()
	t.secretClientMock = NewSecretClientMock()

	t.unitUnderTest = NewUserManager(t.userJWTSignerMock, t.secretClientMock)
}

func (t *UserManagerTestSuite) TearDownTest() {
	t.userJWTSignerMock.AssertExpectations(t.T())
	t.secretClientMock.AssertExpectations(t.T())
}

func TestUserManager_TestSuite(t *testing.T) {
	suite.Run(t, new(UserManagerTestSuite))
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldSucceed_WhenNewUser() {
	// Given
	accountRoot, _ := nkeys.CreateAccount()
	accountID, _ := accountRoot.PublicKey()
	accountSign, _ := nkeys.CreateAccount()
	accountSignPub, _ := accountSign.PublicKey()

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}

	var signedUserJWT *SignedUserJWT = nil
	t.userJWTSignerMock.mockSignUserJWT(t.ctx, domain.NewNamespacedName("my-namespace", "my-account"),
		func(claims *jwt.UserClaims) *SignedUserJWT {
			t.Nil(signedUserJWT, "signedUserJWT should only be set once")
			t.Empty(claims.IssuerAccount, "IssuerAccount should not be set before Account lookup has occurred")
			claims.IssuerAccount = accountID
			userJWT, err := claims.Encode(accountSign)
			t.NoError(err, "claims.Encode should not return an error")
			signedUserJWT = &SignedUserJWT{
				UserJWT:   userJWT,
				AccountID: accountID,
				SignedBy:  accountSignPub,
			}
			return signedUserJWT
		})
	var caughtSecrets map[string]string = nil
	t.secretClientMock.mockApplyWithCatch(t.ctx,
		mock.MatchedBy(func(owner *v1alpha1.User) bool {
			return owner == user
		}),
		mock.MatchedBy(func(s v1.ObjectMeta) bool {
			return s.GetName() == "my-user-nats-user-creds" && s.GetNamespace() == "my-namespace"
		}),
		mock.AnythingOfType("map[string]string"), func(secret map[string]string) {
			t.Nil(caughtSecrets, "secretClient.Apply should only be called once")
			caughtSecrets = secret
		})

	// When
	err := t.unitUnderTest.CreateOrUpdate(t.ctx, user)

	// Then
	t.NoError(err)
	t.NotNil(signedUserJWT, "signedUserJWT not set")
	t.NotNil(caughtSecrets, "caughtSecrets not set")

	userID := user.GetLabels()[k8s.LabelUserID]
	t.NotEmpty(userID, "UserID label should not be empty")
	t.Equal(accountID, user.GetLabels()[k8s.LabelUserAccountID])
	t.Equal(accountSignPub, user.GetLabels()[k8s.LabelUserSignedBy])
	t.verifySecret(accountSignPub, accountID, userID, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldSucceed_WhenUpdatedUser() {
	// Given
	accountRoot, _ := nkeys.CreateAccount()
	accountID, _ := accountRoot.PublicKey()
	accountSign, _ := nkeys.CreateAccount()
	accountSignPub, _ := accountSign.PublicKey()

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
			Labels: map[string]string{
				k8s.LabelUserAccountID: accountID,
				k8s.LabelUserID:        "fake-prev-user-pub-key",
				k8s.LabelUserSignedBy:  "fake-prev-sign-pub-key",
			},
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}

	var signedUserJWT *SignedUserJWT = nil
	t.userJWTSignerMock.mockSignUserJWT(t.ctx, domain.NewNamespacedName("my-namespace", "my-account"),
		func(claims *jwt.UserClaims) *SignedUserJWT {
			t.Nil(signedUserJWT, "signedUserJWT should only be set once")
			t.Equal(accountID, claims.IssuerAccount, "IssuerAccount should match previous Account ID")
			userJWT, err := claims.Encode(accountSign)
			t.NoError(err, "claims.Encode should not return an error")
			signedUserJWT = &SignedUserJWT{
				UserJWT:   userJWT,
				AccountID: accountID,
				SignedBy:  accountSignPub,
			}
			return signedUserJWT
		})
	var caughtSecrets map[string]string = nil
	t.secretClientMock.mockApplyWithCatch(t.ctx,
		mock.MatchedBy(func(owner *v1alpha1.User) bool {
			return owner == user
		}),
		mock.MatchedBy(func(s v1.ObjectMeta) bool {
			return s.GetName() == "my-user-nats-user-creds" && s.GetNamespace() == "my-namespace"
		}),
		mock.AnythingOfType("map[string]string"), func(secret map[string]string) {
			t.Nil(caughtSecrets, "secretClient.Apply should only be called once")
			caughtSecrets = secret
		})

	// When
	err := t.unitUnderTest.CreateOrUpdate(t.ctx, user)

	// Then
	t.NoError(err)
	t.NotNil(signedUserJWT, "signedUserJWT not set")
	t.NotNil(caughtSecrets, "caughtSecrets not set")

	userID := user.GetLabels()[k8s.LabelUserID]
	t.NotEmpty(userID, "UserID label should not be empty")
	t.Equal(accountID, user.GetLabels()[k8s.LabelUserAccountID])
	t.Equal(accountSignPub, user.GetLabels()[k8s.LabelUserSignedBy])
	t.verifySecret(accountSignPub, accountID, userID, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_Delete_ShouldSucceed() {
	// Given
	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}
	t.secretClientMock.mockDelete(t.ctx, domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds"))

	// When
	err := t.unitUnderTest.Delete(t.ctx, user)

	// Then
	t.NoError(err)
}

func (t *UserManagerTestSuite) Test_Delete_ShouldFail_WhenDeleteSecretFails() {
	// Given
	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}
	t.secretClientMock.mockDeleteError(t.ctx, domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds"), fmt.Errorf("wops"))

	// When
	err := t.unitUnderTest.Delete(t.ctx, user)

	// Then
	t.ErrorContains(err, "wops")
}

func (t *UserManagerTestSuite) verifySecret(accountSignPub string, accountID string, userID string, secretData map[string]string) {
	t.Contains(secretData, "user.creds")
	userCreds := secretData["user.creds"]
	t.NotEmpty(userCreds, fmt.Sprintf("user.creds in secret data should not be empty. Found: %v", secretData))
	userJWT, err := jwt.ParseDecoratedJWT([]byte(userCreds))
	t.NoError(err, "userCreds should be decorated JWT")
	userClaims, err := jwt.DecodeUserClaims(userJWT)
	t.NoError(err, "failed to decode user claims")
	t.Equal(accountSignPub, userClaims.Issuer)
	t.Equal(accountID, userClaims.IssuerAccount)
	t.Equal("my-namespace/my-user", userClaims.Name)
	t.Equal(userID, userClaims.Subject)
}
