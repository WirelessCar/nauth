package provider

import (
	"context"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// SystemProvider defines the interface for different system backends
type SystemProvider interface {
	// Account operations
	CreateAccount(ctx context.Context, account *natsv1alpha1.Account) (*AccountResult, error)
	UpdateAccount(ctx context.Context, account *natsv1alpha1.Account) (*AccountResult, error)
	DeleteAccount(ctx context.Context, account *natsv1alpha1.Account) error

	// User operations
	CreateUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) (*UserResult, error)
	UpdateUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) (*UserResult, error)
	DeleteUser(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) error
	GetUserCredentials(ctx context.Context, user *natsv1alpha1.User, account *natsv1alpha1.Account) (string, error)
}

// SystemResolver resolves the appropriate SystemProvider for an Account
type SystemResolver interface {
	ResolveProvider(ctx context.Context, account *natsv1alpha1.Account) (SystemProvider, error)
}

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
