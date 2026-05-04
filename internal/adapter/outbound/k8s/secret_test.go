package k8s

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretClientTestSuite struct {
	suite.Suite
	ctx        context.Context
	secretName string
	secretMeta metav1.ObjectMeta
	secretRef  domain.NamespacedName

	unitUnderTest *SecretClient
}

func TestSecretClient_TestSuite(t *testing.T) {
	suite.Run(t, new(SecretClientTestSuite))
}

func (t *SecretClientTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.secretName = sanitizeTestName(t.T().Name())
	t.secretMeta = metav1.ObjectMeta{
		Name:      t.secretName,
		Namespace: testNamespace,
		Labels: map[string]string{
			LabelManaged: LabelManagedValue,
		},
	}
	t.secretRef = domain.NewNamespacedName(t.secretMeta.Namespace, t.secretMeta.Name)
	t.Require().NoError(t.secretRef.Validate())
	t.unitUnderTest = NewSecretClient(k8sClient)
	t.Require().NoError(cleanSecret(t.ctx, t.secretRef))
}

func (t *SecretClientTestSuite) TearDownTest() {
	t.Require().NoError(cleanSecret(t.ctx, t.secretRef))
}

func (t *SecretClientTestSuite) Test_Apply_ShouldSucceed_WhenCreatingAndUpdatingSecret() {
	// Given
	secret := map[string]string{"key": "value"}

	// When
	err := t.unitUnderTest.Apply(t.ctx, nil, t.secretMeta, secret)

	// Then
	t.NoError(err)

	fetchedSecret, found, err := t.unitUnderTest.Get(t.ctx, t.secretRef)
	t.NoError(err)
	t.True(found)
	t.Equal(secret, fetchedSecret)

	newSecret := map[string]string{"key": "new value"}
	err = t.unitUnderTest.Apply(t.ctx, nil, t.secretMeta, newSecret)

	t.NoError(err)

	newFetchedSecret, found, err := t.unitUnderTest.Get(t.ctx, t.secretRef)
	t.NoError(err)
	t.True(found)
	t.Equal(newSecret, newFetchedSecret)
}

func (t *SecretClientTestSuite) Test_Apply_ShouldFail_WhenExistingSecretNotManagedByNauth() {
	testCases := map[string]map[string]string{
		"absent_labels_map":                          nil,
		"empty_labels_map":                           {},
		"irrelevant_labels":                          {"foo": "bar"},
		"existing_managed_label_with_unexpected_val": {LabelManaged: "false"},
	}

	for name, existingSecretLabels := range testCases {
		t.Run(name, func() {
			// Given
			existingSecret := map[string]string{"key": "value"}
			err := k8sClient.Create(t.ctx, &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      t.secretName,
					Namespace: t.secretMeta.Namespace,
					Labels:    existingSecretLabels,
				},
				StringData: existingSecret,
			})
			t.Require().NoError(err)

			// When
			newSecret := map[string]string{"key": "new value"}
			err = t.unitUnderTest.Apply(t.ctx, nil, t.secretMeta, newSecret)

			// Then
			t.Error(err)
			t.EqualError(err, fmt.Sprintf("existing secret %s not managed by nauth", t.secretRef))

			fetchedSecret, found, fetchErr := t.unitUnderTest.Get(t.ctx, t.secretRef)
			t.NoError(fetchErr)
			t.True(found)
			t.Equal(existingSecret, fetchedSecret)

			t.Require().NoError(cleanSecret(t.ctx, t.secretRef))
		})
	}
}

func (t *SecretClientTestSuite) Test_Delete_ShouldSucceed_WhenSecretDoesNotExist() {
	nonExistingSecretRef := domain.NewNamespacedName(testNamespace, "non-existing-secret")
	t.Require().NoError(nonExistingSecretRef.Validate())

	err := t.unitUnderTest.Delete(t.ctx, nonExistingSecretRef)

	t.NoError(err)
}

func (t *SecretClientTestSuite) Test_Get_ShouldFail_WhenSecretDoesNotExist() {
	nonExistingSecretRef := domain.NewNamespacedName(testNamespace, "non-existing-secret")
	t.Require().NoError(nonExistingSecretRef.Validate())

	result, found, err := t.unitUnderTest.Get(t.ctx, nonExistingSecretRef)

	t.NoError(err)
	t.False(found)
	t.Nil(result)
}

func (t *SecretClientTestSuite) Test_Delete_ShouldSucceed_WhenSecretExists() {
	secret := map[string]string{"key": "value"}
	err := t.unitUnderTest.Apply(t.ctx, nil, t.secretMeta, secret)
	t.Require().NoError(err)

	err = t.unitUnderTest.Delete(t.ctx, t.secretRef)

	t.NoError(err)

	result, found, getErr := t.unitUnderTest.Get(t.ctx, t.secretRef)
	t.NoError(getErr)
	t.False(found)
	t.Nil(result)
}

func cleanSecret(ctx context.Context, secretRef domain.NamespacedName) error {
	k8sSecret := &v1.Secret{}

	key := client.ObjectKey{Namespace: secretRef.Namespace, Name: secretRef.Name}
	if err := k8sClient.Get(ctx, key, k8sSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return k8sClient.Delete(ctx, k8sSecret)
}
