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

package system

import (
	"context"
	"fmt"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// APIGroupNauth is the API group for native nauth systems
	APIGroupNauth = "nauth.io"
)

// ProviderFactory creates Provider instances for a specific system type
type ProviderFactory interface {
	// CreateProvider creates a provider for the given System
	// If system is nil, creates a provider using legacy label-based configuration
	CreateProvider(system *natsv1alpha1.System) (Provider, error)
}

// DefaultResolver implements Resolver by looking up System CRDs and creating providers
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

// RegisterFactory registers a ProviderFactory for the given API group
func (r *DefaultResolver) RegisterFactory(apiGroup string, factory ProviderFactory) {
	r.factories[apiGroup] = factory
}

// ResolveForAccount returns the appropriate Provider for the given account
func (r *DefaultResolver) ResolveForAccount(ctx context.Context, account *natsv1alpha1.Account) (Provider, error) {
	// Determine API group
	apiGroup := APIGroupNauth
	var system *natsv1alpha1.System

	if account.Spec.SystemRef != nil {
		systemRef := account.Spec.SystemRef
		if systemRef.APIGroup != "" {
			apiGroup = systemRef.APIGroup
		}

		// Fetch the System CRD
		namespace := systemRef.Namespace
		if namespace == "" {
			namespace = account.GetNamespace()
		}

		system = &natsv1alpha1.System{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      systemRef.Name,
			Namespace: namespace,
		}, system); err != nil {
			return nil, fmt.Errorf("failed to get System %s/%s: %w", namespace, systemRef.Name, err)
		}
	}

	// Get the factory for this API group
	factory, ok := r.factories[apiGroup]
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for API group %q", apiGroup)
	}

	// Create the provider
	return factory.CreateProvider(system)
}
