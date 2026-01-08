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
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UserSpec defines the desired state of User.
type UserSpec struct {
	// AccountName references the account used to create the user.
	AccountName string `json:"accountName"`
	// +optional
	Permissions *Permissions `json:"permissions,omitempty"`
	// +optional
	UserLimits *UserLimits `json:"userLimits,omitempty"`
	// +optional
	NatsLimits *NatsLimits `json:"natsLimits,omitempty"`
}

type UserClaims struct {
	// Deprecated. Will be removed in a future release (>v0.5.0). Ref: https://github.com/WirelessCar/nauth/issues/102
	// +optional
	AccountName string `json:"accountName"`
	// +optional
	Permissions *Permissions `json:"permissions,omitempty"`
	// +optional
	NatsLimits *NatsLimits `json:"natsLimits,omitempty"`
	// +optional
	UserLimits *UserLimits `json:"userLimits,omitempty"`
}

// UserStatus defines the observed state of User.
type UserStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// +optional
	Claims UserClaims `json:"claims,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	ReconcileTimestamp metav1.Time `json:"reconcileTimestamp,omitempty"`
	// +optional
	OperatorVersion string `json:"operatorVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`

// User is the Schema for the users API.
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

func (u *User) GetConditions() *[]metav1.Condition {
	return &u.Status.Conditions
}

func (u *User) GetUserSecretName() string {
	return fmt.Sprintf("%s-nats-user-creds", u.GetName())
}

// +kubebuilder:object:root=true

// UserList contains a list of User.
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

type UserLimits struct {
	// +optional
	// Src is a comma separated list of CIDR specifications
	Src CIDRList `json:"src,omitempty"`
	// +optional
	Times []TimeRange `json:"times,omitempty"`
	// +optional
	Locale string `json:"timesLocation,omitempty"`
}

func (u *UserLimits) Empty() bool {
	return reflect.DeepEqual(*u, UserLimits{})
}

func (u *UserLimits) IsUnlimited() bool {
	return len(u.Src) == 0 && len(u.Times) == 0
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}
