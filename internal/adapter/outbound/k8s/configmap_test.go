package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
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
	t.configMapRef = domain.NewNamespacedName(testNamespace, testutil.SanitizeTestName(t.T().Name()))
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

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldCreateConfigMap_WhenConfigMapDoesNotExist() {
	owner := newConfigMapTestOwner("owner-uid")

	err := t.unitUnderTest.Apply(t.ctx, owner, t.meta(map[string]string{"extra": "label"}), map[string]string{"default": "value"})

	t.Require().NoError(err)
	cm := t.getConfigMap()
	t.Equal(map[string]string{"default": "value"}, cm.Data)
	t.Equal(LabelManagedValue, cm.Labels[LabelManaged])
	t.Equal("label", cm.Labels["extra"])
	t.Require().NotNil(metav1.GetControllerOf(cm))
	t.Equal(owner.UID, metav1.GetControllerOf(cm).UID)
}

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldUpdateConfigMap_WhenControlledBySameOwner() {
	owner := newConfigMapTestOwner("owner-uid")
	t.Require().NoError(t.unitUnderTest.Apply(t.ctx, owner, t.meta(map[string]string{"extra": "old"}), map[string]string{"default": "old"}))

	err := t.unitUnderTest.Apply(t.ctx, owner, t.meta(map[string]string{"extra": "new"}), map[string]string{"default": "new"})

	t.Require().NoError(err)
	cm := t.getConfigMap()
	t.Equal(map[string]string{"default": "new"}, cm.Data)
	t.Equal("new", cm.Labels["extra"])
}

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldSkipUpdate_WhenNothingChanged() {
	owner := newConfigMapTestOwner("owner-uid")
	countingClient := &updateCountingClient{Client: k8sClient}
	unitUnderTest := NewConfigMapClient(countingClient)
	labels := map[string]string{"extra": "label"}
	t.Require().NoError(unitUnderTest.Apply(t.ctx, owner, t.meta(labels), map[string]string{"default": "value"}))

	err := unitUnderTest.Apply(t.ctx, owner, t.meta(labels), map[string]string{"default": "value"})

	t.Require().NoError(err)
	t.Zero(countingClient.updates)
}

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldAdoptConfigMap_WhenManagedWithoutController() {
	managed := t.meta(map[string]string{LabelManaged: LabelManagedValue})
	t.Require().NoError(k8sClient.Create(t.ctx, &v1.ConfigMap{
		ObjectMeta: managed,
		Data:       map[string]string{"default": "old"},
	}))
	owner := newConfigMapTestOwner("owner-uid")

	err := t.unitUnderTest.Apply(t.ctx, owner, t.meta(nil), map[string]string{"default": "new"})

	t.Require().NoError(err)
	cm := t.getConfigMap()
	t.Equal(map[string]string{"default": "new"}, cm.Data)
	t.Require().NotNil(metav1.GetControllerOf(cm))
	t.Equal(owner.UID, metav1.GetControllerOf(cm).UID)
}

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldFailWithConflict_WhenExistingConfigMapNotManaged() {
	t.Require().NoError(k8sClient.Create(t.ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: t.configMapRef.Name, Namespace: t.configMapRef.Namespace},
		Data:       map[string]string{"default": "foreign"},
	}))

	err := t.unitUnderTest.Apply(t.ctx, newConfigMapTestOwner("owner-uid"), t.meta(nil), map[string]string{"default": "new"})

	t.ErrorIs(err, domain.ErrConfigMapConflict)
	cm := t.getConfigMap()
	t.Equal(map[string]string{"default": "foreign"}, cm.Data)
	t.Nil(metav1.GetControllerOf(cm))
}

func (t *ConfigMapClientTestSuite) Test_Apply_ShouldFailWithConflict_WhenControlledByAnotherOwner() {
	t.Require().NoError(t.unitUnderTest.Apply(t.ctx, newConfigMapTestOwner("other-uid"), t.meta(nil), map[string]string{"default": "other"}))

	err := t.unitUnderTest.Apply(t.ctx, newConfigMapTestOwner("owner-uid"), t.meta(nil), map[string]string{"default": "new"})

	t.ErrorIs(err, domain.ErrConfigMapConflict)
	cm := t.getConfigMap()
	t.Equal(map[string]string{"default": "other"}, cm.Data)
	t.Equal(ktypes.UID("other-uid"), metav1.GetControllerOf(cm).UID)
}

func (t *ConfigMapClientTestSuite) meta(labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: t.configMapRef.Name, Namespace: t.configMapRef.Namespace, Labels: labels}
}

func (t *ConfigMapClientTestSuite) getConfigMap() *v1.ConfigMap {
	cm := &v1.ConfigMap{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKey{Namespace: t.configMapRef.Namespace, Name: t.configMapRef.Name}, cm))
	return cm
}

type updateCountingClient struct {
	client.Client
	updates int
}

func (c *updateCountingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updates++
	return c.Client.Update(ctx, obj, opts...)
}

func newConfigMapTestOwner(uid ktypes.UID) *v1alpha1.Account {
	return &v1alpha1.Account{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: testNamespace, UID: uid}}
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
