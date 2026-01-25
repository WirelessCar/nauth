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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// URLFromKind is the type of resource to load the NATS URL from.
// +kubebuilder:validation:Enum=ConfigMap;Secret
type URLFromKind string

const (
	URLFromKindConfigMap URLFromKind = "ConfigMap"
	URLFromKindSecret    URLFromKind = "Secret"
)

// SecretKeyReference contains information to locate a secret in the same namespace
type SecretKeyReference struct {
	// Name of the Secret.
	// +required
	Name string `json:"name"`

	// Key in the Secret, when not specified an implementation-specific default key is used.
	// +optional
	Key string `json:"key,omitempty"`
}

// URLFromReference describes how to load the NATS URL from a ConfigMap or Secret.
type URLFromReference struct {
	// Kind is the type of resource to load from: ConfigMap or Secret.
	// +required
	Kind URLFromKind `json:"kind"`

	// Name of the ConfigMap or Secret.
	// +required
	Name string `json:"name"`

	// Namespace of the resource. When empty, defaults to the NatsCluster's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key in the ConfigMap or Secret whose value is the NATS URL.
	// +required
	Key string `json:"key"`
}

// NatsClusterSpec defines the desired state of NatsCluster
// +kubebuilder:validation:XValidation:rule="has(self.url) != has(self.urlFrom)",message="exactly one of url or urlFrom must be specified"
type NatsClusterSpec struct {
	// URL is the NATS server URL for this cluster. Mutually exclusive with urlFrom.
	// +optional
	URL string `json:"url,omitempty"`

	// URLFrom loads the NATS URL from a ConfigMap or Secret. Mutually exclusive with url.
	// +optional
	URLFrom *URLFromReference `json:"urlFrom,omitempty"`

	OperatorSigningKeySecretRef     SecretKeyReference `json:"operatorSigningKeySecretRef"`
	SystemAccountUserCredsSecretRef SecretKeyReference `json:"systemAccountUserCredsSecretRef"`
}

// NatsClusterStatus defines the observed state of NatsCluster
type NatsClusterStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// NatsCluster is the Schema for the natsclusters API
type NatsCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatsClusterSpec   `json:"spec,omitempty"`
	Status NatsClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NatsClusterList contains a list of NatsCluster
type NatsClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatsCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NatsCluster{}, &NatsClusterList{})
}
