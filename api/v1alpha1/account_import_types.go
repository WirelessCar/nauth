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

type AccountImportLabel string

const (
	AccountImportLabelAccountID       AccountImportLabel = "accountimport.nauth.io/account-id"
	AccountImportLabelExportAccountID AccountImportLabel = "accountimport.nauth.io/export-account-id"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Rules",type=string,JSONPath=`.status.conditions[?(@.type=="ValidRules")].reason`
// +kubebuilder:printcolumn:name="Account",type=string,JSONPath=`.status.conditions[?(@.type=="BoundToAccount")].reason`
// +kubebuilder:printcolumn:name="Export",type=string,JSONPath=`.status.conditions[?(@.type=="BoundToExportAccount")].reason`
// +kubebuilder:printcolumn:name="Adopted",type=string,JSONPath=`.status.conditions[?(@.type=="AdoptedByAccount")].reason`

// AccountImport is a component resource for imports in the accounts API.
type AccountImport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountImportSpec   `json:"spec,omitempty"`
	Status AccountImportStatus `json:"status,omitempty"`
}

func (a *AccountImport) GetConditions() *[]metav1.Condition {
	return &a.Status.Conditions
}

func (a *AccountImport) GetLabel(label AccountImportLabel) string {
	return a.GetLabels()[string(label)]
}

func (a *AccountImport) SetLabel(label AccountImportLabel, value string) {
	if a.Labels == nil {
		a.Labels = make(map[string]string)
	}
	a.Labels[string(label)] = value
}

// AccountImportSpec defines the desired state of AccountImport.
type AccountImportSpec struct {
	// AccountName refers to the Account in the same namespace to which this import applies.
	// +required
	AccountName string `json:"accountName"`
	// ExportAccountRef refers to the Account from which the exports are imported.
	// This reference may point to an Account in another namespace.
	// +required
	ExportAccountRef AccountRef `json:"exportAccountRef"`
	// Rules defines the import rules for this AccountImport.
	// +required
	// +kubebuilder:validation:MinItems=1
	Rules []AccountImportRule `json:"rules"`
}

type AccountImportRule struct {
	// +optional
	Name string `json:"name,omitempty"`
	// Subject is the exported subject to import.
	// It must be identical to or a subset of the exported subject.
	// +required
	Subject Subject `json:"subject,omitempty"`
	// LocalSubject remaps the imported subject locally in the importing account.
	// +optional
	LocalSubject RenamingSubject `json:"localSubject,omitempty"`
	// Type defines whether the import is a stream or service import.
	// +required
	Type ExportType `json:"type,omitempty"`
	// +optional
	Share *bool `json:"share,omitempty"`
	// +optional
	AllowTrace *bool `json:"allowTrace,omitempty"`
}

type AccountImportRuleDerived struct {
	AccountImportRule `json:",inline"`
	// Account is the resolved export account ID used for this import rule.
	// +required
	Account string `json:"account,omitempty"`
}

// AccountImportStatus defines the observed state of AccountImport.
type AccountImportStatus struct {
	// AccountID is the resolved ID of the Account referenced by spec.accountName.
	// +optional
	AccountID string `json:"accountID,omitempty"`
	// ExportAccountID is the resolved ID of the Account referenced by spec.exportAccountRef.
	// +optional
	ExportAccountID string `json:"exportAccountID,omitempty"`
	// DesiredClaim is the normalized claim for Account to use.
	// +optional
	DesiredClaim *AccountImportClaim `json:"desiredClaim,omitempty"`

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

type AccountImportClaim struct {
	// Rules contains import rules that have been validated and are ready to be used by Account.
	// +required
	// +kubebuilder:validation:MinItems=1
	Rules []AccountImportRuleDerived `json:"rules,omitempty"`
	// +required
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

// AccountImportList contains a list of AccountImport.
type AccountImportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountImport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccountImport{}, &AccountImportList{})
}
