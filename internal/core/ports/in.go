package ports

import (
	"context"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

type AccountManager interface {
	CreateAccount(ctx context.Context, state *natsv1alpha1.Account) error
	UpdateAccount(ctx context.Context, state *natsv1alpha1.Account) error
	DeleteAccount(ctx context.Context, desired *natsv1alpha1.Account) error
}

type UserManager interface {
	CreateOrUpdateUser(ctx context.Context, state *natsv1alpha1.User) error
	DeleteUser(ctx context.Context, desired *natsv1alpha1.User) error
}
