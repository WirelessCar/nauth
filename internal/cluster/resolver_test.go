package cluster

import (
	"context"
	"testing"

	v1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolver_parseClusterReference_ShouldSucceed(t *testing.T) {

	testCases := []struct {
		name   string
		value  string
		expect types.NamespacedName
	}{
		{
			name:  "name only",
			value: "my-cluster",
			expect: types.NamespacedName{
				Name:      "my-cluster",
				Namespace: "default",
			},
		},
		{
			name:  "namespace and name",
			value: "my-namespace/my-cluster",
			expect: types.NamespacedName{
				Name:      "my-cluster",
				Namespace: "my-namespace",
			},
		},
		{
			name:  "namespace and name with only numbers",
			value: "0/1",
			expect: types.NamespacedName{
				Name:      "1",
				Namespace: "0",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseClusterReference(testCase.value, "default")

			require.NoError(t, err)
			require.Equal(t, testCase.expect, result)
		})
	}
}

func TestResolver_parseClusterReference_ShouldFail(t *testing.T) {

	testCases := []struct {
		name  string
		value string
	}{
		{
			name:  "empty string/undefined",
			value: "",
		},
		{
			name:  "separator without namespace",
			value: "/my-cluster",
		},
		{
			name:  "only namespace",
			value: "my-namespace/",
		},
		{
			name:  "invalid namespace char",
			value: "my.namespace/my-cluster",
		},
		{
			name:  "invalid name char",
			value: "my-namespace/my.cluster",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := parseClusterReference(testCase.value, "default")

			require.Error(t, err)
			require.ErrorContains(t, err, "invalid Cluster Reference pattern")
			require.Equal(t, types.NamespacedName{}, result)
		})
	}
}

func TestResolver_ResolveForAccount_NoCluster(t *testing.T) {
	t.Setenv("DEFAULT_CLUSTER_REF", "")

	resolver, factory := createResolverWithFactory(t)
	account := &v1alpha1.Account{}
	account.SetName("my-account")
	account.SetNamespace("tenant-a")

	provider, err := resolver.ResolveForAccount(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Nil(t, factory.cluster)
}

func TestResolver_ResolveForAccount_OperatorDefaultClusterRef(t *testing.T) {
	testCases := []struct {
		name              string
		defaultClusterRef string
		accountNamespace  string
		clusterNamespace  string
		clusterName       string
	}{
		{
			name:              "name only defaults to account namespace",
			defaultClusterRef: "default-cluster",
			accountNamespace:  "tenant-a",
			clusterNamespace:  "tenant-a",
			clusterName:       "default-cluster",
		},
		{
			name:              "namespace and name",
			defaultClusterRef: "shared/default-cluster",
			accountNamespace:  "tenant-a",
			clusterNamespace:  "shared",
			clusterName:       "default-cluster",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("DEFAULT_CLUSTER_REF", testCase.defaultClusterRef)

			cluster := &v1alpha1.NatsCluster{}
			cluster.SetName(testCase.clusterName)
			cluster.SetNamespace(testCase.clusterNamespace)

			resolver, factory := createResolverWithFactory(t, cluster)
			account := &v1alpha1.Account{}
			account.SetName("my-account")
			account.SetNamespace(testCase.accountNamespace)

			provider, err := resolver.ResolveForAccount(context.Background(), account)
			require.NoError(t, err)
			require.NotNil(t, provider)
			require.NotNil(t, factory.cluster)
			require.Equal(t, testCase.clusterName, factory.cluster.GetName())
			require.Equal(t, testCase.clusterNamespace, factory.cluster.GetNamespace())
		})
	}
}

func TestResolver_ResolveForAccount_AccountClusterRef(t *testing.T) {
	t.Setenv("DEFAULT_CLUSTER_REF", "")

	cluster := &v1alpha1.NatsCluster{}
	cluster.SetName("account-cluster")
	cluster.SetNamespace("clusters")

	resolver, factory := createResolverWithFactory(t, cluster)
	account := &v1alpha1.Account{}
	account.SetName("my-account")
	account.SetNamespace("tenant-a")
	account.Spec.NatsClusterRef = &v1alpha1.NatsClusterRef{
		Kind:      KindNatsCluster,
		Name:      "account-cluster",
		Namespace: "clusters",
	}

	provider, err := resolver.ResolveForAccount(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.NotNil(t, factory.cluster)
	require.Equal(t, "account-cluster", factory.cluster.GetName())
	require.Equal(t, "clusters", factory.cluster.GetNamespace())
}

type resolverProviderStub struct{}

func (s *resolverProviderStub) CreateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}

func (s *resolverProviderStub) UpdateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}

func (s *resolverProviderStub) ImportAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}

func (s *resolverProviderStub) DeleteAccount(ctx context.Context, account *v1alpha1.Account) error {
	return nil
}

func (s *resolverProviderStub) CreateOrUpdateUser(ctx context.Context, user *v1alpha1.User) (*UserResult, error) {
	return nil, nil
}

func (s *resolverProviderStub) DeleteUser(ctx context.Context, user *v1alpha1.User) error {
	return nil
}

type resolverProviderFactoryStub struct {
	provider Provider
	cluster  *v1alpha1.NatsCluster
}

func (f *resolverProviderFactoryStub) CreateProvider(ctx context.Context, cluster *v1alpha1.NatsCluster) (Provider, error) {
	f.cluster = cluster
	return f.provider, nil
}

func createResolverWithFactory(t *testing.T, objects ...client.Object) (*DefaultResolver, *resolverProviderFactoryStub) {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(s))

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objects...).
		Build()

	resolver := NewResolver(c, "nauth-system")
	factory := &resolverProviderFactoryStub{
		provider: &resolverProviderStub{},
	}
	resolver.RegisterFactory(KindNatsCluster, factory)
	return resolver, factory
}
