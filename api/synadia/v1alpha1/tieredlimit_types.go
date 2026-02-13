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

// AccountRef references a nauth.io Account by name and namespace.
type AccountRef struct {
	// Name of the Account.
	// +required
	Name string `json:"name"`
	// Namespace of the Account; when empty, defaults to the TieredLimit's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// TieredLimitTier holds Synadia tiered limit values (R1 or R3).
// MemStorage and MemMaxStreamBytes are not available for Synadia Cloud and are not included.
type TieredLimitTier struct {
	// DiskStorage in bytes (limited; -1 in CR is translated to 0 for API).
	// +optional
	DiskStorage *int64 `json:"diskStorage,omitempty"`
	// DiskMaxStreamBytes is max bytes per stream on disk (limited; -1 in CR → 0 for API).
	// +optional
	DiskMaxStreamBytes *int64 `json:"diskMaxStreamBytes,omitempty"`
	// Streams max count (limited; -1 in CR → 0 for API).
	// +optional
	Streams *int64 `json:"streams,omitempty"`
	// Consumer max count (limited; -1 in CR → 0 for API).
	// +optional
	Consumer *int64 `json:"consumer,omitempty"`
	// MaxAckPending is max pending acks per consumer; -1 means unlimited (passed through to API).
	// +optional
	MaxAckPending *int64 `json:"maxAckPending,omitempty"`
	// MaxBytesRequired when true requires max_bytes to be set on streams/consumers (default true).
	// +optional
	MaxBytesRequired *bool `json:"maxBytesRequired,omitempty"`
}

// TieredLimitSpec defines the desired state of TieredLimit.
// Exactly one TieredLimit per account is used; the controller selects and marks it via status.selectedForAccount.
type TieredLimitSpec struct {
	// AccountRef references the nauth.io Account this TieredLimit applies to.
	// +required
	AccountRef AccountRef `json:"accountRef"`
	// R1 tier limits.
	// +optional
	R1 *TieredLimitTier `json:"r1,omitempty"`
	// R3 tier limits.
	// +optional
	R3 *TieredLimitTier `json:"r3,omitempty"`
}

// TieredLimitStatus defines the observed state of TieredLimit.
type TieredLimitStatus struct {
	// SelectedForAccount is set by the controller to mark this TieredLimit as the one in use for the given account.
	// At most one TieredLimit per account has this set; the controller ensures only that one is used.
	// +optional
	SelectedForAccount *AccountRef `json:"selectedForAccount,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced

// TieredLimit is the Schema for Synadia tiered limits (one per account).
type TieredLimit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TieredLimitSpec   `json:"spec,omitempty"`
	Status TieredLimitStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TieredLimitList contains a list of TieredLimit.
type TieredLimitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TieredLimit `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TieredLimit{}, &TieredLimitList{})
}
