package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapClientTestSuite struct {
	suite.Suite
	ctx          context.Context
	configMapRef domain.NamespacedName

	unitUnderTest *ConfigMapClient
}

func TestConfigMapClient_TestSuite(t *testing.T) {
	suite.Run(t, new(ConfigMapClientTestSuite))
}

func (t *ConfigMapClientTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.configMapRef = domain.NewNamespacedName(testNamespace, sanitizeTestName(t.T().Name()))
	t.Require().NoError(t.configMapRef.Validate())
	t.unitUnderTest = NewConfigMapClient(k8sClient)
	t.Require().NoError(cleanConfigMap(t.ctx, t.configMapRef))
}

func (t *ConfigMapClientTestSuite) TearDownTest() {
	t.Require().NoError(cleanConfigMap(t.ctx, t.configMapRef))
}

func (t *ConfigMapClientTestSuite) Test_Get_ShouldFail_WhenConfigMapDoesNotExist() {
	nonExistingConfigMapRef := domain.NewNamespacedName(testNamespace, "non-existing-configmap")
	t.Require().NoError(nonExistingConfigMapRef.Validate())

	result, err := t.unitUnderTest.Get(t.ctx, nonExistingConfigMapRef)

	t.ErrorIs(err, domain.ErrConfigMapNotFound)
	t.Nil(result)
}

func (t *ConfigMapClientTestSuite) Test_Get_ShouldSucceed_WhenConfigMapContainsData() {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.configMapRef.Name,
			Namespace: t.configMapRef.Namespace,
		},
		Data: map[string]string{
			"url":   "nats://nats.example.com:4222",
			"other": "value",
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, cm))

	data, err := t.unitUnderTest.Get(t.ctx, t.configMapRef)

	t.NoError(err)
	t.Equal("nats://nats.example.com:4222", data["url"])
	t.Equal("value", data["other"])
	t.Len(data, 2)
}

func (t *ConfigMapClientTestSuite) Test_Get_ShouldSucceed_WhenConfigMapContainsBinaryData() {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.configMapRef.Name,
			Namespace: t.configMapRef.Namespace,
		},
		BinaryData: map[string][]byte{
			"url": []byte("nats://nats.example.com:4222"),
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, cm))

	data, err := t.unitUnderTest.Get(t.ctx, t.configMapRef)

	t.NoError(err)
	t.Equal("nats://nats.example.com:4222", data["url"])
	t.Len(data, 1)
}

func (t *ConfigMapClientTestSuite) Test_Get_ShouldSucceed_WhenConfigMapContainsDataAndBinaryData() {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.configMapRef.Name,
			Namespace: t.configMapRef.Namespace,
		},
		Data: map[string]string{
			"data-key": "data-value",
		},
		BinaryData: map[string][]byte{
			"binary-key": []byte("binary-value"),
		},
	}
	t.Require().NoError(k8sClient.Create(t.ctx, cm))

	data, err := t.unitUnderTest.Get(t.ctx, t.configMapRef)

	t.NoError(err)
	t.Equal("data-value", data["data-key"])
	t.Equal("binary-value", data["binary-key"])
	t.Len(data, 2)
}

func cleanConfigMap(ctx context.Context, configMapRef domain.NamespacedName) error {
	cm := &v1.ConfigMap{}
	key := client.ObjectKey{Namespace: configMapRef.Namespace, Name: configMapRef.Name}
	if err := k8sClient.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return k8sClient.Delete(ctx, cm)
}
