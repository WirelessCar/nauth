package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterTestSuite struct {
	suite.Suite
	ctx                     context.Context
	natsClusterResolverMock *NatsClusterResolverMock
	secretClientMock        *SecretClientMock
	configMapResolverMock   *ConfigMapResolverMock
}

func (t *ClusterTestSuite) SetupTest() {
	t.ctx = context.Background()
	t.natsClusterResolverMock = NewNatsClusterResolverMock()
	t.secretClientMock = NewSecretClientMock()
	t.configMapResolverMock = NewConfigMapResolverMock()
}

func (t *ClusterTestSuite) TearDownTest() {
	t.natsClusterResolverMock.AssertExpectations(t.T())
	t.secretClientMock.AssertExpectations(t.T())
	t.configMapResolverMock.AssertExpectations(t.T())
}

func TestClusterConfigResolver_TestSuite(t *testing.T) {
	suite.Run(t, new(ClusterTestSuite))
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldSucceed_WhenLegacyImplicitLookup() {
	// Given
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats", "nats://nats:4222")

	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.secretClientMock.mockGetByLabelsSimple("nats", map[string]string{k8s.LabelSecretType: k8s.SecretTypeOperatorSign},
		k8s.DefaultSecretKeyName, opSignSeed)
	t.secretClientMock.mockGetByLabelsSimple("nats", map[string]string{k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds},
		k8s.DefaultSecretKeyName, sauCreds.Creds)

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, nil)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &ClusterConfig{
		NatsURL:            "nats://nats:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldSucceed_WhenOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats", "nats://nats:4222")

	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, ports.NamespacedName{Namespace: "my-namespace", Name: "my-cluster"},
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
	t.secretClientMock.mockGet(t.ctx, "my-namespace", "op-sign-secret",
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, "my-namespace", "sau-creds",
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, nil)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &ClusterConfig{
		NatsURL:            "nats://my-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldSucceed_WhenAccountClusterRef() {
	// Given´
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats", "")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, ports.NamespacedName{Namespace: "ac-namespace", Name: "ac-cluster"},
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
	t.secretClientMock.mockGet(t.ctx, "ac-namespace", "op-sign-secret",
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, "ac-namespace", "sau-creds",
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &ClusterConfig{
		NatsURL:            "nats://ac-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldSucceed_WhenAccountClusterRefSameAsNonOptionalOperatorClusterRef() {
	// Given
	clusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(clusterRef, false, "nats", "nats://nats:4222")

	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, ports.NamespacedName{Namespace: "my-namespace", Name: "my-cluster"},
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
	t.secretClientMock.mockGet(t.ctx, "my-namespace", "op-sign-secret",
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, "my-namespace", "sau-creds",
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, clusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &ClusterConfig{
		NatsURL:            "nats://my-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldSucceed_WhenAccountClusterRefDifferentFromOptionalOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, true, "nats", "nats://nats:4222")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	opSignKey, sauCreds := t.generateSecrets()
	opSignSeed, _ := opSignKey.Seed()
	t.natsClusterResolverMock.mockGetNatsCluster(t.ctx, ports.NamespacedName{Namespace: "ac-namespace", Name: "ac-cluster"},
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
	t.secretClientMock.mockGet(t.ctx, "ac-namespace", "op-sign-secret",
		map[string]string{k8s.DefaultSecretKeyName: string(opSignSeed)})
	t.secretClientMock.mockGet(t.ctx, "ac-namespace", "sau-creds",
		map[string]string{k8s.DefaultSecretKeyName: string(sauCreds.Creds)})

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, acClusterRef)

	// Then
	require.NoError(t.T(), err)
	require.NotNil(t.T(), result)
	require.Equal(t.T(), &ClusterConfig{
		NatsURL:            "nats://ac-cluster:4222",
		SystemAdminCreds:   sauCreds,
		OperatorSigningKey: opSignKey,
	}, result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenAccountClusterRefDifferentFromNonOptionalOperatorClusterRef() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats", "nats://nats:4222")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "invalid cluster reference: account cluster reference ac-namespace/ac-cluster does not match required operator cluster op-namespace/op-cluster")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenOperatorClusterNotFound() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, false, "nats", "nats://nats:4222")

	t.natsClusterResolverMock.mockGetNatsClusterError(t.ctx, ports.NamespacedName{Namespace: "my-namespace", Name: "my-cluster"},
		fmt.Errorf("the root cause"))

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: failed to resolve NATS cluster my-namespace/my-cluster: the root cause")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenAccountClusterNotFound() {
	// Given
	opClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "op-namespace",
		Name:      "op-cluster",
	}
	unitUnderTest := t.newUnitUnderTest(opClusterRef, true, "nats", "nats://nats:4222")

	acClusterRef := &v1alpha1.NatsClusterRef{
		Namespace: "ac-namespace",
		Name:      "ac-cluster",
	}
	t.natsClusterResolverMock.mockGetNatsClusterError(t.ctx, ports.NamespacedName{Namespace: "ac-namespace", Name: "ac-cluster"},
		fmt.Errorf("the root cause"))

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: failed to resolve NATS cluster ac-namespace/ac-cluster: the root cause")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenAccountClusterRefDoesNotContainNamespace() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults()

	acClusterRef := &v1alpha1.NatsClusterRef{
		Name: "ac-cluster",
	}

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, acClusterRef)

	// Then
	require.ErrorContains(t.T(), err, "invalid account cluster reference: namespace is required")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenLegacyLookupAndDefaultNatsURLNotProvided() {
	// Given
	unitUnderTest := t.newUnitUnderTest(nil, false, "nats", "")

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: default NATS URL is not configured for implicit cluster lookup")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_GetClusterConfig_ShouldFail_WhenLegacyLookupAndOperatorNamespaceNotProvided() {
	// Given
	unitUnderTest := t.newUnitUnderTest(nil, false, "", "nats://nats:4222")

	// When
	result, err := unitUnderTest.GetClusterConfig(t.ctx, nil)

	// Then
	require.ErrorContains(t.T(), err, "resolve cluster target: operator namespace is required for implicit cluster lookup")
	require.Nil(t.T(), result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldSucceed_FromConfigMap() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

	t.configMapResolverMock.mockGet(t.ctx, "my-namespace", "my-config",
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
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

	t.configMapResolverMock.mockGet(t.ctx, "config-namespace", "my-config",
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
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

	t.secretClientMock.mockGet(t.ctx, "my-namespace", "my-secret",
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
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

	t.secretClientMock.mockGet(t.ctx, "config-namespace", "my-secret",
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
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

	// When
	result, err := unitUnderTest.resolveNatsURL(t.ctx, &v1alpha1.NatsCluster{})

	// Then
	require.ErrorContains(t.T(), err, "neither url nor urlFrom is set")
	require.Empty(t.T(), result)
}

func (t *ClusterTestSuite) Test_resolveNatsURL_ShouldFail_WhenUnsupportedFromKindProvided() {
	// Given
	unitUnderTest := t.newUnitUnderTestWithDefaults().(*clusterConfigResolver)

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

func (t *ClusterTestSuite) newUnitUnderTestWithDefaults() ClusterConfigResolver {
	return t.newUnitUnderTest(nil, false, "", "")
}

func (t *ClusterTestSuite) newUnitUnderTest(opClusterRef *v1alpha1.NatsClusterRef, opClusterOptional bool, opNamespace string, defaultNatsURL string) ClusterConfigResolver {
	u, err := NewClusterConfigResolver(
		t.natsClusterResolverMock,
		t.secretClientMock,
		t.configMapResolverMock,
		opClusterRef,
		opClusterOptional,
		opNamespace,
		defaultNatsURL,
	)
	if err != nil {
		t.Failf("failed to create cluster config resolver", "error: %v", err)
		return nil
	}

	return u
}

func (t *ClusterTestSuite) generateSecrets() (ports.NatsOperatorSigningKey, ports.NatsUserCreds) {
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
	sauNatsUserCreds, err := ports.NewNatsUserCreds(sauCreds)
	if err != nil {
		t.Failf("failed to create NatsUserCreds from SAU creds", "error: %v", err)
	}

	return opSign, *sauNatsUserCreds
}
