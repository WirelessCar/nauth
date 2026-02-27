package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultNatsClusterRef          = "default-cluster-namespace/default-cluster"
	defaultNatsClusterRefNamespace = "default-cluster-namespace"
	defaultNatsClusterRefName      = "default-cluster"
)

type FactoryTestSuite struct {
	suite.Suite
	ctx           context.Context
	clustersMock  *ClusterGetterMock
	accountsMock  *AccountGetterMock
	secretsMock   *SecretStorerMock
	configMapMock *ConfigMapClientMock
}

func (suite *FactoryTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.clustersMock = NewClusterGetterMock()
	suite.accountsMock = NewAccountGetterMock()
	suite.secretsMock = NewSecretStorerMock()
	suite.configMapMock = NewConfigMapClientMock()
}

func (suite *FactoryTestSuite) TearDownTest() {
	suite.clustersMock.AssertExpectations(suite.T())
	suite.accountsMock.AssertExpectations(suite.T())
	suite.secretsMock.AssertExpectations(suite.T())
	suite.configMapMock.AssertExpectations(suite.T())
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldSucceed_WhenLegacyNoClustersUsed() {
	// Given a manager factory with no default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, "", "controller-namespace", "nats://nats:4222")

	// And an account without a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{},
	}

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should succeed
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), result)

	manager := result.(*Manager)
	require.Nil(suite.T(), manager.natsCluster)
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldSucceed_WhenDefaultClusterRefIsSet() {
	// Given a manager factory with a default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, defaultNatsClusterRef, "controller-namespace", "nats://nats:4222")

	// And an account without a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{},
	}

	cluster := &v1alpha1.NatsCluster{
		Spec: v1alpha1.NatsClusterSpec{
			URL: "nats://cluster:4222",
			OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
				Name: "operator-signing-key",
			},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
				Name: "system-account-user-creds",
			},
		},
	}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, defaultNatsClusterRefNamespace, defaultNatsClusterRefName).
		Return(cluster, nil).
		Once()

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should succeed
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), result)

	manager := result.(*Manager)
	require.Equal(suite.T(), cluster, manager.natsCluster)
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldSucceed_WhenClusterDefinedOnAccount() {
	// Given a manager factory with a default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, defaultNatsClusterRef, "controller-namespace", "nats://nats:4222")

	// And an account with a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{
			NatsClusterRef: &v1alpha1.NatsClusterRef{
				Name:      "account-cluster",
				Namespace: "account-namespace",
			},
		},
	}

	cluster := &v1alpha1.NatsCluster{
		Spec: v1alpha1.NatsClusterSpec{
			URL: "nats://account-cluster:4222",
			OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
				Name: "operator-signing-key",
			},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
				Name: "system-account-user-creds",
			},
		},
	}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, "account-namespace", "account-cluster").
		Return(cluster, nil).
		Once()

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should succeed
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), result)

	manager := result.(*Manager)
	require.Equal(suite.T(), cluster, manager.natsCluster)
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldFail_WhenDefaultClusterNotFound() {
	// Given a manager factory with a default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, defaultNatsClusterRef, "controller-namespace", "nats://nats:4222")

	// And an account without a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{},
	}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, defaultNatsClusterRefNamespace, defaultNatsClusterRefName).
		Return(nil, fmt.Errorf("nats cluster not found")).
		Once()

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should fail with an error indicating the cluster was not found
	require.ErrorContains(suite.T(), err, "nats cluster not found")
	require.Nil(suite.T(), result)
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldFail_WhenClusterURLMissing() {
	// Given a manager factory with a default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, defaultNatsClusterRef, "controller-namespace", "nats://nats:4222")

	// And an account without a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{},
	}

	cluster := &v1alpha1.NatsCluster{
		Spec: v1alpha1.NatsClusterSpec{
			URL: "",
			OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
				Name: "operator-signing-key",
			},
			SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
				Name: "system-account-user-creds",
			},
		},
	}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, defaultNatsClusterRefNamespace, defaultNatsClusterRefName).
		Return(cluster, nil).
		Once()

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should fail with an error indicating the cluster URL is missing
	require.ErrorContains(suite.T(), err, "neither url nor urlFrom is set")
	require.Nil(suite.T(), result)
}

func (suite *FactoryTestSuite) Test_ForAccount_ShouldFail_WhenDefaultClusterRefIsMalformed() {
	// Given a manager factory with an invalid default NATS cluster reference
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, "invalid-ref/", "controller-namespace", "nats://nats:4222")

	// And an account without a cluster reference
	acct := &v1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "account-name",
			Namespace: "account-namespace",
		},
		Spec: v1alpha1.AccountSpec{},
	}

	// When creating an account manager for the account
	result, err := unitUnderTest.ForAccount(suite.ctx, acct)

	// Then the operation should fail with an error indicating the default NATS cluster reference is malformed
	require.ErrorContains(suite.T(), err, "parse default NATS cluster reference")
	require.Nil(suite.T(), result)
}

func (suite *FactoryTestSuite) Test_resolveNatsClusterForAccount_ShouldUtilizeSuppliedDefaultNamespace() {
	// Given
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, "", "controller-namespace", "nats://nats:4222")
	clusterRef := &v1alpha1.NatsClusterRef{Name: "cluster-name"}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, "supplied-default-namespace", clusterRef.Name).
		Return(&v1alpha1.NatsCluster{}, nil).
		Once()

	// When
	result, err := unitUnderTest.resolveNatsClusterForAccount(suite.ctx, clusterRef, "supplied-default-namespace")

	// Then
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), result)
}

func (suite *FactoryTestSuite) Test_resolveNatsClusterForAccount_ShouldPrioritizeClusterNamespace() {
	// Given
	unitUnderTest := NewManagerFactory(suite.clustersMock, suite.accountsMock, suite.secretsMock, suite.configMapMock, "", "controller-namespace", "nats://nats:4222")
	clusterRef := &v1alpha1.NatsClusterRef{Name: "cluster-name", Namespace: "cluster-namespace"}
	suite.clustersMock.On("GetNatsCluster", suite.ctx, "cluster-namespace", clusterRef.Name).
		Return(&v1alpha1.NatsCluster{}, nil).
		Once()

	// When
	result, err := unitUnderTest.resolveNatsClusterForAccount(suite.ctx, clusterRef, "supplied-default-namespace")

	// Then
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), result)
}

func TestFactory_TestSuite(t *testing.T) {
	suite.Run(t, new(FactoryTestSuite))
}

func TestFactory_parseNatsClusterRef_ShouldSucceed(t *testing.T) {
	testCases := []struct {
		name   string
		value  string
		expect *v1alpha1.NatsClusterRef
	}{
		{
			name:  "namespace and name",
			value: "my-namespace/my-cluster",
			expect: &v1alpha1.NatsClusterRef{
				Name:      "my-cluster",
				Namespace: "my-namespace",
			},
		},
		{
			name:  "name as dns subdomain",
			value: "my-namespace/my.cluster",
			expect: &v1alpha1.NatsClusterRef{
				Name:      "my.cluster",
				Namespace: "my-namespace",
			},
		},
		{
			name:  "namespace and name with only numbers",
			value: "0/1",
			expect: &v1alpha1.NatsClusterRef{
				Name:      "1",
				Namespace: "0",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseNatsClusterRef(testCase.value)

			require.NoError(t, err)
			require.Equal(t, testCase.expect, result)
		})
	}
}

func TestFactory_parseNatsClusterRef_ShouldFail(t *testing.T) {
	testCases := []struct {
		name  string
		value string
	}{
		{
			name:  "empty string/undefined",
			value: "",
		},
		{
			name:  "no separator",
			value: "my-cluster",
		},
		{
			name:  "no namespace",
			value: "/my-cluster",
		},
		{
			name:  "no name",
			value: "my-cluster/",
		},
		{
			name:  "invalid namespace char",
			value: "my.namespace/my-cluster",
		},
		{
			name:  "invalid name char",
			value: "my-namespace/my_cluster",
		},
		{
			name:  "too many segments",
			value: "ns1/ns2/cluster",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseNatsClusterRef(testCase.value)

			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
