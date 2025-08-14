package ports

import (
	"context"

	natsv1alpha1 "github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
)

type AccountManager interface {
	RefreshState(ctx context.Context, observed *types.Account, desired *natsv1alpha1.Account) error
	CreateAccount(ctx context.Context, state *natsv1alpha1.Account) error
	UpdateAccount(ctx context.Context, state *natsv1alpha1.Account) error
	DeleteAccount(ctx context.Context, desired *natsv1alpha1.Account) error
}

type UserManager interface {
	CreateOrUpdateUser(ctx context.Context, state *natsv1alpha1.User) error
	DeleteUser(ctx context.Context, desired *natsv1alpha1.User) error
}
