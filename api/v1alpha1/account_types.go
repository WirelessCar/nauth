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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountSpec defines the desired state of Account.
type AccountSpec struct {
	// +optional
	AccountLimits *AccountLimits `json:"accountLimits,omitempty"`
	// +optional
	Exports Exports `json:"exports,omitempty"`
	// +optional
	Imports Imports `json:"imports,omitempty"`
	// +optional
	JetStreamLimits *JetStreamLimits `json:"jetStreamLimits,omitempty"`
	// +optional
	NatsLimits *NatsLimits `json:"natsLimits,omitempty"`
}

type AccountClaims struct {
	// +optional
	AccountLimits *AccountLimits `json:"accountLimits,omitempty"`
	// +optional
	Exports Exports `json:"exports,omitempty"`
	// +optional
	Imports Imports `json:"imports,omitempty"`
	// +optional
	JetStreamLimits *JetStreamLimits `json:"jetStreamLimits,omitempty"`
	// +optional
	NatsLimits *NatsLimits `json:"natsLimits,omitempty"`
}

// AccountStatus defines the observed state of Account.
type AccountStatus struct {
	// +optional
	Claims AccountClaims `json:"claims,omitempty"`
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
	SigningKey KeyInfo `json:"signingKey"`
}

type KeyInfo struct {
	Name           string      `json:"name,omitempty"`
	CreationDate   metav1.Time `json:"creationDate,omitempty"`
	ExpirationDate metav1.Time `json:"expirationDate,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Account is the Schema for the accounts API.
type Account struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountSpec   `json:"spec,omitempty"`
	Status AccountStatus `json:"status,omitempty"`
}

func (a *Account) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

func (a *Account) GetAccountSecretName() string {
	return fmt.Sprintf("%s-ac-root", a.GetName())
}

func (a *Account) GetAccountSignSecretName() string {
	return fmt.Sprintf("%s-ac-sign", a.GetName())
}

type JetStreamLimits struct {
	// +optional
	// +kubebuilder:default=-1
	MemoryStorage *int64 `json:"memStorage,omitempty"` // Max number of bytes stored in memory across all streams. (0 means disabled)
	// +optional
	// +kubebuilder:default=-1
	DiskStorage *int64 `json:"diskStorage,omitempty"` // Max number of bytes stored on disk across all streams. (0 means disabled)
	// +optional
	// +kubebuilder:default=-1
	Streams *int64 `json:"streams,omitempty"` // Max number of streams
	// +optional
	// +kubebuilder:default=-1
	Consumer *int64 `json:"consumer,omitempty"` // Max number of consumers
	// +optional
	// +kubebuilder:default=-1
	MaxAckPending *int64 `json:"maxAckPending,omitempty"` // Max ack pending of a Stream
	// +optional
	// +kubebuilder:default=-1
	MemoryMaxStreamBytes *int64 `json:"memMaxStreamBytes,omitempty"` // Max bytes a memory backed stream can have. (0 means disabled/unlimited)
	// +optional
	// +kubebuilder:default=-1
	DiskMaxStreamBytes *int64 `json:"diskMaxStreamBytes,omitempty"` // Max bytes a disk backed stream can have. (0 means disabled/unlimited)
	// +optional
	// +kubebuilder:default=false
	MaxBytesRequired bool `json:"maxBytesRequired,omitempty"` // Max bytes required by all Streams
}

type AccountLimits struct {
	// +optional
	// +kubebuilder:default=-1
	Imports *int64 `json:"imports,omitempty"` // Max number of imports
	// +optional
	// +kubebuilder:default=-1
	Exports *int64 `json:"exports,omitempty"` // Max number of exports
	// +optional
	// +kubebuilder:default=true
	WildcardExports *bool `json:"wildcards,omitempty"` // Are wildcards allowed in exports
	// +optional
	// +kubebuilder:default=-1
	Conn *int64 `json:"conn,omitempty"` // Max number of active connections
	// +optional
	// +kubebuilder:default=-1
	LeafNodeConn *int64 `json:"leaf,omitempty"` // Max number of active leaf node connections
}

// +kubebuilder:object:root=true

// AccountList contains a list of Account.
type AccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Account `json:"items"`
}

type RevocationList map[string]int64

func init() {
	SchemeBuilder.Register(&Account{}, &AccountList{})
}

type Exports []*Export
type Export struct {
	Name                 string          `json:"name,omitempty"`
	Subject              Subject         `json:"subject,omitempty"`
	Type                 ExportType      `json:"type,omitempty"`
	TokenReq             bool            `json:"tokenReq,omitempty"`
	Revocations          RevocationList  `json:"revocations,omitempty"`
	ResponseType         ResponseType    `json:"responseType,omitempty"`
	ResponseThreshold    time.Duration   `json:"responseThreshold,omitempty"`
	Latency              *ServiceLatency `json:"serviceLatency,omitempty"`
	AccountTokenPosition uint            `json:"accountTokenPosition,omitempty"`
	Advertise            bool            `json:"advertise,omitempty"`
	AllowTrace           bool            `json:"allowTrace,omitempty"`
}

type Imports []*Import
type Import struct {
	// AccountRefName references the account used to create the user.
	AccountRef AccountRef `json:"accountRef"`
	Name       string     `json:"name,omitempty"`
	// Subject field in an import is always from the perspective of the
	// initial publisher - in the case of a stream it is the account owning
	// the stream (the exporter), and in the case of a service it is the
	// account making the request (the importer).
	Subject Subject `json:"subject,omitempty"`
	Account string  `json:"account,omitempty"`
	// Local subject used to subscribe (for streams) and publish (for services) to.
	// This value only needs setting if you want to change the value of Subject.
	// If the value of Subject ends in > then LocalSubject needs to end in > as well.
	// LocalSubject can contain $<number> wildcard references where number references the nth wildcard in Subject.
	// The sum of wildcard reference and * tokens needs to match the number of * token in Subject.
	LocalSubject RenamingSubject `json:"localSubject,omitempty"`
	Type         ExportType      `json:"type,omitempty"`
	Share        bool            `json:"share,omitempty"`
	AllowTrace   bool            `json:"allowTrace,omitempty"`
}

type ServiceLatency struct {
	Sampling SamplingRate `json:"sampling"`
	Results  Subject      `json:"results"`
}

type SamplingRate int

type AccountRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
