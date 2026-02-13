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

package cluster

import (
	"context"
	"time"

	v1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// AccountResult contains the result of account operations
type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *v1alpha1.AccountClaims
	// RequeueAfter optionally requests the controller to requeue after this duration (e.g. for periodic sync).
	RequeueAfter *time.Duration
	// AccountNkeyRotated is set when the account's public key changed; controller should patch Users to refresh creds.
	AccountNkeyRotated bool
}

// UserResult contains the result of user operations
type UserResult struct {
	UserID       string
	UserSignedBy string
	Claims       *v1alpha1.UserClaims
	// RequeueAfter optionally requests the controller to requeue after this duration (e.g. for periodic sync).
	RequeueAfter *time.Duration
}

// Provider defines the interface for cluster backends (e.g., nauth, Synadia)
// Each provider implements the specific logic for managing NATS accounts and users
type Provider interface {
	// CreateAccount creates a new account.
	CreateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	// UpdateAccount updates an existing account (account has account ID in labels).
	UpdateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	// ImportAccount syncs state for observe policy (account already has ID).
	ImportAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	DeleteAccount(ctx context.Context, account *v1alpha1.Account) error

	// CreateOrUpdateUser creates a new user or updates an existing one.
	// The provider decides whether to create or update based on the user's state.
	CreateOrUpdateUser(ctx context.Context, user *v1alpha1.User) (*UserResult, error)
	DeleteUser(ctx context.Context, user *v1alpha1.User) error
}

// ProviderFactory creates Provider instances for a cluster type.
// config is the cluster config object (e.g. *v1alpha1.NatsCluster or *synadia.System); may be nil for legacy.
type ProviderFactory interface {
	CreateProvider(ctx context.Context, config any) (Provider, error)
	// RequiresPeriodicSync reports whether this backend needs periodic
	// reconciliation even when the resource spec hasn't changed
	// (e.g. for syncing state with an external API).
	RequiresPeriodicSync() bool
}

// Resolver resolves the appropriate Provider for an Account based on its NatsClusterRef
type Resolver interface {
	// ResolveForAccount returns the appropriate Provider for the given account
	// If Account.Spec.NatsClusterRef is nil, returns a default/legacy provider
	ResolveForAccount(ctx context.Context, account *v1alpha1.Account) (Provider, error)
	// RequiresPeriodicSync reports whether the backend for the given account
	// needs periodic reconciliation even when the resource spec hasn't changed.
	RequiresPeriodicSync(account *v1alpha1.Account) bool
}
