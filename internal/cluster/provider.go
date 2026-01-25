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

	v1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// AccountResult contains the result of account operations
type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *v1alpha1.AccountClaims
}

// UserResult contains the result of user operations
type UserResult struct {
	UserID       string
	UserSignedBy string
	Claims       *v1alpha1.UserClaims
}

// Provider defines the interface for cluster backends (e.g., nauth)
// Each provider implements the specific logic for managing NATS accounts and users
type Provider interface {
	// Account operations
	CreateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	UpdateAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	ImportAccount(ctx context.Context, account *v1alpha1.Account) (*AccountResult, error)
	DeleteAccount(ctx context.Context, account *v1alpha1.Account) error

	// CreateOrUpdateUser creates a new user or updates an existing one.
	// The provider decides whether to create or update based on the user's state.
	CreateOrUpdateUser(ctx context.Context, user *v1alpha1.User) (*UserResult, error)
	DeleteUser(ctx context.Context, user *v1alpha1.User) error
}

// Resolver resolves the appropriate Provider for an Account based on its NatsClusterRef
type Resolver interface {
	// ResolveForAccount returns the appropriate Provider for the given account
	// If Account.Spec.NatsClusterRef is nil, returns a default/legacy provider
	ResolveForAccount(ctx context.Context, account *v1alpha1.Account) (Provider, error)
}
