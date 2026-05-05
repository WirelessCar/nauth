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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AccountExportLabel string

const (
	AccountExportLabelAccountID AccountExportLabel = "accountexport.nauth.io/account-id"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Rules",type=string,JSONPath=`.status.conditions[?(@.type=="ValidRules")].reason`
// +kubebuilder:printcolumn:name="Bound",type=string,JSONPath=`.status.conditions[?(@.type=="BoundToAccount")].reason`
// +kubebuilder:printcolumn:name="Adopted",type=string,JSONPath=`.status.conditions[?(@.type=="AdoptedByAccount")].reason`

// AccountExport is a component resource for exports in the accounts API.
type AccountExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountExportSpec   `json:"spec,omitempty"`
	Status AccountExportStatus `json:"status,omitempty"`
}

func (a *AccountExport) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

func (a *AccountExport) GetLabel(label AccountExportLabel) string {
	return a.GetLabels()[string(label)]
}

func (a *AccountExport) SetLabel(label AccountExportLabel, value string) {
	if a.Labels == nil {
		a.Labels = make(map[string]string)
	}
	a.Labels[string(label)] = value
}

// AccountExportSpec defines the desired state of AccountExport.
type AccountExportSpec struct {
	// AccountName refers to the Account in the same namespace to which this export applies.
	// +required
	AccountName string `json:"accountName"`
	// Rules defines the export rules for this account export. Must have at least one rule.
	// +required
	// +kubebuilder:validation:MinItems=1
	Rules []AccountExportRule `json:"rules"`
}

type AccountExportRule struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +required
	Subject Subject `json:"subject,omitempty"`
	// +required
	Type ExportType `json:"type,omitempty"`
	// +optional
	ResponseType ResponseType `json:"responseType,omitempty"`
	// +optional
	ResponseThreshold *time.Duration `json:"responseThreshold,omitempty"`
	// +optional
	Latency *ServiceLatency `json:"serviceLatency,omitempty"`
	// +optional
	AccountTokenPosition *uint `json:"accountTokenPosition,omitempty"`
	// +optional
	Advertise *bool `json:"advertise,omitempty"`
	// +optional
	AllowTrace *bool `json:"allowTrace,omitempty"`
}

// AccountExportStatus defines the observed state of AccountExport.
type AccountExportStatus struct {
	// AccountID is the ID of the account that this export is bound to.
	// +optional
	AccountID string `json:"accountID,omitempty"`
	// Normalized claim for account to use
	// +optional
	DesiredClaim *AccountExportClaim `json:"desiredClaim,omitempty"`

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

type AccountExportClaim struct {
	// Rules contains export rules that have been validated and are ready to be used by Account
	// +required
	// +kubebuilder:validation:MinItems=1
	Rules []AccountExportRule `json:"rules,omitempty"`
	// +required
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

// AccountExportList contains a list of AccountExport.
type AccountExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountExport `json:"items"`
}
