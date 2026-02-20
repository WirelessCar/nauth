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
	"os"
	"regexp"

	v1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//goland:noinspection GoNameStartsWithPackageName
const (
	// APIVersionNauth is the default API version for native nauth clusters
	APIVersionNauth = "nauth.io/v1alpha1"
	// KindNatsCluster is the default kind for native nauth clusters
	KindNatsCluster = "NatsCluster"
)

// clusterRefRegexp is a regex to parse cluster references in the format [namespace/]name
var clusterRefRegexp = regexp.MustCompile(
	"^((?P<namespace>[a-z0-9]([-a-z0-9]*[a-z0-9])?)/)?(?P<name>[a-z0-9]([-a-z0-9]*[a-z0-9])?)$")

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
func (r *DefaultResolver) RegisterFactory(clusterKind string, factory ProviderFactory) {
	if _, exists := r.factories[clusterKind]; exists {
		panic(fmt.Sprintf("provider factory already registered for Cluster Kind %q", clusterKind))
	}
	r.factories[clusterKind] = factory
}

// ResolveForAccount returns the appropriate Provider for the given account
func (r *DefaultResolver) ResolveForAccount(ctx context.Context, account *v1alpha1.Account) (Provider, error) {
	// Default to native nauth cluster if no reference is provided
	apiVersion := APIVersionNauth
	clusterKind := KindNatsCluster
	defaultClusterRef := os.Getenv("DEFAULT_CLUSTER_REF") // Optional default cluster reference in the format [namespace/]name
	var cluster *v1alpha1.NatsCluster

	if account.Spec.NatsClusterRef != nil {
		clusterRef := account.Spec.NatsClusterRef
		if clusterRef.APIVersion != "" {
			apiVersion = clusterRef.APIVersion
		}
		if clusterRef.Kind != "" {
			clusterKind = clusterRef.Kind
		}
		resource := types.NamespacedName{
			Name: clusterRef.Name,
		}
		if clusterRef.Namespace != "" {
			resource.Namespace = clusterRef.Namespace
		} else {
			resource.Namespace = account.GetNamespace()
		}
		var err error
		cluster, err = r.resolveCluster(ctx, apiVersion, clusterKind, resource)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve NatsCluster reference for Account %s/%s: %w", account.GetNamespace(), account.GetName(), err)
		}
	} else if defaultClusterRef != "" {
		resource, err := parseClusterReference(defaultClusterRef, account.GetNamespace())
		if err != nil {
			return nil, fmt.Errorf("invalid default NatsCluster Reference %q: %w", defaultClusterRef, err)
		}
		cluster, err = r.resolveCluster(ctx, apiVersion, clusterKind, resource)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve default NatsCluster reference from %q: %w", resource, err)
		}
	}

	// Get the factory for this API version
	factory, ok := r.factories[clusterKind]
	if !ok {
		return nil, fmt.Errorf("no provider factory registered for Cluster Kind %q", clusterKind)
	}

	// Create the provider
	return factory.CreateProvider(ctx, cluster)
}

func (r *DefaultResolver) resolveCluster(ctx context.Context, apiVersion string, clusterKind string, resource types.NamespacedName) (*v1alpha1.NatsCluster, error) {
	if apiVersion != APIVersionNauth {
		return nil, fmt.Errorf("unsupported NatsCluster API Version %q", apiVersion)
	}
	if clusterKind != KindNatsCluster {
		return nil, fmt.Errorf("unsupported NatsCluster Kind %q", clusterKind)
	}

	cluster := &v1alpha1.NatsCluster{}
	if err := r.client.Get(ctx, resource, cluster); err != nil {
		return nil, fmt.Errorf("failed to resolve NatsCluster %q: %w", resource, err)
	}
	return cluster, nil
}

func parseClusterReference(value string, defaultNamespace string) (types.NamespacedName, error) {
	match := clusterRefRegexp.FindStringSubmatch(value)
	if match == nil {
		return types.NamespacedName{}, fmt.Errorf("invalid Cluster Reference pattern: %q", value)
	}
	matchMap := make(map[string]string)
	for i, name := range clusterRefRegexp.SubexpNames() {
		if i != 0 && name != "" {
			matchMap[name] = match[i]
		}
	}
	result := types.NamespacedName{
		Namespace: matchMap["namespace"],
		Name:      matchMap["name"],
	}
	if result.Namespace == "" {
		result.Namespace = defaultNamespace
	}
	return result, nil
}
