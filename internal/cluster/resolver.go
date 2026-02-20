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

	v1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// APIVersionNauth is the API version for native nauth clusters
	APIVersionNauth = "nauth.io/v1alpha1"
	// APIVersionSynadia is the API version for Synadia Cloud (System CR)
	APIVersionSynadia = "synadia.nauth.io/v1alpha1"
)

// ConfigFetcher retrieves the backend-specific config object from Kubernetes.
// It is called by the Resolver when an Account references a NatsClusterRef
// with the corresponding API version.
type ConfigFetcher func(ctx context.Context, c client.Client, nn types.NamespacedName) (any, error)

// factoryEntry pairs a ProviderFactory with its config fetcher.
type factoryEntry struct {
	factory       ProviderFactory
	configFetcher ConfigFetcher
}

// DefaultResolver implements Resolver by looking up cluster config objects
// and creating providers via registered factories.
type DefaultResolver struct {
	client         client.Client
	entries        map[string]factoryEntry
	nauthNamespace string
}

// NewResolver creates a new DefaultResolver
func NewResolver(c client.Client, nauthNamespace string) *DefaultResolver {
	return &DefaultResolver{
		client:         c,
		entries:        make(map[string]factoryEntry),
		nauthNamespace: nauthNamespace,
	}
}

// RegisterFactory registers a ProviderFactory and its ConfigFetcher for the given API version.
// The configFetcher is called to retrieve the backend-specific config object from K8s before
// passing it to the factory. Pass nil if no config fetch is needed.
// Panics if a factory is already registered for the given apiVersion to prevent silent misconfiguration.
func (r *DefaultResolver) RegisterFactory(apiVersion string, factory ProviderFactory, configFetcher ConfigFetcher) {
	if _, exists := r.entries[apiVersion]; exists {
		panic(fmt.Sprintf("provider factory already registered for API version %q", apiVersion))
	}
	r.entries[apiVersion] = factoryEntry{factory: factory, configFetcher: configFetcher}
}

// ResolveForAccount returns the appropriate Provider for the given account.
// Fetches the referenced cluster config via the registered ConfigFetcher and passes it to the factory.
func (r *DefaultResolver) ResolveForAccount(ctx context.Context, account *v1alpha1.Account) (Provider, error) {
	apiVersion := APIVersionNauth
	var config any

	if account.Spec.NatsClusterRef != nil {
		ref := account.Spec.NatsClusterRef
		if ref.APIVersion != "" {
			apiVersion = ref.APIVersion
		}
		namespace := ref.Namespace
		if namespace == "" {
			namespace = account.GetNamespace()
		}
		nn := types.NamespacedName{Name: ref.Name, Namespace: namespace}

		entry, ok := r.entries[apiVersion]
		if !ok {
			return nil, fmt.Errorf("no provider factory registered for API version %q", apiVersion)
		}
		if entry.configFetcher != nil {
			var err error
			config, err = entry.configFetcher(ctx, r.client, nn)
			if err != nil {
				return nil, err
			}
		}
	}

	entry, ok := r.entries[apiVersion]
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for API version %q", apiVersion)
	}
	return entry.factory.CreateProvider(ctx, config)
}

// RequiresPeriodicSync reports whether the backend for the given account needs
// periodic reconciliation even when the resource spec hasn't changed.
func (r *DefaultResolver) RequiresPeriodicSync(account *v1alpha1.Account) bool {
	apiVersion := APIVersionNauth
	if account.Spec.NatsClusterRef != nil && account.Spec.NatsClusterRef.APIVersion != "" {
		apiVersion = account.Spec.NatsClusterRef.APIVersion
	}
	entry, ok := r.entries[apiVersion]
	if !ok {
		return false
	}
	return entry.factory.RequiresPeriodicSync()
}
