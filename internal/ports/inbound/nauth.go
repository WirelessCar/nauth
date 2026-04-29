package inbound

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

type AccountManager interface {
	CreateOrUpdate(ctx context.Context, accountResources nauth.AccountResources) (*nauth.AccountResult, error)
	Import(ctx context.Context, state *v1alpha1.Account) (*nauth.AccountResult, error)
	Delete(ctx context.Context, desired *v1alpha1.Account) error
}

type AccountExportManager interface {
	ValidateExports(exports nauth.Exports) error
}

type AccountImportManager interface {
	ValidateImports(importAccountID nauth.AccountID, imports nauth.Imports) error
}

type UserManager interface {
	CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error
	Delete(ctx context.Context, desired *v1alpha1.User) error
}

type NatsClusterManager interface {
	Validate(ctx context.Context, state *v1alpha1.NatsCluster) error
}
