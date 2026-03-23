package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterTestSuite struct {
	suite.Suite
	ctx                     context.Context
	natsClusterResolverMock *NatsClusterReaderMock
	natsClientMock          *NatsClientMock
	secretClientMock        *SecretClientMock
	configMapResolverMock   *ConfigMapReaderMock
}

func (t *ClusterTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.natsClusterResolverMock = NewNatsClusterReaderMock()
	t.natsClientMock = NewNatsClientMock()
	t.secretClientMock = NewSecretClientMock()
	t.configMapResolverMock = NewConfigMapReaderMock()
}

func (t *ClusterTestSuite) TearDownTest() {
	t.natsClusterResolverMock.AssertExpectations(t.T())
	t.natsClientMock.AssertExpectations(t.T())
	t.secretClientMock.AssertExpectations(t.T())
	t.configMapResolverMock.AssertExpectations(t.T())
}

func TestClusterManager_TestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats")

	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, domain.NewNamespacedName("my-namespace", "my-cluster"),
		&v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-cluster",
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL:                             "nats://my-cluster:4222",
				OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
			},
		})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &clusterTarget{
		NatsURL:            "nats://my-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRef() {
	// Given´
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, domain.NewNamespacedName("ac-namespace", "ac-cluster"),
		&v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ac-namespace",
				Name:      "ac-cluster",
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL:                             "nats://ac-cluster:4222",
				OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
			},
		})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("ac-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("ac-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &clusterTarget{
		NatsURL:            "nats://ac-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRefSameAsNonOptionalOperatorClusterRef() {
	// Given
	clusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(clusterRef, false, "nats")

	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, domain.NewNamespacedName("my-namespace", "my-cluster"),
		&v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-cluster",
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL:                             "nats://my-cluster:4222",
				OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
			},
		})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, clusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &clusterTarget{
		NatsURL:            "nats://my-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldSucceed_WhenAccountClusterRefDifferentFromOptionalOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, true, "nats")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, domain.NewNamespacedName("ac-namespace", "ac-cluster"),
		&v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ac-namespace",
				Name:      "ac-cluster",
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL:                             "nats://ac-cluster:4222",
				OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
			},
		})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("ac-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("ac-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &clusterTarget{
		NatsURL:            "nats://ac-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterRefDifferentFromNonOptionalOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "invalid cluster reference: account cluster reference ac-namespace/ac-cluster does not match required operator cluster op-namespace/op-cluster")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenOperatorClusterNotFound() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats")

	t.natsClusterResolverMock.mockGetNatsClusterError(t.ctx, domain.NewNamespacedName("my-namespace", "my-cluster"),
		fmt.Errorf("the root cause"))

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: failed to resolve NATS cluster my-namespace/my-cluster: the root cause")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenNeitherAccountNorOperatorClusterRefDefined() {
	// Given
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats")

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "no cluster reference provided and no operator cluster configured")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterNotFound() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, true, "nats")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	t.natsClusterResolverMock.mockGetNatsClusterError(t.ctx, domain.NewNamespacedName("ac-namespace", "ac-cluster"),
		fmt.Errorf("the root cause"))

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: failed to resolve NATS cluster ac-namespace/ac-cluster: the root cause")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterTarget_ShouldFail_WhenAccountClusterRefDoesNotContainNamespace() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	acClusterRef := &v1alpha1.NatsClusterRef{
		Name: "ac-cluster",
	}

	// When
	result, err := unitUnderTest.GetClusterTarget(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "invalid account cluster reference: namespace required")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldSucceed_FromConfigMap() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	t.configMapResolverMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "my-config"),
		map[string]string{"theNatsURL": "nats://custom-nats:4222"})

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-cluster",
		},
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind: v1alpha1.URLFromKindConfigMap,
				Name: "my-config",
				Key:  "theNatsURL",
			},
		},
	})

	// Then
	require.NoError(t.T(), err)
	require.Equal(t.T(), "nats://custom-nats:4222", result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldSucceed_FromConfigMapWithExplicitNamespace() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	t.configMapResolverMock.mockGet(t.ctx, domain.NewNamespacedName("config-namespace", "my-config"),
		map[string]string{"theNatsURL": "nats://custom-nats:4222"})

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-cluster",
		},
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind:      v1alpha1.URLFromKindConfigMap,
				Namespace: "config-namespace",
				Name:      "my-config",
				Key:       "theNatsURL",
			},
		},
	})

	// Then
	require.NoError(t.T(), err)
	require.Equal(t.T(), "nats://custom-nats:4222", result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldSucceed_FromSecret() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "my-secret"),
		map[string]string{"theNatsURL": "nats://custom-nats:4222"})

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-cluster",
		},
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind: v1alpha1.URLFromKindSecret,
				Name: "my-secret",
				Key:  "theNatsURL",
			},
		},
	})

	// Then
	require.NoError(t.T(), err)
	require.Equal(t.T(), "nats://custom-nats:4222", result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldSucceed_FromSecretWithExplicitNamespace() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("config-namespace", "my-secret"),
		map[string]string{"theNatsURL": "nats://custom-nats:4222"})

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-cluster",
		},
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind:      v1alpha1.URLFromKindSecret,
				Namespace: "config-namespace",
				Name:      "my-secret",
				Key:       "theNatsURL",
			},
		},
	})

	// Then
	require.NoError(t.T(), err)
	require.Equal(t.T(), "nats://custom-nats:4222", result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldFail_WhenNoNatsURLReferenceProvided() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{})

	// Then
	require.ErrorContains(t.T(), err, "neither url nor urlFrom is set")
	require.Empty(t.T(), result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldFail_WhenUnsupportedFromKindProvided() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind: "NotSoKind",
			},
		},
	})

	// Then
	require.ErrorContains(t.T(), err, "unsupported urlFrom.kind \"NotSoKind\"")
	require.Empty(t.T(), result)
}

func (t *ClusterTestSuite) Test_Validate_ShouldSucceed() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsCluster("my-namespace", "my-cluster", "nats://my-cluster:4222")
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	natsConnMock := NewNatsConnectionMock()

	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})
	t.natsClientMock.mockConnect("nats://my-cluster:4222", sauCreds, natsConnMock)
	natsConnMock.mockVerifySystemAccountAccess()
	natsConnMock.mockDisconnect()

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.NoError(err)
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenOperatorSigningKeySecretMissing() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsCluster("my-namespace", "my-cluster", "nats://my-cluster:4222")
	_, sauCreds := t.generateSecrets()

	t.secretClientMock.mockGetError(domain.NewNamespacedName("my-namespace", "op-sign-secret"), fmt.Errorf("the root cause"))
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.ErrorContains(err, "resolve operator signing key for NatsCluster my-namespace/my-cluster:")
	t.ErrorContains(err, "resolve secret my-namespace/op-sign-secret: the root cause")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenSystemAccountUserCredsSecretMissing() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsCluster("my-namespace", "my-cluster", "nats://my-cluster:4222")
	t.secretClientMock.mockGetError(domain.NewNamespacedName("my-namespace", "sau-creds"), fmt.Errorf("the root cause"))

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.ErrorContains(err, "resolve system account user creds for NatsCluster my-namespace/my-cluster:")
	t.ErrorContains(err, "resolve secret my-namespace/sau-creds: the root cause")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenNatsURLCannotBeResolved() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsClusterFromConfigMap("my-namespace", "my-cluster", "my-config", "theNatsURL")
	t.configMapResolverMock.mockGetError(t.ctx, domain.NewNamespacedName("my-namespace", "my-config"), fmt.Errorf("the root cause"))

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.ErrorContains(err, "resolve NATS URL for NatsCluster my-namespace/my-cluster:")
	t.ErrorContains(err, "get ConfigMap my-namespace/my-config: the root cause")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenNatsConnectionFails() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsCluster("my-namespace", "my-cluster", "nats://my-cluster:4222")
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()

	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})
	t.natsClientMock.mockConnectError("nats://my-cluster:4222", sauCreds, fmt.Errorf("authentication failed"))

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.ErrorContains(err, "connect to NATS cluster using System Account User Credentials: authentication failed")
}

func (t *ClusterTestSuite) Test_Validate_ShouldFail_WhenVerifySystemAccountAccessFails() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()
	cluster := t.newNatsCluster("my-namespace", "my-cluster", "nats://my-cluster:4222")
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	natsConnMock := NewNatsConnectionMock()

	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "op-sign-secret"),
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, domain.NewNamespacedName("my-namespace", "sau-creds"),
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})
	t.natsClientMock.mockConnect("nats://my-cluster:4222", sauCreds, natsConnMock)
	natsConnMock.mockVerifySystemAccountAccessError(fmt.Errorf("permission denied"))
	natsConnMock.mockDisconnect()

	// When
	err := unitUnderTest.Validate(t.ctx, cluster)

	// Then
	t.ErrorContains(err, "verify NATS System Account access: permission denied")
}

func (t *ClusterTestSuite) newUnitUnderTestWithDefaults() *ClusterManager {
	return t.newUnitUnderTest(nil, false, "")
}

func (t *ClusterTestSuite) newUnitUnderTest(opClusterRef *v1alpha1.NatsClusterRef, opClusterOptional bool, opNamespace domain.Namespace) *ClusterManager {
	var operatorNatsCluster *OperatorNatsCluster
	var err error
	if opClusterRef != nil {
		operatorNatsCluster, err = NewOperatorNatsCluster(*opClusterRef, opClusterOptional)
		if err != nil {
			t.Failf("failed to create operator NATS cluster config", "error: %v", err)
			return nil
		}
	}

	config, err := NewConfig(operatorNatsCluster, opNamespace)
	if err != nil {
		t.Failf("failed to create operator config", "error: %v", err)
		return nil
	}

	u, err := NewClusterManager(
		t.natsClusterResolverMock,
		t.natsClientMock,
		t.secretClientMock,
		t.configMapResolverMock,
		config,
	)
	if err != nil {
		t.Failf("failed to create ClusterManager", "error: %v", err)
		return nil
	}

	return u
}

func (t *ClusterTestSuite) newNatsCluster(namespace, name, natsURL string) *v1alpha1.NatsCluster {
	return &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha1.NatsClusterSpec{
			URL:                             natsURL,
			OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
		},
	}
}

func (t *ClusterTestSuite) newNatsClusterFromConfigMap(namespace, name, configMapName, key string) *v1alpha1.NatsCluster {
	return &v1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha1.NatsClusterSpec{
			URLFrom: &v1alpha1.URLFromReference{
				Kind: v1alpha1.URLFromKindConfigMap,
				Name: configMapName,
				Key:  key,
			},
			OperatorSigningKeySecretRef:     v1alpha1.SecretKeyReference{Name: "op-sign-secret"},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{Name: "sau-creds"},
		},
	}
}

func (t *ClusterTestSuite) generateSecrets() (domain.NatsOperatorSigningKey, domain.NatsUserCreds) {
	opSign, _ := nkeys.CreateOperator()

	acKey, _ := nkeys.CreateAccount()
	acKeyPub, _ := acKey.PublicKey()

	sauKey, _ := nkeys.CreateUser()
	sauKeySeed, _ := sauKey.Seed()
	sauKeyPub, _ := sauKey.PublicKey()

	sauClaims := jwt.NewUserClaims(sauKeyPub)
	sauClaims.IssuerAccount = acKeyPub

	sauJwt, err := sauClaims.Encode(acKey)
	if err != nil {
		t.Failf("failed to encode SAU JWT", "error: %v", err)
	}
	sauCreds, err := jwt.FormatUserConfig(sauJwt, sauKeySeed)
	if err != nil {
		t.Failf("failed to format SAU creds", "error: %v", err)
	}
	sauNatsUserCreds, err := domain.NewNatsUserCreds(sauCreds)
	if err != nil {
		t.Failf("failed to create NatsUserCreds from SAU creds", "error: %v", err)
	}

	return opSign, *sauNatsUserCreds
}
