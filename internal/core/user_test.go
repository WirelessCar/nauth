package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	accountKeys := testutil.CreateNatsTestAccount()
	expiresAt := futureUserExpiresAt()

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
			ExpiresAt:   &expiresAt,
		},
	}

	var signedUserJWT *SignedUserJWT = nil
	t.userJWTSignerMock.mockSignUserJWT(t.ctx, domain.NewNamespacedName("my-namespace", "my-account"),
		func(claims *jwt.UserClaims) *SignedUserJWT {
			t.Nil(signedUserJWT, "signedUserJWT should only be set once")
			t.Empty(claims.IssuerAccount, "IssuerAccount should not be set before Account lookup has occurred")
			claims.IssuerAccount = accountKeys.Root.PublicKey
			userJWT, err := claims.Encode(accountKeys.Sign.Key)
			t.NoError(err, "claims.Encode should not return an error")
			signedUserJWT = &SignedUserJWT{
				UserJWT:   userJWT,
				AccountID: accountKeys.AccountID(),
				SignedBy:  accountKeys.Sign.PublicKey,
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

	userID := user.GetLabel(v1alpha1.UserLabelUserID)
	t.NotEmpty(userID, "UserID label should not be empty")
	t.Equal(accountKeys.AccountID(), user.GetLabel(v1alpha1.UserLabelAccountID))
	t.Equal(accountKeys.Sign.PublicKey, user.GetLabel(v1alpha1.UserLabelSignedBy))
	t.verifySecret(accountKeys.Sign.PublicKey, accountKeys.AccountID(), userID, &expiresAt, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldSucceed_WhenUpdatedUser() {
	// Given
	accountKeys := testutil.CreateNatsTestAccount()

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
			Labels: map[string]string{
				string(v1alpha1.UserLabelAccountID): accountKeys.AccountID(),
				string(v1alpha1.UserLabelUserID):    "fake-prev-user-pub-key",
				string(v1alpha1.UserLabelSignedBy):  "fake-prev-sign-pub-key",
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
			t.Equal(accountKeys.AccountID(), claims.IssuerAccount, "IssuerAccount should match previous Account ID")
			userJWT, err := claims.Encode(accountKeys.Sign.Key)
			t.NoError(err, "claims.Encode should not return an error")
			signedUserJWT = &SignedUserJWT{
				UserJWT:   userJWT,
				AccountID: accountKeys.AccountID(),
				SignedBy:  accountKeys.Sign.PublicKey,
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

	userID := user.GetLabel(v1alpha1.UserLabelUserID)
	t.NotEmpty(userID, "UserID label should not be empty")
	t.Equal(accountKeys.AccountID(), user.GetLabel(v1alpha1.UserLabelAccountID))
	t.Equal(accountKeys.Sign.PublicKey, user.GetLabel(v1alpha1.UserLabelSignedBy))
	t.verifySecret(accountKeys.Sign.PublicKey, accountKeys.AccountID(), userID, nil, caughtSecrets)
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

func (t *UserManagerTestSuite) verifySecret(accountSignPub string, accountID string, userID string, expectedExpiresAt *v1.Time, secretData map[string]string) {
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
	if expectedExpiresAt == nil {
		t.Zero(userClaims.Expires)
	} else {
		t.Equal(expectedExpiresAt.Unix(), userClaims.Expires)
	}
}

func futureUserExpiresAt() v1.Time {
	now := time.Now().UTC()
	expiresAt := time.Date(now.Year(), time.December, 31, 23, 59, 59, 0, time.UTC)
	if !expiresAt.After(now) {
		expiresAt = time.Date(now.Year()+1, time.December, 31, 23, 59, 59, 0, time.UTC)
	}
	return v1.NewTime(expiresAt)
}
