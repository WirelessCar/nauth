package inbound

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
)

type AccountManager interface {
	Create(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error)
	Update(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error)
	Import(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error)
	Delete(ctx context.Context, desired *v1alpha1.Account) error
}

type UserManager interface {
	CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error
	Delete(ctx context.Context, desired *v1alpha1.User) error
}

type NatsClusterManager interface {
	Validate(ctx context.Context, state *v1alpha1.NatsCluster) error
}
