package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/suite"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NatsClusterClientTestSuite struct {
	suite.Suite
	ctx        context.Context
	clusterNsN domain.NamespacedName
	clusterRef nauth.ClusterRef

	unitUnderTest *ClusterClient
}

func TestNatsClusterClient_TestSuite(t *testing.T) {
	suite.Run(t, new(NatsClusterClientTestSuite))
}

func (t *NatsClusterClientTestSuite) SetupTest() {
	t.ctx = context.Background()

	namespace := scopedTestName("cluster-ns", t.T().Name())
	t.clusterNsN = domain.NewNamespacedName(namespace, sanitizeTestName(t.T().Name()))
	t.Require().NoError(t.clusterNsN.Validate())

	t.clusterRef = nauth.ClusterRef(t.clusterNsN.String())
	t.Require().NoError(t.clusterRef.Validate())

	secretReader := NewSecretClient(k8sClient)
	configMapReader := NewConfigMapClient(k8sClient)
	t.unitUnderTest = NewClusterClient(k8sClient, secretReader, configMapReader)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed() {
	// Given
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URL: "nats://nats:4222",
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
			Key:  "seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
			Key:  "user.creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://nats:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenSecretsUsingDefaultKey() {
	// Given
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URL: "nats://nats:4222",
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
		},
	})
	testData := t.generateTestSecrets()
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"default": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"default": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://nats:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromConfigMap() {
	// Given
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URLFrom: &v1alpha1.URLFromReference{
			Kind: v1alpha1.URLFromKindConfigMap,
			Name: "url-configmap",
			Key:  "nats.url",
		},
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
			Key:  "seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
			Key:  "user.creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createConfigMap(t.clusterNsN.Namespace, "url-configmap", map[string]string{"nats.url": "nats://cm-cluster:4222"})
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromConfigMapWithExplicitNamespace() {
	// Given
	configNamespace := scopedTestName("config-ns", t.T().Name())
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URLFrom: &v1alpha1.URLFromReference{
			Kind:      v1alpha1.URLFromKindConfigMap,
			Name:      "url-configmap",
			Namespace: configNamespace, // Explicit namespace different from cluster resource namespace
			Key:       "nats.url",
		},
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
			Key:  "seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
			Key:  "user.creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createConfigMap(configNamespace, "url-configmap", map[string]string{"nats.url": "nats://cm-cluster:4222"})
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromSecret() {
	// Given
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URLFrom: &v1alpha1.URLFromReference{
			Kind: v1alpha1.URLFromKindSecret,
			Name: "url-secret",
			Key:  "nats.url",
		},
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
			Key:  "seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
			Key:  "user.creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createSecret(t.clusterNsN.Namespace, "url-secret", map[string]string{"nats.url": "nats://cm-cluster:4222"})
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromSecretWithExplicitNamespace() {
	// Given
	secretNamespace := scopedTestName("secret-ns", t.T().Name())
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URLFrom: &v1alpha1.URLFromReference{
			Kind:      v1alpha1.URLFromKindSecret,
			Name:      "url-secret",
			Namespace: secretNamespace, // Explicit namespace different from cluster resource namespace
			Key:       "nats.url",
		},
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "op-sign-secret",
			Key:  "seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "sau-creds-secret",
			Key:  "user.creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createSecret(secretNamespace, "url-secret", map[string]string{"nats.url": "nats://cm-cluster:4222"})
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSignKeySeed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenAllDetailsInSameSecret() {
	// Given
	t.createNatsCluster(v1alpha1.NatsClusterSpec{
		URLFrom: &v1alpha1.URLFromReference{
			Kind: v1alpha1.URLFromKindSecret,
			Name: "cluster-secret",
			Key:  "nats-url",
		},
		OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
			Name: "cluster-secret",
			Key:  "op-sign-seed",
		},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
			Name: "cluster-secret",
			Key:  "sau-creds",
		},
	})
	testData := t.generateTestSecrets()
	t.createSecret(t.clusterNsN.Namespace, "cluster-secret", map[string]string{
		"nats-url":     "nats://cm-cluster:4222",
		"op-sign-seed": string(testData.opSignKeySeed),
		"sau-creds":    string(testData.sauCredsData),
	})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.Require().Equal(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSignKey,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldFail_WhenClusterRefIsNotNamespacedName() {
	// Given
	clusterRef := nauth.ClusterRef("not a namespaced name")

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, clusterRef)

	// Then
	t.Error(err)
	t.Nil(result)
	t.ErrorContains(err, "invalid cluster reference \"not a namespaced name\" (expected NamespacedName):")
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldFail_WhenNatsClusterResourceDoesNotExist() {
	// Given
	clusterRef := nauth.ClusterRef(testNamespace + "/missing-cluster")
	t.Require().NoError(clusterRef.Validate())

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, clusterRef)

	// Then
	t.Error(err)
	t.Nil(result)
	t.ErrorContains(err, "failed getting NatsCluster resource "+testNamespace+"/missing-cluster")
	t.ErrorContains(err, "not found")
}

func (t *NatsClusterClientTestSuite) Test_resolveNatsURL_ShouldFail_WhenURLAmbiguous() {
	// Given
	cluster := v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.clusterNsN.Name,
			Namespace: t.clusterNsN.Namespace,
		},
		Spec: v1alpha1.NatsClusterSpec{
			URL: "nats://cluster-1:4222",
			URLFrom: &v1alpha1.URLFromReference{
				Kind: v1alpha1.URLFromKindConfigMap,
				Name: "url-configmap",
				Key:  "nats.url",
			},
		},
	}
	t.createConfigMap(t.clusterNsN.Namespace, "url-configmap", map[string]string{"nats.url": "nats://cluster-2:4222"})

	// When
	url, err := t.unitUnderTest.resolveNatsURL(t.ctx, &cluster)

	// Then
	t.Empty(url)
	t.ErrorContains(err, "ambiguous NATS URL, url and urlFrom reference resolve to different URLs (\"nats://cluster-1:4222\" vs \"nats://cluster-2:4222\")")
}

// Helpers

func (t *NatsClusterClientTestSuite) createNatsCluster(spec v1alpha1.NatsClusterSpec) {
	t.Require().NoError(ensureNamespace(t.ctx, t.clusterNsN.Namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.clusterNsN.Name,
			Namespace: t.clusterNsN.Namespace,
		},
		Spec: spec,
	}))
}

func (t *NatsClusterClientTestSuite) createConfigMap(namespace string, resourceName string, data map[string]string) {
	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &k8sv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		Data: data,
	}))
}

func (t *NatsClusterClientTestSuite) createSecret(namespace string, resourceName string, data map[string]string) {
	t.Require().NoError(ensureNamespace(t.ctx, namespace))
	t.Require().NoError(k8sClient.Create(t.ctx, &k8sv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
		},
		StringData: data,
	}))
}

type clusterTestSecrets struct {
	opSignKeySeed []byte
	opSignKey     domain.NatsOperatorSigningKey
	sauCredsData  []byte
	sauCreds      domain.NatsUserCreds
}

func (t *NatsClusterClientTestSuite) generateTestSecrets() clusterTestSecrets {
	opSign, _ := nkeys.CreateOperator()
	opSeed, _ := opSign.Seed()
	opSignKey := domain.NatsOperatorSigningKey(opSign)

	acKey, _ := nkeys.CreateAccount()
	acKeyPub, _ := acKey.PublicKey()

	sauKey, _ := nkeys.CreateUser()
	sauKeySeed, _ := sauKey.Seed()
	sauKeyPub, _ := sauKey.PublicKey()

	sauClaims := jwt.NewUserClaims(sauKeyPub)
	sauClaims.IssuerAccount = acKeyPub

	sauJwt, err := sauClaims.Encode(acKey)
	t.Require().Nilf(err, "failed to encode SAU JWT: %v", err)
	sauCredsData, err := jwt.FormatUserConfig(sauJwt, sauKeySeed)
	t.Require().Nilf(err, "failed to format SAU creds: %v", err)
	sauCreds, err := domain.NewNatsUserCreds(sauCredsData)
	t.Require().Nilf(err, "failed to create NatsUserCreds from SAU creds: %v", err)
	return clusterTestSecrets{
		opSignKeySeed: opSeed,
		opSignKey:     opSignKey,
		sauCredsData:  sauCredsData,
		sauCreds:      *sauCreds,
	}
}
