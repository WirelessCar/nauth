package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/nkeys"
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

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldDoMultipleLookups() {
	// Given
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: "FAKE_ACCOUNT_ID",
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountName: "my-account",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetError("account-namespace", "my-account-ac-root", fmt.Errorf("root_not_found"))
	t.secretClientMock.mockGetError("account-namespace", "my-account-ac-sign", fmt.Errorf("sign_not_found"))

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "my-account", "FAKE_ACCOUNT_ID")

	// Then
	t.Error(err)
	t.ErrorContains(err, "failed to get account secrets by account ID \"FAKE_ACCOUNT_ID\"")
	t.ErrorContains(err, "failed to get account secrets by account name \"my-account\"")
	t.ErrorContains(err, "failed to get account secrets by secret name (deprecated) for account name \"my-account\"")
	t.NotContains(err.Error(), "panicked go routine") // Ensure getDeprecatedAccountSecretsByName parallel lookup does not panic
	t.Nil(result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_LookupByAccountIDLabel() {
	// Given
	rootKey, rootSeed, rootPub := t.generateAccount()
	signKey, signSeed, _ := t.generateAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: rootPub,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      rootSeed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      signSeed,
		},
	})

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "account-name", rootPub)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(&Secrets{Root: rootKey, Sign: signKey}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_LookupByAccountNameLabel() {
	// Given
	rootKey, rootSeed, rootPub := t.generateAccount()
	signKey, signSeed, _ := t.generateAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: rootPub,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{})

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountName: "account-name",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      rootSeed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      signSeed,
		},
	})

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "account-name", rootPub)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(&Secrets{Root: rootKey, Sign: signKey}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_DeprecatedLookupBySecretName() {
	// Given
	rootKey, rootSeed, rootPub := t.generateAccount()
	signKey, signSeed, _ := t.generateAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: rootPub,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountName: "my-account",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGet(t.ctx, "account-namespace", "my-account-ac-root", map[string]string{
		k8s.DefaultSecretKeyName: string(rootSeed),
	})
	t.secretClientMock.mockLabel("account-namespace", "my-account-ac-root", map[string]string{
		k8s.LabelAccountID:  rootPub,
		k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	})
	t.secretClientMock.mockGet(t.ctx, "account-namespace", "my-account-ac-sign", map[string]string{
		k8s.DefaultSecretKeyName: string(signSeed),
	})
	t.secretClientMock.mockLabel("account-namespace", "my-account-ac-sign", map[string]string{
		k8s.LabelAccountID:  rootPub,
		k8s.LabelSecretType: k8s.SecretTypeAccountSign,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	})

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "my-account", rootPub)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(&Secrets{Root: rootKey, Sign: signKey}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldSucceed_DeprecatedLookupBySecretNameWhenLabelFails() {
	// Given
	rootKey, rootSeed, rootPub := t.generateAccount()
	signKey, signSeed, _ := t.generateAccount()

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: rootPub,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountName: "my-account",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{})
	t.secretClientMock.mockGet(t.ctx, "account-namespace", "my-account-ac-root", map[string]string{
		k8s.DefaultSecretKeyName: string(rootSeed),
	})
	t.secretClientMock.mockLabel("account-namespace", "my-account-ac-root", map[string]string{
		k8s.LabelAccountID:  rootPub,
		k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	})
	t.secretClientMock.mockGet(t.ctx, "account-namespace", "my-account-ac-sign", map[string]string{
		k8s.DefaultSecretKeyName: string(signSeed),
	})
	t.secretClientMock.mockLabelError("account-namespace", "my-account-ac-sign", map[string]string{
		k8s.LabelAccountID:  rootPub,
		k8s.LabelSecretType: k8s.SecretTypeAccountSign,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	}, fmt.Errorf("something went wrong"))

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "my-account", rootPub)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Equal(&Secrets{Root: rootKey, Sign: signKey}, result)
}

func (t *SecretManagerTestSuite) Test_GetSecrets_ShouldFail_WhenSecretRootPubKeyDoesNotMatchSuppliedAccountID() {
	// Given
	_, _, rootPub := t.generateAccount()
	_, signSeed, _ := t.generateAccount()
	_, secretRootSeed, secretRootPub := t.generateAccount() // Generate a different root key to simulate mismatch

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountID: rootPub,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}, []mockSecret{})

	t.secretClientMock.mockGetByLabelsSimplified("account-namespace", map[string]string{
		k8s.LabelAccountName: "account-name",
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}, []mockSecret{
		{
			SecretType: k8s.SecretTypeAccountRoot,
			Value:      secretRootSeed,
		},
		{
			SecretType: k8s.SecretTypeAccountSign,
			Value:      signSeed,
		},
	})

	// When
	result, err := t.unitUnderTest.GetSecrets(t.ctx, "account-namespace", "account-name", rootPub)

	// Then
	t.Error(err)
	t.Nil(result)
	t.ErrorContains(err, fmt.Sprintf("account root public key (%s) in found secret does not match expected account ID (%s)", secretRootPub, rootPub))
}

func (t *SecretManagerTestSuite) Test_ApplyRootSecret_ShouldSucceed() {
	// Given
	rootKey, rootSeed, rootPub := t.generateAccount()

	var caughtMeta metav1.ObjectMeta
	t.secretClientMock.mockApply(
		t.ctx,
		nil,
		mock.Anything,
		map[string]string{
			k8s.DefaultSecretKeyName: string(rootSeed),
		},
	).Run(func(args mock.Arguments) {
		caughtMeta = args.Get(2).(metav1.ObjectMeta)
	}).Return(nil)

	// When
	err := t.unitUnderTest.ApplyRootSecret(t.ctx, "account-namespace", "account-name", rootKey)

	// Then
	t.NoError(err)
	t.NotNil(caughtMeta)
	t.Equal("account-namespace", caughtMeta.Namespace)
	t.Contains(caughtMeta.Name, "account-name-ac-root-")
	t.Equal(rootPub, caughtMeta.Labels[k8s.LabelAccountID])
	t.Equal("account-name", caughtMeta.Labels[k8s.LabelAccountName])
	t.Equal(k8s.SecretTypeAccountRoot, caughtMeta.Labels[k8s.LabelSecretType])
	t.Equal(k8s.LabelManagedValue, caughtMeta.Labels[k8s.LabelManaged])
}

func (t *SecretManagerTestSuite) Test_ApplySignSecret_ShouldSucceed() {
	// Given
	_, _, rootPub := t.generateAccount()
	signKey, signSeed, _ := t.generateAccount()

	var caughtMeta metav1.ObjectMeta
	t.secretClientMock.mockApply(
		t.ctx,
		nil,
		mock.Anything,
		map[string]string{
			k8s.DefaultSecretKeyName: string(signSeed),
		},
	).Run(func(args mock.Arguments) {
		caughtMeta = args.Get(2).(metav1.ObjectMeta)
	}).Return(nil)

	// When
	err := t.unitUnderTest.ApplySignSecret(t.ctx, "account-namespace", "account-name", rootPub, signKey)

	// Then
	t.NoError(err)
	t.NotNil(caughtMeta)
	t.Equal("account-namespace", caughtMeta.Namespace)
	t.Contains(caughtMeta.Name, "account-name-ac-sign-")
	t.Equal(rootPub, caughtMeta.Labels[k8s.LabelAccountID])
	t.Equal("account-name", caughtMeta.Labels[k8s.LabelAccountName])
	t.Equal(k8s.SecretTypeAccountSign, caughtMeta.Labels[k8s.LabelSecretType])
	t.Equal(k8s.LabelManagedValue, caughtMeta.Labels[k8s.LabelManaged])
}

func (t *SecretManagerTestSuite) Test_DeleteAll_ShouldSucceed() {
	// Given
	_, _, rootPub := t.generateAccount()

	t.secretClientMock.mockDeleteByLabels("account-namespace", map[string]string{
		k8s.LabelAccountID:   rootPub,
		k8s.LabelAccountName: "account-name",
	})

	// When
	err := t.unitUnderTest.DeleteAll(t.ctx, "account-namespace", "account-name", rootPub)

	// Then
	t.NoError(err)
}

/* ****************************************************
* Helpers
*****************************************************/

func (t *SecretManagerTestSuite) generateAccount() (key nkeys.KeyPair, seed []byte, pub string) {
	key, _ = nkeys.CreateAccount()
	seed, _ = key.Seed()
	pub, _ = key.PublicKey()
	return key, seed, pub
}
