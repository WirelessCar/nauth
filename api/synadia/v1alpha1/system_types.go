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

// SecretKeyReference contains information to locate a secret.
type SecretKeyReference struct {
	// Name of the Secret.
	// +required
	Name string `json:"name"`
	// Key in the Secret; when empty, implementation-specific default is used.
	// +optional
	Key string `json:"key,omitempty"`
	// Namespace of the Secret; when empty, defaults to the System's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ConfigMapKeyReference contains information to locate a value in a ConfigMap.
type ConfigMapKeyReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key"`
}

// SystemSelector selects a Synadia system by name (e.g. "NGS").
type SystemSelector struct {
	// Name of the system in Synadia Cloud (e.g. "NGS").
	// +required
	Name string `json:"name"`
}

// SystemSpec defines the desired state of System (Synadia Cloud connection).
// +kubebuilder:validation:XValidation:rule="has(self.teamId) != has(self.teamIdFrom)",message="exactly one of teamId or teamIdFrom must be specified"
type SystemSpec struct {
	// TeamID is the Synadia team identifier. Mutually exclusive with TeamIDFrom.
	// +optional
	TeamID string `json:"teamId,omitempty"`
	// TeamIDFrom loads the team ID from a ConfigMap or Secret. Mutually exclusive with TeamID.
	// +optional
	TeamIDFrom *TeamIDFromReference `json:"teamIdFrom,omitempty"`
	// SystemSelector selects the system by name (e.g. "NGS").
	// +required
	SystemSelector SystemSelector `json:"systemSelector"`
	// APICredentialsSecretRef references the Secret containing the Bearer token for the Synadia API.
	// +required
	APICredentialsSecretRef SecretKeyReference `json:"apiCredentialsSecretRef"`
	// APIEndpoint is the Synadia Cloud API base URL (e.g. https://cloud.synadia.com).
	// +required
	APIEndpoint string `json:"apiEndpoint"`
	// ReconcileInterval is how often accounts and users backed by this System are requeued for reconciliation.
	// Defaults to 5m if not set.
	// +optional
	ReconcileInterval *metav1.Duration `json:"reconcileInterval,omitempty"`
}

// TeamIDFromReference describes how to load the team ID from a ConfigMap or Secret.
type TeamIDFromReference struct {
	// Kind is ConfigMap or Secret.
	// +kubebuilder:validation:Enum=ConfigMap;Secret
	Kind string `json:"kind"`
	Name string `json:"name"`
	// +optional
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key"`
}

// SystemStatus defines the observed state of System.
type SystemStatus struct {
	// SystemID is the Synadia system identifier after resolution.
	// +optional
	SystemID string `json:"systemId,omitempty"`
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
// +kubebuilder:printcolumn:name="SystemID",type=string,JSONPath=`.status.systemId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// System is the Schema for the Synadia Cloud system connection.
type System struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemSpec   `json:"spec,omitempty"`
	Status SystemStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SystemList contains a list of System.
type SystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []System `json:"items"`
}

// GetConditions returns the status conditions (for use with controller status reporter).
func (s *System) GetConditions() *[]metav1.Condition {
	return &s.Status.Conditions
}

func init() {
	SchemeBuilder.Register(&System{}, &SystemList{})
}
