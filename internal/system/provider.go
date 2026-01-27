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

package system

import (
	"context"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// AccountResult contains the result of account operations
type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *natsv1alpha1.AccountClaims
}

// UserResult contains the result of user operations
type UserResult struct {
	UserID       string
	UserSignedBy string
	Claims       *natsv1alpha1.UserClaims
}

// Provider defines the interface for system backends (e.g., nauth, synadia cloud)
// Each provider implements the specific logic for managing NATS accounts and users
type Provider interface {
	// Account operations
	CreateAccount(ctx context.Context, account *natsv1alpha1.Account) (*AccountResult, error)
	UpdateAccount(ctx context.Context, account *natsv1alpha1.Account) (*AccountResult, error)
	ImportAccount(ctx context.Context, account *natsv1alpha1.Account) (*AccountResult, error)
	DeleteAccount(ctx context.Context, account *natsv1alpha1.Account) error

	// User operations
	CreateUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) (*UserResult, error)
	UpdateUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) (*UserResult, error)
	DeleteUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) error
}

// Resolver resolves the appropriate Provider for an Account based on its SystemRef
type Resolver interface {
	// ResolveForAccount returns the appropriate Provider for the given account
	// If Account.Spec.SystemRef is nil, returns a default/legacy provider
	ResolveForAccount(ctx context.Context, account *natsv1alpha1.Account) (Provider, error)
}
