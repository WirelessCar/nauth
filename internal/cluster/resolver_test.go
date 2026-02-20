/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"context"
	"fmt"
	"testing"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultResolver_ResolveForAccount_nil_ref_uses_nauth_factory(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))

	nauthFactory := &mockProviderFactory{name: "nauth"}
	synadiaFactory := &mockProviderFactory{name: "synadia"}

	r := NewResolver(nil, "nauth-ns")
	r.RegisterFactory(APIVersionNauth, nauthFactory, nil)
	r.RegisterFactory(APIVersionSynadia, synadiaFactory, nil)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec:       nauthv1alpha1.AccountSpec{}, // NatsClusterRef is nil
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, "nauth", provider.(*mockProvider).name)
	assert.Nil(t, nauthFactory.lastConfig, "legacy nil ref should pass nil config")
	assert.Nil(t, synadiaFactory.lastConfig)
}

func TestDefaultResolver_ResolveForAccount_nauth_ref_fetches_nats_cluster(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))

	cluster := &nauthv1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: "default"},
		Spec: nauthv1alpha1.NatsClusterSpec{
			URL:                             "nats://nats.example.com:4222",
			OperatorSigningKeySecretRef:     nauthv1alpha1.SecretKeyReference{Name: "op", Key: "key"},
			SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{Name: "sys", Key: "creds"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	nauthFactory := &mockProviderFactory{name: "nauth"}
	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionNauth, nauthFactory, nauthConfigFetcher)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: APIVersionNauth,
				Kind:       "NatsCluster",
				Name:       "my-cluster",
				Namespace:  "default",
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, "nauth", provider.(*mockProvider).name)
	require.NotNil(t, nauthFactory.lastConfig)
	_, ok := nauthFactory.lastConfig.(*nauthv1alpha1.NatsCluster)
	assert.True(t, ok, "nauth factory should receive *NatsCluster")
}

func TestDefaultResolver_ResolveForAccount_synadia_ref_fetches_system(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))

	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "synadia"},
		Spec: synadiav1alpha1.SystemSpec{
			TeamID:                  "team-1",
			SystemSelector:          synadiav1alpha1.SystemSelector{Name: "NGS"},
			APICredentialsSecretRef: synadiav1alpha1.SecretKeyReference{Name: "token", Key: "token"},
			APIEndpoint:             "https://cloud.synadia.com",
		},
		Status: synadiav1alpha1.SystemStatus{SystemID: "sys-123"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sys).Build()

	synadiaFactory := &mockProviderFactory{name: "synadia"}
	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionSynadia, synadiaFactory, synadiaConfigFetcher)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: "synadia.nauth.io/v1alpha1",
				Kind:       "System",
				Name:       "ngs",
				Namespace:  "synadia",
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, "synadia", provider.(*mockProvider).name)
	require.NotNil(t, synadiaFactory.lastConfig)
	_, ok := synadiaFactory.lastConfig.(*synadiav1alpha1.System)
	assert.True(t, ok, "synadia factory should receive *System")
}

func TestDefaultResolver_ResolveForAccount_ref_namespace_defaults_to_account_namespace(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))

	cluster := &nauthv1alpha1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "local", Namespace: "my-ns"},
		Spec: nauthv1alpha1.NatsClusterSpec{
			URL:                             "nats://local:4222",
			OperatorSigningKeySecretRef:     nauthv1alpha1.SecretKeyReference{Name: "op", Key: "key"},
			SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{Name: "sys", Key: "creds"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	nauthFactory := &mockProviderFactory{name: "nauth"}
	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionNauth, nauthFactory, nauthConfigFetcher)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "my-ns"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: APIVersionNauth,
				Kind:       "NatsCluster",
				Name:       "local",
				// Namespace empty -> defaults to account namespace my-ns
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.Equal(t, "nauth", provider.(*mockProvider).name)
}

func TestDefaultResolver_ResolveForAccount_unsupported_api_version_returns_error(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionNauth, &mockProviderFactory{name: "nauth"}, nil)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: "unknown.io/v1",
				Kind:       "Something",
				Name:       "x",
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "no provider factory registered")
	assert.Contains(t, err.Error(), "unknown.io/v1")
}

func TestDefaultResolver_ResolveForAccount_no_factory_registered_returns_error(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Resolver with no factories
	r := NewResolver(k8sClient, "nauth-ns")

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec:       nauthv1alpha1.AccountSpec{}, // nil ref -> APIVersionNauth
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "no provider factory registered")
}

func TestDefaultResolver_ResolveForAccount_nats_cluster_not_found_returns_error(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build() // no NatsCluster

	nauthFactory := &mockProviderFactory{name: "nauth"}
	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionNauth, nauthFactory, nauthConfigFetcher)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: APIVersionNauth,
				Kind:       "NatsCluster",
				Name:       "missing",
				Namespace:  "default",
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "failed to get NatsCluster")
}

func TestDefaultResolver_ResolveForAccount_system_not_found_returns_error(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build() // no System

	synadiaFactory := &mockProviderFactory{name: "synadia"}
	r := NewResolver(k8sClient, "nauth-ns")
	r.RegisterFactory(APIVersionSynadia, synadiaFactory, synadiaConfigFetcher)

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "acc", Namespace: "default"},
		Spec: nauthv1alpha1.AccountSpec{
			NatsClusterRef: &nauthv1alpha1.NatsClusterRef{
				APIVersion: APIVersionSynadia,
				Kind:       "System",
				Name:       "missing",
				Namespace:  "default",
			},
		},
	}

	provider, err := r.ResolveForAccount(ctx, account)
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "failed to get System")
}

func TestDefaultResolver_RegisterFactory_panics_on_duplicate(t *testing.T) {
	r := NewResolver(nil, "nauth-ns")
	f := &mockProviderFactory{name: "nauth"}
	r.RegisterFactory(APIVersionNauth, f, nil)
	require.Panics(t, func() {
		r.RegisterFactory(APIVersionNauth, &mockProviderFactory{name: "other"}, nil)
	})
}

// --- test helpers ---

func nauthConfigFetcher(ctx context.Context, c client.Client, nn types.NamespacedName) (any, error) {
	obj := &nauthv1alpha1.NatsCluster{}
	if err := c.Get(ctx, nn, obj); err != nil {
		return nil, fmt.Errorf("failed to get NatsCluster %s/%s: %w", nn.Namespace, nn.Name, err)
	}
	return obj, nil
}

func synadiaConfigFetcher(ctx context.Context, c client.Client, nn types.NamespacedName) (any, error) {
	obj := &synadiav1alpha1.System{}
	if err := c.Get(ctx, nn, obj); err != nil {
		return nil, fmt.Errorf("failed to get System %s/%s: %w", nn.Namespace, nn.Name, err)
	}
	return obj, nil
}

// mockProviderFactory returns a mock provider and records the config passed to CreateProvider.
type mockProviderFactory struct {
	name       string
	lastConfig any
}

func (m *mockProviderFactory) CreateProvider(_ context.Context, config any) (Provider, error) {
	m.lastConfig = config
	return &mockProvider{name: m.name}, nil
}

func (m *mockProviderFactory) RequiresPeriodicSync() bool { return false }

type mockProvider struct {
	name string
}

func (m *mockProvider) CreateAccount(_ context.Context, _ *nauthv1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}
func (m *mockProvider) UpdateAccount(_ context.Context, _ *nauthv1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}
func (m *mockProvider) ImportAccount(_ context.Context, _ *nauthv1alpha1.Account) (*AccountResult, error) {
	return nil, nil
}
func (m *mockProvider) DeleteAccount(_ context.Context, _ *nauthv1alpha1.Account) error {
	return nil
}
func (m *mockProvider) CreateOrUpdateUser(_ context.Context, _ *nauthv1alpha1.User) (*UserResult, error) {
	return nil, nil
}
func (m *mockProvider) DeleteUser(_ context.Context, _ *nauthv1alpha1.User) error {
	return nil
}

var _ Provider = (*mockProvider)(nil)
var _ ProviderFactory = (*mockProviderFactory)(nil)
