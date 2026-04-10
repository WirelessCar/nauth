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

package k8s

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapClient reads ConfigMap data from the cluster.
type ConfigMapClient struct {
	client client.Client
}

// NewConfigMapClient creates a new ConfigMap client.
func NewConfigMapClient(c client.Client) *ConfigMapClient {
	return &ConfigMapClient{client: c}
}

// Get returns the ConfigMap data as a map of key to string value.
// Keys from both Data and BinaryData are included.
func (c *ConfigMapClient) Get(ctx context.Context, configMapRef domain.NamespacedName) (map[string]string, error) {
	if err := configMapRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ConfigMap reference %q: %w", configMapRef, err)
	}
	cm := &v1.ConfigMap{}
	key := client.ObjectKey{Namespace: configMapRef.Namespace, Name: configMapRef.Name}
	if err := c.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", configMapRef, err)
	}
	result := make(map[string]string)
	for k, v := range cm.Data {
		result[k] = v
	}
	for k, v := range cm.BinaryData {
		result[k] = string(v)
	}
	return result, nil
}

// Compile-time assertion that implementation satisfies the ports interface
var _ outbound.ConfigMapReader = (*ConfigMapClient)(nil)
