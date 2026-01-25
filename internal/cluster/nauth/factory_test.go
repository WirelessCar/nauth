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

package nauth

import (
	"context"
	"errors"
	"os"
	"testing"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// secretClientMock implements account.SecretClient for factory tests.
type secretClientMock struct {
	mock.Mock
}

func (s *secretClientMock) Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	args := s.Called(ctx, owner, meta, valueMap)
	return args.Error(0)
}

func (s *secretClientMock) Get(ctx context.Context, namespace, name string) (map[string]string, error) {
	args := s.Called(ctx, namespace, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

func (s *secretClientMock) GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error) {
	args := s.Called(ctx, namespace, labels)
	return args.Get(0).(*v1.SecretList), args.Error(1)
}

func (s *secretClientMock) Delete(ctx context.Context, namespace, name string) error {
	args := s.Called(ctx, namespace, name)
	return args.Error(0)
}

func (s *secretClientMock) DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error {
	args := s.Called(ctx, namespace, labels)
	return args.Error(0)
}

func (s *secretClientMock) Label(ctx context.Context, namespace, name string, labels map[string]string) error {
	args := s.Called(ctx, namespace, name, labels)
	return args.Error(0)
}

// accountGetterMock implements account.AccountGetter for factory tests.
type accountGetterMock struct {
	mock.Mock
}

func (a *accountGetterMock) Get(ctx context.Context, accountRefName, namespace string) (*nauthv1alpha1.Account, error) {
	args := a.Called(ctx, accountRefName, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*nauthv1alpha1.Account), args.Error(1)
}

// configMapClientMock implements account.ConfigMapClient for testing.
type configMapClientMock struct {
	getFunc func(ctx context.Context, namespace, name string) (map[string]string, error)
}

func (c *configMapClientMock) Get(ctx context.Context, namespace, name string) (map[string]string, error) {
	if c.getFunc != nil {
		return c.getFunc(ctx, namespace, name)
	}
	return nil, errors.New("not implemented")
}

func TestFactory_ResolveNatsURL(t *testing.T) {
	ctx := context.Background()
	nauthNamespace := "nauth-system"

	t.Run("returns_url_when_spec_url_is_set", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URL: "nats://nats.example.com:4222",
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("returns_error_when_neither_url_nor_urlFrom_set", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "resolve NATS URL")
		assert.Contains(t, err.Error(), "neither url nor urlFrom is set")
	})

	t.Run("resolves_url_from_configmap", func(t *testing.T) {
		cmClient := &configMapClientMock{
			getFunc: func(ctx context.Context, namespace, name string) (map[string]string, error) {
				assert.Equal(t, "test-ns", namespace)
				assert.Equal(t, "nats-config", name)
				return map[string]string{"url": "nats://configmap.example.com:4222"}, nil
			},
		}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindConfigMap,
					Name:      "nats-config",
					Namespace: "test-ns",
					Key:       "url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("resolves_url_from_configmap_with_default_namespace", func(t *testing.T) {
		cmClient := &configMapClientMock{
			getFunc: func(ctx context.Context, namespace, name string) (map[string]string, error) {
				assert.Equal(t, "test-ns", namespace, "should default to NatsCluster namespace")
				assert.Equal(t, "nats-config", name)
				return map[string]string{"url": "nats://default-ns.example.com:4222"}, nil
			},
		}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind: nauthv1alpha1.URLFromKindConfigMap, // Namespace empty -> defaults to cluster ns
					Name: "nats-config",
					Key:  "url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("resolves_url_from_secret", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindSecret,
					Name:      "nats-url-secret",
					Namespace: "other-ns",
					Key:       "nats-url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		secretClient.On("Get", ctx, "other-ns", "nats-url-secret").Return(map[string]string{"nats-url": "nats://secret.example.com:4222"}, nil)

		provider, err := f.CreateProvider(ctx, nc)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		secretClient.AssertExpectations(t)
	})

	t.Run("returns_error_when_urlFrom_kind_is_invalid", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind: "InvalidKind",
					Name: "something",
					Key:  "key",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "urlFrom.kind must be ConfigMap or Secret")
		assert.Contains(t, err.Error(), "InvalidKind")
	})

	t.Run("returns_error_when_configmap_get_fails", func(t *testing.T) {
		cmClient := &configMapClientMock{
			getFunc: func(ctx context.Context, namespace, name string) (map[string]string, error) {
				return nil, errors.New("configmap not found")
			},
		}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindConfigMap,
					Name:      "missing-config",
					Namespace: "test-ns",
					Key:       "url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "get ConfigMap test-ns/missing-config")
		assert.Contains(t, err.Error(), "configmap not found")
	})

	t.Run("returns_error_when_configmap_missing_key", func(t *testing.T) {
		cmClient := &configMapClientMock{
			getFunc: func(ctx context.Context, namespace, name string) (map[string]string, error) {
				return map[string]string{"other-key": "value"}, nil
			},
		}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindConfigMap,
					Name:      "nats-config",
					Namespace: "test-ns",
					Key:       "url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "configMap test-ns/nats-config has no key \"url\"")
	})

	t.Run("returns_error_when_secret_get_fails", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindSecret,
					Name:      "missing-secret",
					Namespace: "test-ns",
					Key:       "url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		secretClient.On("Get", ctx, "test-ns", "missing-secret").Return(map[string]string(nil), errors.New("secret not found"))

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "get Secret test-ns/missing-secret")
		assert.Contains(t, err.Error(), "secret not found")
	})

	t.Run("returns_error_when_secret_missing_key", func(t *testing.T) {
		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, nauthNamespace)

		nc := &nauthv1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "my-cluster"},
			Spec: nauthv1alpha1.NatsClusterSpec{
				URLFrom: &nauthv1alpha1.URLFromReference{
					Kind:      nauthv1alpha1.URLFromKindSecret,
					Name:      "nats-url-secret",
					Namespace: "test-ns",
					Key:       "nats-url",
				},
				OperatorSigningKeySecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "op-key",
					Key:  "key",
				},
				SystemAccountUserCredsSecretRef: nauthv1alpha1.SecretKeyReference{
					Name: "sys-creds",
					Key:  "creds",
				},
			},
		}

		secretClient.On("Get", ctx, "test-ns", "nats-url-secret").Return(map[string]string{"other": "value"}, nil)

		provider, err := f.CreateProvider(ctx, nc)
		require.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "secret test-ns/nats-url-secret has no key \"nats-url\"")
	})
}

func TestFactory_CreateProvider_LegacyEnv(t *testing.T) {
	ctx := context.Background()

	t.Run("uses_NATS_URL_env_when_cluster_is_nil", func(t *testing.T) {
		const testURL = "nats://legacy.example.com:4222"
		key := "NATS_URL"
		prev := os.Getenv(key)
		defer func() { _ = os.Setenv(key, prev) }()
		_ = os.Setenv(key, testURL)

		cmClient := &configMapClientMock{}
		secretClient := &secretClientMock{}
		accounts := &accountGetterMock{}
		f := NewFactory(accounts, secretClient, cmClient, "nauth-system")

		provider, err := f.CreateProvider(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})
}
