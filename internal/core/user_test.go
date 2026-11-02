package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	k8s "github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
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
			ExpiresAt:   expiresAt.Format(time.RFC3339),
		},
	}
	expiresAtTime := expiresAt.Time
	request := nauth.UserRequest{
		UserRef:    domain.NewNamespacedName("my-namespace", "my-user"),
		AccountRef: domain.NewNamespacedName("my-namespace", "my-account"),
		ExpiresAt:  &expiresAtTime,
		Owner:      user,
	}
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds"))

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
		mock.MatchedBy(func(owner v1.Object) bool {
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
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.NotNil(signedUserJWT, "signedUserJWT not set")
	t.NotNil(caughtSecrets, "caughtSecrets not set")

	t.NotEmpty(result.UserID, "UserID should not be empty")
	t.Equal(accountKeys.AccountID(), result.AccountID)
	t.Equal(accountKeys.Sign.PublicKey, result.SignedBy)
	t.verifySecret(accountKeys.Sign.PublicKey, accountKeys.AccountID(), string(result.UserID), &expiresAt, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldSucceed_WhenUpdatedUser() {
	// Given
	accountKeys := testutil.CreateNatsTestAccount()
	existingUserKey := testutil.CreateNatsTestUserKey()
	existingCreds := t.userCredentials(accountKeys, existingUserKey)

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-user",
			Namespace: "my-namespace",
			Labels: map[string]string{
				string(v1alpha1.UserLabelAccountID): accountKeys.AccountID(),
				string(v1alpha1.UserLabelUserID):    existingUserKey.PublicKey,
				string(v1alpha1.UserLabelSignedBy):  "fake-prev-sign-pub-key",
			},
		},
		Spec: v1alpha1.UserSpec{
			AccountName: "my-account",
		},
	}
	request := nauth.UserRequest{
		UserRef:        domain.NewNamespacedName("my-namespace", "my-user"),
		ExpectedUserID: nauth.UserID(existingUserKey.PublicKey),
		AccountRef:     domain.NewNamespacedName("my-namespace", "my-account"),
		AccountID:      nauth.AccountID(accountKeys.AccountID()),
		Owner:          user,
	}
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds"), map[string]string{
		k8s.UserCredentialSecretKeyName: string(existingCreds),
	})

	var signedUserJWT *SignedUserJWT = nil
	t.userJWTSignerMock.mockSignUserJWT(t.ctx, domain.NewNamespacedName("my-namespace", "my-account"),
		func(claims *jwt.UserClaims) *SignedUserJWT {
			t.Nil(signedUserJWT, "signedUserJWT should only be set once")
			t.Equal(existingUserKey.PublicKey, claims.Subject, "the existing User ID should be retained")
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
		mock.MatchedBy(func(owner v1.Object) bool {
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
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.NotNil(signedUserJWT, "signedUserJWT not set")
	t.NotNil(caughtSecrets, "caughtSecrets not set")

	t.Equal(nauth.UserID(existingUserKey.PublicKey), result.UserID)
	t.Equal(accountKeys.AccountID(), result.AccountID)
	t.Equal(accountKeys.Sign.PublicKey, result.SignedBy)
	t.verifySecret(accountKeys.Sign.PublicKey, accountKeys.AccountID(), string(result.UserID), nil, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldReuseExistingUserCredentials() {
	// Given
	accountKeys := testutil.CreateNatsTestAccount()
	existingUserKey := testutil.CreateNatsTestUserKey()
	existingCreds := t.userCredentials(accountKeys, existingUserKey)

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{Name: "my-user", Namespace: "my-namespace"},
	}
	userRef := domain.NewNamespacedName("my-namespace", "my-user")
	secretRef := domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds")
	t.secretClientMock.mockGet(t.ctx, secretRef, map[string]string{
		k8s.UserCredentialSecretKeyName: string(existingCreds),
	})

	t.userJWTSignerMock.mockSignUserJWT(t.ctx, domain.NewNamespacedName("my-namespace", "my-account"),
		func(claims *jwt.UserClaims) *SignedUserJWT {
			t.Equal(existingUserKey.PublicKey, claims.Subject)
			claims.IssuerAccount = accountKeys.AccountID()
			userJWT, err := claims.Encode(accountKeys.Sign.Key)
			t.Require().NoError(err)
			return &SignedUserJWT{
				UserJWT:   userJWT,
				AccountID: accountKeys.AccountID(),
				SignedBy:  accountKeys.Sign.PublicKey,
			}
		})
	var caughtSecrets map[string]string
	t.secretClientMock.mockApplyWithCatch(t.ctx, user, mock.Anything, mock.Anything, func(secret map[string]string) {
		caughtSecrets = secret
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.UserRequest{
		UserRef:    userRef,
		AccountRef: domain.NewNamespacedName("my-namespace", "my-account"),
		AccountID:  nauth.AccountID(accountKeys.AccountID()),
		Owner:      user,
	})

	// Then
	t.Require().NoError(err)
	t.Equal(nauth.UserID(existingUserKey.PublicKey), result.UserID)
	t.verifySecret(accountKeys.Sign.PublicKey, accountKeys.AccountID(), existingUserKey.PublicKey, nil, caughtSecrets)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenExpectedUserIDHasNoCredentialsSecret() {
	// Given
	secretRef := domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds")
	t.secretClientMock.mockGetNotFound(secretRef)

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.UserRequest{
		UserRef:        domain.NewNamespacedName("my-namespace", "my-user"),
		ExpectedUserID: nauth.UserID("expected-user-id"),
		AccountRef:     domain.NewNamespacedName("my-namespace", "my-account"),
	})

	// Then
	t.Error(err)
	t.ErrorIs(err, domain.ErrUserIdentityNotReady)
	t.Nil(result)
	t.Contains(err.Error(), "not found")
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenCredentialsDoNotMatchExpectedUserID() {
	// Given
	accountKeys := testutil.CreateNatsTestAccount()
	existingUserKey := testutil.CreateNatsTestUserKey()
	secretRef := domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds")
	t.secretClientMock.mockGet(t.ctx, secretRef, map[string]string{
		k8s.UserCredentialSecretKeyName: string(t.userCredentials(accountKeys, existingUserKey)),
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.UserRequest{
		UserRef:        domain.NewNamespacedName("my-namespace", "my-user"),
		ExpectedUserID: nauth.UserID("different-user-id"),
		AccountRef:     domain.NewNamespacedName("my-namespace", "my-account"),
	})

	// Then
	t.Error(err)
	t.ErrorIs(err, domain.ErrUserIdentityNotReady)
	t.Nil(result)
	t.Contains(err.Error(), existingUserKey.PublicKey)
	t.Contains(err.Error(), "different-user-id")
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_ValidateIdentity_ShouldSucceed_WhenCredentialsMatchExpectedUserID() {
	accountKeys := testutil.CreateNatsTestAccount()
	userKey := testutil.CreateNatsTestUserKey()
	secretRef := domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds")
	t.secretClientMock.mockGet(t.ctx, secretRef, map[string]string{
		k8s.UserCredentialSecretKeyName: string(t.userCredentials(accountKeys, userKey)),
	})

	err := t.unitUnderTest.ValidateIdentity(t.ctx, domain.NewNamespacedName("my-namespace", "my-user"), nauth.UserID(userKey.PublicKey))

	t.NoError(err)
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_ValidateIdentity_ShouldFail_WhenCredentialsDoNotMatchExpectedUserID() {
	accountKeys := testutil.CreateNatsTestAccount()
	userKey := testutil.CreateNatsTestUserKey()
	secretRef := domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds")
	t.secretClientMock.mockGet(t.ctx, secretRef, map[string]string{
		k8s.UserCredentialSecretKeyName: string(t.userCredentials(accountKeys, userKey)),
	})

	err := t.unitUnderTest.ValidateIdentity(t.ctx, domain.NewNamespacedName("my-namespace", "my-user"), nauth.UserID("different-user-id"))

	t.Error(err)
	t.ErrorIs(err, domain.ErrUserIdentityNotReady)
	t.Contains(err.Error(), userKey.PublicKey)
	t.Contains(err.Error(), "different-user-id")
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenSigningKeySecretRefSetWithoutSigningPublicKey() {
	// Given
	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{Name: "my-user", Namespace: "my-namespace"},
		Spec:       v1alpha1.UserSpec{AccountName: "my-account", SigningKeyRef: &v1alpha1.AccountSigningKeyRef{Name: "external-auth"}},
	}
	request := nauth.UserRequest{
		UserRef:                    domain.NewNamespacedName("my-namespace", "my-user"),
		AccountRef:                 domain.NewNamespacedName("my-namespace", "my-account"),
		Owner:                      user,
		SigningPrivateKeySecretRef: new(domain.NewNamespacedName("my-namespace", "external-auth-secret")),
		// SigningPublicKey deliberately left empty.
	}

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)

	// Then
	t.Require().Error(err)
	t.Nil(result)
	t.Contains(err.Error(), "signing public key is required")
	t.secretClientMock.AssertNotCalled(t.T(), "Get", mock.Anything, mock.Anything)
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenSigningKeySecretDriftedFromTrustedPublicKey() {
	// Given: the controller validated that "trustedKey" is trusted by the Account,
	// but the Secret at the supplied ref now holds a seed for a different account key
	// (drift between trust check and seed read).
	trustedKey := testutil.CreateNatsTestAccountKey()
	driftedKey := testutil.CreateNatsTestAccountKey()
	signingSecretRef := domain.NewNamespacedName("my-namespace", "external-auth-secret")

	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{Name: "my-user", Namespace: "my-namespace"},
		Spec:       v1alpha1.UserSpec{AccountName: "my-account", SigningKeyRef: &v1alpha1.AccountSigningKeyRef{Name: "external-auth"}},
	}
	request := nauth.UserRequest{
		UserRef:                    domain.NewNamespacedName("my-namespace", "my-user"),
		AccountRef:                 domain.NewNamespacedName("my-namespace", "my-account"),
		Owner:                      user,
		SigningPrivateKeySecretRef: &signingSecretRef,
		SigningPublicKey:           trustedKey.PublicKey,
	}

	t.secretClientMock.mockGet(t.ctx, signingSecretRef, map[string]string{
		k8s.DefaultSecretKeyName: string(driftedKey.Seed),
	})

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)

	// Then
	t.Require().Error(err)
	t.Nil(result)
	t.Contains(err.Error(), trustedKey.PublicKey, "error must name the trusted public key")
	t.Contains(err.Error(), driftedKey.PublicKey, "error must name the drifted public key")
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) Test_CreateOrUpdate_ShouldFail_WhenSigningKeySecretRefHasNoTrustedPublicKey() {
	// Given
	signingSecretRef := domain.NewNamespacedName("my-namespace", "external-auth-secret")
	user := &v1alpha1.User{
		ObjectMeta: v1.ObjectMeta{Name: "my-user", Namespace: "my-namespace"},
		Spec:       v1alpha1.UserSpec{AccountName: "my-account", SigningKeyRef: &v1alpha1.AccountSigningKeyRef{Name: "external-auth"}},
	}
	request := nauth.UserRequest{
		UserRef:                    domain.NewNamespacedName("my-namespace", "my-user"),
		AccountRef:                 domain.NewNamespacedName("my-namespace", "my-account"),
		Owner:                      user,
		SigningPrivateKeySecretRef: &signingSecretRef,
	}

	// When
	result, err := t.unitUnderTest.CreateOrUpdate(t.ctx, request)

	// Then
	t.Require().Error(err)
	t.Nil(result)
	t.Contains(err.Error(), "signing public key is required")
	t.secretClientMock.AssertNotCalled(t.T(), "Get", mock.Anything, mock.Anything)
	t.secretClientMock.AssertNotCalled(t.T(), "Apply", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	t.userJWTSignerMock.AssertNotCalled(t.T(), "SignUserJWT", mock.Anything, mock.Anything)
}

func (t *UserManagerTestSuite) verifySecret(accountSignPub string, accountID string, userID string, expectedExpiresAt *v1.Time, secretData map[string]string) {
	t.Contains(secretData, "user.creds")
	userCreds := secretData["user.creds"]
	t.NotEmpty(userCreds, "user.creds in secret data should not be empty")
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

func (t *UserManagerTestSuite) userCredentials(accountKeys testutil.NatsTestAccount, userKey testutil.NatsTestKey) []byte {
	claims := jwt.NewUserClaims(userKey.PublicKey)
	claims.IssuerAccount = accountKeys.AccountID()
	userJWT, err := claims.Encode(accountKeys.Sign.Key)
	t.Require().NoError(err)
	credentials, err := jwt.FormatUserConfig(userJWT, userKey.Seed)
	t.Require().NoError(err)
	return credentials
}

func futureUserExpiresAt() v1.Time {
	now := time.Now().UTC()
	expiresAt := time.Date(now.Year(), time.December, 31, 23, 59, 59, 0, time.UTC)
	if !expiresAt.After(now) {
		expiresAt = time.Date(now.Year()+1, time.December, 31, 23, 59, 59, 0, time.UTC)
	}
	return v1.NewTime(expiresAt)
}

func (t *UserManagerTestSuite) Test_Create_ShouldPreserveNatsLimitDefaultsAndOverrides() {
	zero, unlimited, payload := int64(0), int64(-1), int64(1024)
	tests := []struct {
		name   string
		limits *nauth.NatsLimits
		want   jwt.NatsLimits
	}{
		{name: "omitted", want: jwt.NatsLimits{Subs: -1, Data: -1, Payload: -1}},
		{name: "empty", limits: &nauth.NatsLimits{}, want: jwt.NatsLimits{Subs: -1, Data: -1, Payload: -1}},
		{name: "partial", limits: &nauth.NatsLimits{Payload: &payload}, want: jwt.NatsLimits{Subs: -1, Data: -1, Payload: 1024}},
		{
			name:   "explicit_zero_and_unlimited",
			limits: &nauth.NatsLimits{Subs: &zero, Data: &unlimited, Payload: &payload},
			want:   jwt.NatsLimits{Subs: 0, Data: -1, Payload: 1024},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func() {
			t.userJWTSignerMock.Mock, t.secretClientMock.Mock = mock.Mock{}, mock.Mock{}
			defer t.userJWTSignerMock.AssertExpectations(t.T())
			defer t.secretClientMock.AssertExpectations(t.T())
			keys := testutil.NatsTestAccountA
			accountRef := domain.NewNamespacedName("my-namespace", "my-account")
			user := &v1alpha1.User{ObjectMeta: v1.ObjectMeta{Name: "my-user", Namespace: "my-namespace"}}
			t.userJWTSignerMock.mockSignUserJWT(t.ctx, accountRef, func(claims *jwt.UserClaims) *SignedUserJWT {
				claims.IssuerAccount = keys.AccountID()
				token, err := claims.Encode(keys.Sign.Key)
				t.Require().NoError(err)
				return &SignedUserJWT{UserJWT: token, AccountID: keys.AccountID(), SignedBy: keys.Sign.PublicKey}
			})
			t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("my-namespace", "my-user-nats-user-creds"))
			var credentials map[string]string
			t.secretClientMock.mockApplyWithCatch(t.ctx, user, mock.Anything, mock.Anything, func(data map[string]string) {
				credentials = data
			})
			before, err := json.Marshal(tt.limits)
			t.Require().NoError(err)

			_, err = t.unitUnderTest.CreateOrUpdate(t.ctx, nauth.UserRequest{
				UserRef: domain.NewNamespacedName("my-namespace", "my-user"), AccountRef: accountRef,
				Owner: user, NatsLimits: tt.limits,
			})

			t.Require().NoError(err)
			token, err := jwt.ParseDecoratedJWT([]byte(credentials["user.creds"]))
			t.Require().NoError(err)
			claims, err := jwt.DecodeUserClaims(token)
			t.Require().NoError(err)
			t.Equal(tt.want, claims.NatsLimits)
			after, err := json.Marshal(tt.limits)
			t.Require().NoError(err)
			t.Equal(string(before), string(after))
		})
	}
}
