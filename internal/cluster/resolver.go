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
	// APIVersionNauth is the default API version for native nauth clusters
	APIVersionNauth = "nauth.io/v1alpha1"
)

// ProviderFactory creates Provider instances for a specific cluster type
type ProviderFactory interface {
	// CreateProvider creates a provider for the given NatsCluster
	// If cluster is nil, creates a provider using legacy label-based configuration
	CreateProvider(ctx context.Context, cluster *v1alpha1.NatsCluster) (Provider, error)
}

// DefaultResolver implements Resolver by looking up NatsCluster CRDs and creating providers
type DefaultResolver struct {
	client         client.Client
	factories      map[string]ProviderFactory
	nauthNamespace string
}

// NewResolver creates a new DefaultResolver
func NewResolver(c client.Client, nauthNamespace string) *DefaultResolver {
	return &DefaultResolver{
		client:         c,
		factories:      make(map[string]ProviderFactory),
		nauthNamespace: nauthNamespace,
	}
}

// RegisterFactory registers a ProviderFactory for the given API version.
// Panics if a factory is already registered for the given apiVersion to prevent silent misconfiguration.
func (r *DefaultResolver) RegisterFactory(apiVersion string, factory ProviderFactory) {
	if _, exists := r.factories[apiVersion]; exists {
		panic(fmt.Sprintf("provider factory already registered for API version %q", apiVersion))
	}
	r.factories[apiVersion] = factory
}

// ResolveForAccount returns the appropriate Provider for the given account
func (r *DefaultResolver) ResolveForAccount(ctx context.Context, account *v1alpha1.Account) (Provider, error) {
	// Determine API version - default to nauth.io/v1alpha1
	apiVersion := APIVersionNauth
	var cluster *v1alpha1.NatsCluster

	if account.Spec.NatsClusterRef != nil {
		clusterRef := account.Spec.NatsClusterRef
		if clusterRef.APIVersion != "" {
			apiVersion = clusterRef.APIVersion
		}

		// Fetch the NatsCluster CRD
		namespace := clusterRef.Namespace
		if namespace == "" {
			namespace = account.GetNamespace()
		}

		cluster = &v1alpha1.NatsCluster{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      clusterRef.Name,
			Namespace: namespace,
		}, cluster); err != nil {
			return nil, fmt.Errorf("failed to get NatsCluster %s/%s: %w", namespace, clusterRef.Name, err)
		}
	}

	// Get the factory for this API version
	factory, ok := r.factories[apiVersion]
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for API version %q", apiVersion)
	}

	// Create the provider
	return factory.CreateProvider(ctx, cluster)
}
