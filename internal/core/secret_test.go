package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SecretManagerTestSuite struct {
	suite.Suite
	ctx context.Context

	secretClientMock *SecretClientMock
	unitUnderTest    *secretManagerImpl
}

func (t *SecretManagerTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.secretClientMock = NewSecretClientMock()

	var err error
	t.unitUnderTest, err = newSecretManagerImpl(t.secretClientMock)
	t.NoError(err)
}

func (t *SecretManagerTestSuite) TearDownTest() {
	t.secretClientMock.AssertExpectations(t.T())
}

func TestSecretManager_TestSuite(t *testing.T) {
	suite.Run(t, new(SecretManagerTestSuite))
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldReturnNotFoundAfterTryingMultipleLookups() {
	// Given
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: "FAKE_ACCOUNT_ID",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "my-account",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-root"))
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-sign"))

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "my-account"), "FAKE_ACCOUNT_ID")

	// Then
	t.NoError(err)
	t.False(found)
	t.Nil(result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_LookupByAccountIDLabel() {
	// Given
	account := testutil.CreateNatsTestAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      account.Root.Seed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      account.Sign.Seed,
		},
	})

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.PublicKey)

	// Then
	t.NoError(err)
	t.True(found)
	t.NotNil(result)
	t.Equal(&Secrets{Root: account.Root.Key, Sign: account.Sign.Key}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_LookupByAccountNameLabel() {
	// Given
	account := testutil.CreateNatsTestAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "account-name",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      account.Root.Seed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      account.Sign.Seed,
		},
	})

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.PublicKey)

	// Then
	t.NoError(err)
	t.True(found)
	t.NotNil(result)
	t.Equal(&Secrets{Root: account.Root.Key, Sign: account.Sign.Key}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_DeprecatedLookupBySecretName() {
	// Given
	account := testutil.CreateNatsTestAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "my-account",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{})
	acRootRef := domain.NewNamespacedName("account-namespace", "my-account-ac-root")
	t.secretClientMock.mockGet(t.ctx, acRootRef, map[string]string{
		k8s.DefaultSecretKeyName: string(account.Root.Seed),
	})
	t.secretClientMock.mockLabel(acRootRef, map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelSecretType:  k8s.SecretTypeAccountRoot,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	})
	acSignRef := domain.NewNamespacedName("account-namespace", "my-account-ac-sign")
	t.secretClientMock.mockGet(t.ctx, acSignRef, map[string]string{
		k8s.DefaultSecretKeyName: string(account.Sign.Seed),
	})
	t.secretClientMock.mockLabel(acSignRef, map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelSecretType:  k8s.SecretTypeAccountSign,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	})

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "my-account"), account.Root.PublicKey)

	// Then
	t.NoError(err)
	t.True(found)
	t.NotNil(result)
	t.Equal(&Secrets{Root: account.Root.Key, Sign: account.Sign.Key}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_DeprecatedLookupBySecretNameWhenLabelFails() {
	// Given
	account := testutil.CreateNatsTestAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "my-account",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{})
	acRootRef := domain.NewNamespacedName("account-namespace", "my-account-ac-root")
	t.secretClientMock.mockGet(t.ctx, acRootRef, map[string]string{
		k8s.DefaultSecretKeyName: string(account.Root.Seed),
	})
	t.secretClientMock.mockLabel(acRootRef, map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelSecretType:  k8s.SecretTypeAccountRoot,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	})
	acSignRef := domain.NewNamespacedName("account-namespace", "my-account-ac-sign")
	t.secretClientMock.mockGet(t.ctx, acSignRef, map[string]string{
		k8s.DefaultSecretKeyName: string(account.Sign.Seed),
	})
	t.secretClientMock.mockLabelError(acSignRef, map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelSecretType:  k8s.SecretTypeAccountSign,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, fmt.Errorf("something went wrong"))

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "my-account"), account.Root.PublicKey)

	// Then
	t.NoError(err)
	t.True(found)
	t.NotNil(result)
	t.Equal(&Secrets{Root: account.Root.Key, Sign: account.Sign.Key}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldReturnNotFound_WhenSecretsAreMissing() {
	// Given
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: "FAKE_ACCOUNT_ID",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "my-account",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-root"))
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-sign"))

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "my-account"), "FAKE_ACCOUNT_ID")

	// Then
	t.NoError(err)
	t.False(found)
	t.Nil(result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldFail_WhenLookupFailsUnexpectedly() {
	// Given
	t.secretClientMock.mockGetByLabelsError("account-namespace", map[string]string{
		SecretLabelAccountID: "FAKE_ACCOUNT_ID",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, fmt.Errorf("boom"))
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "my-account",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-root"))
	t.secretClientMock.mockGetNotFound(domain.NewNamespacedName("account-namespace", "my-account-ac-sign"))

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "my-account"), "FAKE_ACCOUNT_ID")

	// Then
	t.Error(err)
	t.False(found)
	t.Nil(result)
	t.ErrorContains(err, "failed to get account secrets by account ID")
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldFail_WhenSecretRootPubKeyDoesNotMatchSuppliedAccountID() {
	// Given
	account := testutil.CreateNatsTestAccount()
	secretRoot := testutil.CreateNatsTestAccountKey() // Generate a different root key to simulate mismatchsmatch

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountID: account.Root.PublicKey,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		SecretLabelAccountName: "account-name",
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      secretRoot.Seed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      account.Sign.Seed,
		},
	})

	// When
	result, found, err := t.unitUnderTest.GetSecrets(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.PublicKey)

	// Then
	t.Error(err)
	t.True(found)
	t.Nil(result)
	t.ErrorContains(err, fmt.Sprintf("account root public key (%s) in found secret does not match expected account ID (%s)", secretRoot.PublicKey, account.Root.PublicKey))
}

func (t *SecretManagerTestSuite) Test_ApplyRootSecret_ShouldSucceed() {
	// Given
	account := testutil.CreateNatsTestAccount()

	var caughtMeta metav1.ObjectMeta
	t.secretClientMock.mockApply(
		t.ctx,
		nil,
		mock.Anything,
		map[string]string{
			k8s.DefaultSecretKeyName: string(account.Root.Seed),
		},
	).Run(func(args mock.Arguments) {
		caughtMeta = args.Get(2).(metav1.ObjectMeta)
	}).Return(nil)

	// When
	err := t.unitUnderTest.ApplyRootSecret(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.Key)

	// Then
	t.NoError(err)
	t.NotNil(caughtMeta)
	t.Equal("account-namespace", caughtMeta.Namespace)
	t.Contains(caughtMeta.Name, "account-name-ac-root-")
	t.Equal(account.Root.PublicKey, caughtMeta.Labels[SecretLabelAccountID])
	t.Equal("account-name", caughtMeta.Labels[SecretLabelAccountName])
	t.Equal(k8s.SecretTypeAccountRoot, caughtMeta.Labels[k8s.LabelSecretType])
	t.Equal(k8s.LabelManagedValue, caughtMeta.Labels[k8s.LabelManaged])
}

func (t *SecretManagerTestSuite) Test_ApplySignSecret_ShouldSucceed() {
	// Given
	account := testutil.CreateNatsTestAccount()

	var caughtMeta metav1.ObjectMeta
	t.secretClientMock.mockApply(
		t.ctx,
		nil,
		mock.Anything,
		map[string]string{
			k8s.DefaultSecretKeyName: string(account.Sign.Seed),
		},
	).Run(func(args mock.Arguments) {
		caughtMeta = args.Get(2).(metav1.ObjectMeta)
	}).Return(nil)

	// When
	err := t.unitUnderTest.ApplySignSecret(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.PublicKey, account.Sign.Key)

	// Then
	t.NoError(err)
	t.NotNil(caughtMeta)
	t.Equal("account-namespace", caughtMeta.Namespace)
	t.Contains(caughtMeta.Name, "account-name-ac-sign-")
	t.Equal(account.Root.PublicKey, caughtMeta.Labels[SecretLabelAccountID])
	t.Equal("account-name", caughtMeta.Labels[SecretLabelAccountName])
	t.Equal(k8s.SecretTypeAccountSign, caughtMeta.Labels[k8s.LabelSecretType])
	t.Equal(k8s.LabelManagedValue, caughtMeta.Labels[k8s.LabelManaged])
}

func (t *SecretManagerTestSuite) Test_DeleteAll_ShouldSucceed() {
	// Given
	account := testutil.CreateNatsTestAccount()

	t.secretClientMock.mockDeleteByLabels("account-namespace", map[string]string{
		SecretLabelAccountID:   account.Root.PublicKey,
		SecretLabelAccountName: "account-name",
	})

	// When
	err := t.unitUnderTest.DeleteAll(t.ctx, domain.NewNamespacedName("account-namespace", "account-name"), account.Root.PublicKey)

	// Then
	t.NoError(err)
}
