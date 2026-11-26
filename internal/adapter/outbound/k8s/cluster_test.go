package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/suite"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	namespace := testutil.ScopedTestName("cluster-ns", t.T().Name())
	t.clusterNsN = domain.NewNamespacedName(namespace, testutil.SanitizeTestName(t.T().Name()))
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://nats:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"default": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"default": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://nats:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromConfigMapWithExplicitNamespace() {
	// Given
	configNamespace := testutil.ScopedTestName("config-ns", t.T().Name())
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
	}, result)
}

func (t *NatsClusterClientTestSuite) Test_GetTarget_ShouldSucceed_WhenNatsURLFromSecretWithExplicitNamespace() {
	// Given
	secretNamespace := testutil.ScopedTestName("secret-ns", t.T().Name())
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
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
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
		"op-sign-seed": string(testData.opSign.Seed),
		"sau-creds":    string(testData.sauCredsData),
	})

	// When
	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	// Then
	t.NoError(err)
	t.NotNil(result)
	t.EqualClusterTarget(&nauth.ClusterTarget{
		NatsURL:            "nats://cm-cluster:4222",
		SystemAdminCreds:   testData.sauCreds,
		OperatorSigningKey: testData.opSign.Key,
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

// baseTLSSpec returns a valid NatsCluster spec with the given TLS settings.
func (t *NatsClusterClientTestSuite) baseTLSSpec(tls *v1alpha1.NatsClusterTLS) v1alpha1.NatsClusterSpec {
	return v1alpha1.NatsClusterSpec{
		URL:                             "tls://nats:4222",
		OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret", Key: "seed"},
		SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds-secret", Key: "user.creds"},
		TLS:                             tls,
	}
}

// createClusterSecrets creates the operator signing key and system account Secrets used by baseTLSSpec.
func (t *NatsClusterClientTestSuite) createClusterSecrets() {
	testData := t.generateTestSecrets()
	t.createSecret(t.clusterNsN.Namespace, "op-sign-secret", map[string]string{"seed": string(testData.opSign.Seed)})
	t.createSecret(t.clusterNsN.Namespace, "sau-creds-secret", map[string]string{"user.creds": string(testData.sauCredsData)})
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_NoTLS_ShouldLeaveTLSNil() {
	t.createNatsCluster(t.baseTLSSpec(nil))
	t.createClusterSecrets()

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().NoError(err)
	t.Nil(result.TLS)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefConfigMap_ShouldLoadBundle() {
	pki := testutil.NewTestPKI(t.T())
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindConfigMap, Name: "nats-ca", Key: "bundle.pem"},
	}))
	t.createClusterSecrets()
	t.createConfigMap(t.clusterNsN.Namespace, "nats-ca", map[string]string{"bundle.pem": string(pki.CAPEM)})

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().NoError(err)
	t.Require().NotNil(result.TLS)
	t.Equal(pki.CAPEM, result.TLS.CABundlePEM)
	t.False(result.TLS.HandshakeFirst)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefSecretDefaultKey_ShouldLoadCaCrt() {
	pki := testutil.NewTestPKI(t.T())
	cluster := t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindSecret, Name: "nats-ca"},
	})
	t.createClusterSecrets()
	t.createSecret(t.clusterNsN.Namespace, "nats-ca", map[string]string{"ca.crt": string(pki.CAPEM)})
	t.createNatsCluster(cluster)

	// Resolve directly with an empty key so the adapter default is exercised, not only the CRD default.
	natsCluster := t.getNatsCluster()
	natsCluster.Spec.TLS.CARef.Key = ""
	result, err := t.unitUnderTest.ResolveClusterTarget(t.ctx, natsCluster)

	t.Require().NoError(err)
	t.Require().NotNil(result.TLS)
	t.Equal(pki.CAPEM, result.TLS.CABundlePEM)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefMissingConfigMap_ShouldFail() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindConfigMap, Name: "missing-ca"},
	}))
	t.createClusterSecrets()

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, "resolve TLS for NatsCluster")
	t.ErrorContains(err, "missing-ca")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefMissingKey_ShouldFail() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindConfigMap, Name: "nats-ca"},
	}))
	t.createClusterSecrets()
	t.createConfigMap(t.clusterNsN.Namespace, "nats-ca", map[string]string{"other": "x"})

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, `has no key "ca.crt"`)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_InvalidCAPEM_ShouldFail() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindConfigMap, Name: "nats-ca"},
	}))
	t.createClusterSecrets()
	t.createConfigMap(t.clusterNsN.Namespace, "nats-ca", map[string]string{"ca.crt": "super-secret-not-a-pem"})

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, "CA bundle contains no PEM certificates")
	t.ErrorContains(err, `configMap `+t.clusterNsN.Namespace+`/nats-ca key "ca.crt"`)
	t.NotContains(err.Error(), "super-secret-not-a-pem")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_InvalidClientCert_ShouldNameSecret() {
	pkiA := testutil.NewTestPKI(t.T())
	pkiB := testutil.NewTestPKI(t.T())
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		ClientCertSecretRef: &v1alpha1.SecretKeyReference{Name: "client-tls"},
	}))
	t.createClusterSecrets()
	t.createSecret(t.clusterNsN.Namespace, "client-tls", map[string]string{
		k8sv1.TLSCertKey:       string(pkiA.ClientCertPEM),
		k8sv1.TLSPrivateKeyKey: string(pkiB.ClientKeyPEM),
	})

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, "invalid client certificate")
	t.ErrorContains(err, `secret `+t.clusterNsN.Namespace+`/client-tls`)
	t.NotContains(err.Error(), "PRIVATE KEY")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_ClientCertSecretBothKeysEmpty_ShouldFail() {
	t.requireEmptyClientCertMaterialRejected("", "")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_ClientCertSecretEmptyCert_ShouldFail() {
	pki := testutil.NewTestPKI(t.T())
	t.requireEmptyClientCertMaterialRejected("", string(pki.ClientKeyPEM))
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_ClientCertSecretEmptyKey_ShouldFail() {
	pki := testutil.NewTestPKI(t.T())
	t.requireEmptyClientCertMaterialRejected(string(pki.ClientCertPEM), "")
}

// requireEmptyClientCertMaterialRejected asserts that a referenced client certificate Secret whose tls.crt
// or tls.key is present but empty fails resolution with an error naming the Secret, instead of being
// treated as "no client certificate".
func (t *NatsClusterClientTestSuite) requireEmptyClientCertMaterialRejected(certPEM, keyPEM string) {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		ClientCertSecretRef: &v1alpha1.SecretKeyReference{Name: "client-tls"},
	}))
	t.createClusterSecrets()
	t.createSecret(t.clusterNsN.Namespace, "client-tls", map[string]string{
		k8sv1.TLSCertKey:       certPEM,
		k8sv1.TLSPrivateKeyKey: keyPEM,
	})

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err, "empty client certificate material must not fall back to no client certificate")
	t.Nil(result)
	t.ErrorContains(err, `secret `+t.clusterNsN.Namespace+`/client-tls`)
	t.ErrorContains(err, "tls.crt and tls.key must not be empty")
	t.NotContains(err.Error(), "PRIVATE KEY")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefUnsupportedKind_ShouldFail() {
	t.createNatsCluster(t.baseTLSSpec(nil))
	t.createClusterSecrets()
	// The CRD enum rejects this at admission; resolve directly to exercise the adapter guard.
	natsCluster := t.getNatsCluster()
	natsCluster.Spec.TLS = &v1alpha1.NatsClusterTLS{CARef: &v1alpha1.TLSCAReference{Kind: "Pod", Name: "a"}}

	_, err := t.unitUnderTest.ResolveClusterTarget(t.ctx, natsCluster)

	t.Require().Error(err)
	t.ErrorContains(err, `unsupported caRef.kind "Pod"`)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_CARefMissingSecret_ShouldNameSecret() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		CARef: &v1alpha1.TLSCAReference{Kind: v1alpha1.TLSCARefKindSecret, Name: "missing-ca"},
	}))
	t.createClusterSecrets()

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, "resolve TLS for NatsCluster")
	t.ErrorContains(err, `secret `+t.clusterNsN.Namespace+`/missing-ca not found`)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_ClientCertSecret_ShouldLoadPair() {
	pki := testutil.NewTestPKI(t.T())
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		ClientCertSecretRef: &v1alpha1.SecretKeyReference{Name: "client-tls"},
	}))
	t.createClusterSecrets()
	t.createSecret(t.clusterNsN.Namespace, "client-tls", map[string]string{
		k8sv1.TLSCertKey:       string(pki.ClientCertPEM),
		k8sv1.TLSPrivateKeyKey: string(pki.ClientKeyPEM),
	})

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().NoError(err)
	t.Require().NotNil(result.TLS)
	t.Equal(pki.ClientCertPEM, result.TLS.ClientCertPEM)
	t.Equal(pki.ClientKeyPEM, result.TLS.ClientKeyPEM)
	t.Empty(result.TLS.CABundlePEM)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_ClientCertSecretMissingTLSKey_ShouldFail() {
	pki := testutil.NewTestPKI(t.T())
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{
		ClientCertSecretRef: &v1alpha1.SecretKeyReference{Name: "client-tls"},
	}))
	t.createClusterSecrets()
	t.createSecret(t.clusterNsN.Namespace, "client-tls", map[string]string{k8sv1.TLSCertKey: string(pki.ClientCertPEM)})

	_, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().Error(err)
	t.ErrorContains(err, "tls.key")
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_HandshakeFirstOnly_ShouldSetFlag() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{HandshakeFirst: true}))
	t.createClusterSecrets()

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().NoError(err)
	t.Require().NotNil(result.TLS)
	t.True(result.TLS.HandshakeFirst)
	t.Empty(result.TLS.CABundlePEM)
}

func (t *NatsClusterClientTestSuite) Test_ResolveClusterTarget_EmptyTLS_ShouldResolveNonNilConfig() {
	t.createNatsCluster(t.baseTLSSpec(&v1alpha1.NatsClusterTLS{}))
	t.createClusterSecrets()

	result, err := t.unitUnderTest.GetTarget(t.ctx, t.clusterRef)

	t.Require().NoError(err)
	t.Require().NotNil(result.TLS, "an empty spec.tls must still reach the NATS adapter so TLS is required")
	t.Empty(result.TLS.CABundlePEM)
	t.False(result.TLS.HasClientCert())
	t.False(result.TLS.HandshakeFirst)
}

// getNatsCluster returns the suite's NatsCluster as stored by the API server.
func (t *NatsClusterClientTestSuite) getNatsCluster() *v1alpha1.NatsCluster {
	cluster := &v1alpha1.NatsCluster{}
	t.Require().NoError(k8sClient.Get(t.ctx, client.ObjectKey{Namespace: t.clusterNsN.Namespace, Name: t.clusterNsN.Name}, cluster))
	return cluster
}

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
	opSign       testutil.NatsTestOperatorKey
	sauCredsData []byte
	sauCreds     domain.NatsUserCreds
}

func (t *NatsClusterClientTestSuite) generateTestSecrets() clusterTestSecrets {
	opSign := testutil.CreateNatsTestOperatorKey()
	acRoot := testutil.CreateNatsTestAccountKey()
	user := testutil.CreateNatsTestUserKey()

	sauClaims := jwt.NewUserClaims(user.PublicKey)
	sauClaims.IssuerAccount = acRoot.PublicKey

	sauJwt, err := sauClaims.Encode(acRoot.Key)
	t.Require().Nilf(err, "failed to encode SAU JWT: %v", err)
	sauCredsData, err := jwt.FormatUserConfig(sauJwt, user.Seed)
	t.Require().Nilf(err, "failed to format SAU creds: %v", err)
	sauCreds, err := domain.NewNatsUserCreds(sauCredsData)
	t.Require().Nilf(err, "failed to create NatsUserCreds from SAU creds: %v", err)
	return clusterTestSecrets{
		opSign:       opSign,
		sauCredsData: sauCredsData,
		sauCreds:     *sauCreds,
	}
}

func (t *NatsClusterClientTestSuite) EqualClusterTarget(expected *nauth.ClusterTarget, result *nauth.ClusterTarget) {
	t.Require().Equal(expected.NatsURL, result.NatsURL)
	t.Require().Equal(expected.SystemAdminCreds.AccountID, result.SystemAdminCreds.AccountID)
	t.Require().Equal(expected.SystemAdminCreds.Creds, result.SystemAdminCreds.Creds)
	t.Require().Equal(expected.OperatorSigningKey, result.OperatorSigningKey)
	t.Require().NotEmpty(result.UID)
}
