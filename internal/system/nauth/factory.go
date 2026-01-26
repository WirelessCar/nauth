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
	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/system"
	"github.com/WirelessCar/nauth/internal/system/nauth/account"
	natsc "github.com/WirelessCar/nauth/internal/system/nauth/nats"
	"github.com/WirelessCar/nauth/internal/system/nauth/user"
)

// Factory creates nauth Provider instances
type Factory struct {
	// Dependencies for creating managers
	accounts       account.AccountGetter
	secretClient   account.SecretClient
	natsURL        string
	nauthNamespace string
}

// NewFactory creates a new nauth Factory
func NewFactory(
	accounts account.AccountGetter,
	secretClient account.SecretClient,
	natsURL string,
	nauthNamespace string,
) *Factory {
	return &Factory{
		accounts:       accounts,
		secretClient:   secretClient,
		natsURL:        natsURL,
		nauthNamespace: nauthNamespace,
	}
}

// CreateProvider creates a new Provider configured for the given System
// If system is nil, creates a provider using legacy label-based configuration
func (f *Factory) CreateProvider(sys *natsv1alpha1.System) (system.Provider, error) {
	// Create NATS client - with or without System configuration
	var natsClient account.NatsClient
	if sys != nil {
		natsClient = natsc.NewClientWithSystem(f.natsURL, f.secretClient, sys)
	} else {
		natsClient = natsc.NewClient(f.natsURL, f.secretClient)
	}

	// Build options for the account manager
	opts := []func(*account.Manager){
		account.WithNamespace(f.nauthNamespace),
	}

	// If System is provided, add it to the options
	if sys != nil {
		opts = append(opts, account.WithSystem(sys))
	}

	// Create managers with the appropriate configuration
	accountManager := account.NewManager(f.accounts, natsClient, f.secretClient, opts...)
	userManager := user.NewManager(f.accounts, f.secretClient)

	return NewProvider(accountManager, userManager), nil
}

// Ensure Factory implements system.ProviderFactory
var _ system.ProviderFactory = (*Factory)(nil)
