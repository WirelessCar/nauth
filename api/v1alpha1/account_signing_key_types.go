/*
Copyright 2026.

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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.status.managementPolicy`
// +kubebuilder:printcolumn:name="Public Key",type=string,JSONPath=`.status.publicKey`

// AccountSigningKey manages one NATS account signing-key seed in a Kubernetes Secret.
// By default NAuth manages the signing key seed: it generates a new key and stores it in
// a Secret named spec.secretName (defaulting to <resourceName>-ac-sign). The Secret is
// owned by this resource and garbage-collected when the resource is deleted.
//
// In observe mode (label nauth.io/management-policy=observe), NAuth only reads an existing
// Secret with the resolved name and derives the public key. Observed Secrets are not
// modified, owned, or deleted by the operator.
//
// An Account trusts the public key by listing this resource in Account.spec.signingKeyRefs.
type AccountSigningKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountSigningKeySpec   `json:"spec,omitempty"`
	Status AccountSigningKeyStatus `json:"status,omitempty"`
}

func (a *AccountSigningKey) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

// AccountSigningKeySpec defines the desired state of AccountSigningKey.
type AccountSigningKeySpec struct {
	// SecretName names the Kubernetes Secret that holds the account signing-key seed.
	//
	// In managed mode (default), SecretName is optional and defaults to
	// <resourceName>-ac-sign; the Secret is created and owned by this AccountSigningKey.
	// In observe mode (label nauth.io/management-policy=observe), SecretName is
	// required and identifies the existing Secret to read; the operator never falls
	// back to the managed default name and never modifies the Secret.
	//
	// Immutable.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="secretName is immutable"
	SecretName string `json:"secretName,omitempty"`
}

// AccountSigningKeyStatus defines the observed state of AccountSigningKey.
type AccountSigningKeyStatus struct {
	// PublicKey is the resolved NATS public key (A-prefixed nkey) for this signing key.
	// +optional
	PublicKey string `json:"publicKey,omitempty"`

	// SecretName is the resolved name of the Secret holding the seed.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ManagementPolicy reflects the effective management policy for this resource.
	// Empty means managed (default); "observe" means the Secret is only read.
	// +optional
	ManagementPolicy string `json:"managementPolicy,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	ReconcileTimestamp metav1.Time `json:"reconcileTimestamp,omitempty"`
	// +optional
	OperatorVersion string `json:"operatorVersion,omitempty"`
}

// +kubebuilder:object:root=true

// AccountSigningKeyList contains a list of AccountSigningKey.
type AccountSigningKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountSigningKey `json:"items"`
}
