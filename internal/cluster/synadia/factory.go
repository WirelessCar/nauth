/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS FOR A PARTICULAR PURPOSE.
See the License for the specific language governing permissions and
limitations under the License.
*/

package synadia

import (
	"context"
	"fmt"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TokenReader reads a secret and returns the API token for the given System.
type TokenReader interface {
	Get(ctx context.Context, namespace, name string) (map[string]string, error)
}

// SecretClient is used for both reading API credentials and applying user credential secrets.
// It is the union of TokenReader and SecretApplier.
type SecretClient interface {
	TokenReader
	SecretApplier
}

// Factory creates Synadia Provider instances for System CRs.
type Factory struct {
	k8sClient    client.Client
	secretClient SecretClient
}

// NewFactory creates a new Synadia factory.
func NewFactory(k8sClient client.Client, secretClient SecretClient) *Factory {
	return &Factory{
		k8sClient:    k8sClient,
		secretClient: secretClient,
	}
}

// CreateProvider creates a Provider for the given config. config must be *synadiav1alpha1.System.
func (f *Factory) CreateProvider(_ context.Context, config any) (cluster.Provider, error) {
	sys, ok := config.(*synadiav1alpha1.System)
	if !ok {
		return nil, fmt.Errorf("synadia factory expected *System, got %T", config)
	}
	if sys.Spec.APIEndpoint == "" {
		return nil, fmt.Errorf("system %s/%s has no apiEndpoint", sys.GetNamespace(), sys.GetName())
	}
	ref := sys.Spec.APICredentialsSecretRef
	if ref.Name == "" {
		return nil, fmt.Errorf("system %s/%s has no apiCredentialsSecretRef.name", sys.GetNamespace(), sys.GetName())
	}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = sys.GetNamespace()
	}
	key := ref.Key
	if key == "" {
		key = "token"
	}
	getToken := func(ctx context.Context) (string, error) {
		data, err := f.secretClient.Get(ctx, namespace, ref.Name)
		if err != nil {
			return "", fmt.Errorf("get API credentials secret %s/%s: %w", namespace, ref.Name, err)
		}
		token, ok := data[key]
		if !ok {
			return "", fmt.Errorf("secret %s/%s has no key %q", namespace, ref.Name, key)
		}
		return token, nil
	}
	apiClient := NewClient(sys.Spec.APIEndpoint, getToken)
	provider := NewProvider(apiClient, sys, f.k8sClient, f.secretClient)
	return provider, nil
}

// RequiresPeriodicSync returns true — Synadia backends need periodic reconciliation
// to keep the local state in sync with the remote API.
func (f *Factory) RequiresPeriodicSync() bool { return true }

// FetchConfig retrieves a System object from Kubernetes for the Resolver.
func FetchConfig(ctx context.Context, c client.Client, nn types.NamespacedName) (any, error) {
	sys := &synadiav1alpha1.System{}
	if err := c.Get(ctx, nn, sys); err != nil {
		return nil, fmt.Errorf("failed to get System %s/%s: %w", nn.Namespace, nn.Name, err)
	}
	return sys, nil
}

// Ensure Factory implements cluster.ProviderFactory.
var _ cluster.ProviderFactory = (*Factory)(nil)
