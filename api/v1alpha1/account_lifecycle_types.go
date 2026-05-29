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

type NatsAccountManagementPolicy string

const (
	NatsAccountManagementPolicyManage  NatsAccountManagementPolicy = "Manage"
	NatsAccountManagementPolicyObserve NatsAccountManagementPolicy = "Observe"
)

type AccountSecretsManagementPolicy string

const (
	AccountSecretsManagementPolicyGenerateIfMissing AccountSecretsManagementPolicy = "GenerateIfMissing"
	AccountSecretsManagementPolicyRequireExisting   AccountSecretsManagementPolicy = "RequireExisting"
)

type AccountLifecycleDeletionPolicy string

const (
	AccountLifecycleDeletionPolicyDelete AccountLifecycleDeletionPolicy = "Delete"
	AccountLifecycleDeletionPolicyRetain AccountLifecycleDeletionPolicy = "Retain"
)

// AccountLifecycle defines how NAuth manages external and supporting resources for an Account.
type AccountLifecycle struct {
	// NatsAccount controls lifecycle behavior for the NATS Account JWT.
	// +optional
	NatsAccount *NatsAccountLifecycle `json:"natsAccount,omitempty"`
	// Secrets controls lifecycle behavior for the Kubernetes Secrets that hold Account root and signing material.
	// +optional
	Secrets *AccountSecretsLifecycle `json:"secrets,omitempty"`
}

// NatsAccountLifecycle controls lifecycle behavior for the NATS Account JWT.
type NatsAccountLifecycle struct {
	// ManagementPolicy controls whether NAuth manages or only observes the NATS Account JWT.
	// +optional
	// +kubebuilder:validation:Enum=Manage;Observe
	// +kubebuilder:default=Manage
	ManagementPolicy NatsAccountManagementPolicy `json:"managementPolicy,omitempty"`
	// DeletionPolicy controls whether NAuth deletes the NATS Account JWT when the Account resource is deleted.
	// +optional
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	DeletionPolicy AccountLifecycleDeletionPolicy `json:"deletionPolicy,omitempty"`
}

// AccountSecretsLifecycle controls lifecycle behavior for Kubernetes Secrets that hold Account root and signing material.
type AccountSecretsLifecycle struct {
	// ManagementPolicy controls whether NAuth may generate missing Account root and signing Secrets.
	// +optional
	// +kubebuilder:validation:Enum=GenerateIfMissing;RequireExisting
	// +kubebuilder:default=GenerateIfMissing
	ManagementPolicy AccountSecretsManagementPolicy `json:"managementPolicy,omitempty"`
	// DeletionPolicy controls whether NAuth deletes Account root and signing Secrets when the Account resource is deleted.
	// +optional
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	DeletionPolicy AccountLifecycleDeletionPolicy `json:"deletionPolicy,omitempty"`
}
