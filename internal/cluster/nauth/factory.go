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
	"fmt"
	"os"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/cluster/nauth/account"
	"github.com/WirelessCar/nauth/internal/cluster/nauth/user"
	natsc "github.com/WirelessCar/nauth/internal/nats"
)

// Factory creates nauth Provider instances
type Factory struct {
	accounts        account.AccountGetter
	secretClient    account.SecretClient
	configmapClient account.ConfigMapClient
	nauthNamespace  string
}

// NewFactory creates a new nauth Factory
func NewFactory(
	accounts account.AccountGetter,
	secretClient account.SecretClient,
	configmapClient account.ConfigMapClient,
	nauthNamespace string,
) *Factory {
	return &Factory{
		accounts:        accounts,
		secretClient:    secretClient,
		configmapClient: configmapClient,
		nauthNamespace:  nauthNamespace,
	}
}

// CreateProvider creates a new Provider configured for the given NatsCluster
// If cluster is nil, creates a provider using legacy label-based configuration (NATS_URL env var)
func (f *Factory) CreateProvider(ctx context.Context, nc *nauthv1alpha1.NatsCluster) (cluster.Provider, error) {
	var natsURL string
	if nc != nil {
		var err error
		natsURL, err = f.resolveNatsURL(ctx, nc)
		if err != nil {
			return nil, fmt.Errorf("resolve NATS URL: %w", err)
		}
	} else {
		natsURL = os.Getenv("NATS_URL") // Legacy: use env var when no NatsClusterRef
	}

	// Create NATS client - with or without NatsCluster configuration
	var natsClient account.NatsClient
	if nc != nil {
		// Build SecretRef from NatsCluster CR for the NATS client
		credsRef := &natsc.SecretRef{
			Namespace: nc.GetNamespace(),
			Name:      nc.Spec.SystemAccountUserCredsSecretRef.Name,
			Key:       nc.Spec.SystemAccountUserCredsSecretRef.Key,
		}
		natsClient = natsc.NewClientWithSecretRef(natsURL, f.secretClient, credsRef)
	} else {
		natsClient = natsc.NewClient(natsURL, f.secretClient)
	}

	// Build options for the account manager
	opts := []func(*account.Manager){
		account.WithNamespace(f.nauthNamespace),
	}

	// If NatsCluster is provided, add it to the options
	if nc != nil {
		opts = append(opts, account.WithNatsCluster(nc))
	}

	// Create managers with the appropriate configuration
	accountManager := account.NewManager(f.accounts, natsClient, f.secretClient, opts...)
	userManager := user.NewManager(f.accounts, f.secretClient)

	return NewProvider(accountManager, userManager), nil
}

// resolveNatsURL returns the NATS URL from NatsCluster spec (url or urlFrom).
func (f *Factory) resolveNatsURL(ctx context.Context, nc *nauthv1alpha1.NatsCluster) (string, error) {
	if nc.Spec.URL != "" {
		return nc.Spec.URL, nil
	}
	if nc.Spec.URLFrom == nil {
		return "", fmt.Errorf("neither url nor urlFrom is set")
	}
	ref := nc.Spec.URLFrom
	namespace := ref.Namespace
	if namespace == "" {
		namespace = nc.GetNamespace()
	}
	switch ref.Kind {
	case nauthv1alpha1.URLFromKindConfigMap:
		data, err := f.configmapClient.Get(ctx, namespace, ref.Name)
		if err != nil {
			return "", fmt.Errorf("get ConfigMap %s/%s: %w", namespace, ref.Name, err)
		}
		if v, ok := data[ref.Key]; ok {
			return v, nil
		}
		return "", fmt.Errorf("configMap %s/%s has no key %q", namespace, ref.Name, ref.Key)
	case nauthv1alpha1.URLFromKindSecret:
		data, err := f.secretClient.Get(ctx, namespace, ref.Name)
		if err != nil {
			return "", fmt.Errorf("get Secret %s/%s: %w", namespace, ref.Name, err)
		}
		if v, ok := data[ref.Key]; ok {
			return v, nil
		}
		return "", fmt.Errorf("secret %s/%s has no key %q", namespace, ref.Name, ref.Key)
	default:
		return "", fmt.Errorf("urlFrom.kind must be ConfigMap or Secret, got %q", ref.Kind)
	}
}

// Ensure Factory implements cluster.ProviderFactory
var _ cluster.ProviderFactory = (*Factory)(nil)
