package inbound

import (
	"context"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

type AccountManager interface {
	CreateOrUpdate(ctx context.Context, request nauth.AccountRequest) (*nauth.AccountResult, error)
	Import(ctx context.Context, reference nauth.AccountReference) (*nauth.AccountResult, error)
	FindAccountID(ctx context.Context, reference nauth.AccountReference) (nauth.AccountID, bool, error)
	Delete(ctx context.Context, reference nauth.AccountReference) error
}

type AccountExportManager interface {
	ValidateExports(exports nauth.Exports) error
}

type AccountImportManager interface {
	ValidateImports(importAccountID nauth.AccountID, imports nauth.Imports) error
}

type AccountSigningKeyManager interface {
	CreateOrUpdate(ctx context.Context, request nauth.AccountSigningKeyRequest) (*nauth.AccountSigningKeyResult, error)
	Import(ctx context.Context, secretRef domain.NamespacedName) (*nauth.AccountSigningKeyResult, error)
}

type UserManager interface {
	CreateOrUpdate(ctx context.Context, request nauth.UserRequest) (*nauth.UserResult, error)
}

type ClusterManager interface {
	GetClusterTarget(ctx context.Context, accountClusterRef *nauth.ClusterRef) (*nauth.ClusterTarget, error)
	Validate(ctx context.Context, target nauth.ClusterTarget) error
}
