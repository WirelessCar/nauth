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

package nauth

import (
	"context"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/cluster/nauth/account"
	"github.com/WirelessCar/nauth/internal/cluster/nauth/user"
	"github.com/WirelessCar/nauth/internal/k8s"
)

// Provider implements cluster.Provider for native NATS authentication
// by delegating to the existing account and user managers
type Provider struct {
	accountManager *account.Manager
	userManager    *user.Manager
}

// NewProvider creates a new nauth Provider that wraps existing managers
func NewProvider(accountManager *account.Manager, userManager *user.Manager) *Provider {
	return &Provider{
		accountManager: accountManager,
		userManager:    userManager,
	}
}

// ============================================================================
// Account operations - delegate to account.Manager
// ============================================================================

func (p *Provider) CreateAccount(ctx context.Context, acc *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	result, err := p.accountManager.Create(ctx, acc)
	if err != nil {
		return nil, err
	}
	return &cluster.AccountResult{
		AccountID:       result.AccountID,
		AccountSignedBy: result.AccountSignedBy,
		Claims:          result.Claims,
	}, nil
}

func (p *Provider) UpdateAccount(ctx context.Context, acc *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	result, err := p.accountManager.Update(ctx, acc)
	if err != nil {
		return nil, err
	}
	return &cluster.AccountResult{
		AccountID:       result.AccountID,
		AccountSignedBy: result.AccountSignedBy,
		Claims:          result.Claims,
	}, nil
}

func (p *Provider) ImportAccount(ctx context.Context, acc *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	result, err := p.accountManager.Import(ctx, acc)
	if err != nil {
		return nil, err
	}
	return &cluster.AccountResult{
		AccountID:       result.AccountID,
		AccountSignedBy: result.AccountSignedBy,
		Claims:          result.Claims,
	}, nil
}

func (p *Provider) DeleteAccount(ctx context.Context, acc *nauthv1alpha1.Account) error {
	return p.accountManager.Delete(ctx, acc)
}

// ============================================================================
// User operations - delegate to user.Manager
// ============================================================================

// CreateOrUpdateUser creates a new user or updates an existing one.
// The user manager internally handles the create vs update decision.
func (p *Provider) CreateOrUpdateUser(ctx context.Context, usr *nauthv1alpha1.User) (*cluster.UserResult, error) {
	// The user manager handles everything internally including looking up the account
	if err := p.userManager.CreateOrUpdate(ctx, usr); err != nil {
		return nil, err
	}

	// Extract result from the user's updated status/labels
	return &cluster.UserResult{
		UserID:       usr.Labels[k8s.LabelUserID],
		UserSignedBy: usr.Labels[k8s.LabelUserSignedBy],
		Claims:       &usr.Status.Claims,
	}, nil
}

func (p *Provider) DeleteUser(ctx context.Context, usr *nauthv1alpha1.User) error {
	return p.userManager.Delete(ctx, usr)
}

// Ensure Provider implements cluster.Provider
var _ cluster.Provider = (*Provider)(nil)
