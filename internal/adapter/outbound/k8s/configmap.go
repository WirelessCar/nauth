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
	"maps"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ConfigMapClient reads and writes ConfigMaps.
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
		return nil, domain.ErrBadRequest.WithCause(fmt.Errorf("invalid ConfigMap reference %q: %w", configMapRef, err))
	}
	cm := &v1.ConfigMap{}
	key := client.ObjectKey{Namespace: configMapRef.Namespace, Name: configMapRef.Name}
	if err := c.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, domain.ErrConfigMapNotFound
		}
		return nil, domain.ErrUnknownError.WithCause(fmt.Errorf("failed to get ConfigMap %s: %w", configMapRef, err))
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

// Apply creates or updates a NAuth-managed ConfigMap controlled by owner and labels it LabelManaged.
// Returns domain.ErrConfigMapConflict without writing when the existing ConfigMap is unmanaged
// or controlled by another owner.
func (c *ConfigMapClient) Apply(ctx context.Context, owner metav1.Object, meta metav1.ObjectMeta, data map[string]string) error {
	key := client.ObjectKey{Namespace: meta.Namespace, Name: meta.Name}
	cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: meta.Namespace, Name: meta.Name}}
	_, err := controllerutil.CreateOrUpdate(ctx, c.client, cm, func() error {
		if cm.ResourceVersion != "" {
			if cm.Labels[LabelManaged] != LabelManagedValue {
				return domain.ErrConfigMapConflict.WithCause(fmt.Errorf("ConfigMap %s not managed by NAuth", key))
			}
			if ref := metav1.GetControllerOf(cm); ref != nil && ref.UID != owner.GetUID() {
				return domain.ErrConfigMapConflict.WithCause(fmt.Errorf("ConfigMap %s controlled by %s %s", key, ref.Kind, ref.Name))
			}
		}
		if cm.Labels == nil {
			cm.Labels = map[string]string{}
		}
		maps.Copy(cm.Labels, meta.Labels)
		cm.Labels[LabelManaged] = LabelManagedValue
		cm.Data = data
		return controllerutil.SetControllerReference(owner, cm, c.client.Scheme())
	})
	if err != nil {
		return fmt.Errorf("failed to apply ConfigMap %s: %w", key, err)
	}
	return nil
}

// Compile-time assertion that implementation satisfies the ports interface
var _ outbound.ConfigMapReader = (*ConfigMapClient)(nil)
