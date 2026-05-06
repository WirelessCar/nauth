package inbound

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

type AccountManager interface {
	CreateOrUpdate(ctx context.Context, request nauth.AccountRequest) (*nauth.AccountResult, error)
	Import(ctx context.Context, reference nauth.AccountReference) (*nauth.AccountResult, error)
	Delete(ctx context.Context, reference nauth.AccountReference) error
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

type ClusterManager interface {
	Validate(ctx context.Context, target nauth.ClusterTarget) error
}
