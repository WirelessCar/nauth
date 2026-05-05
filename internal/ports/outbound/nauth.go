package outbound

import (
	"context"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

type ClusterReader interface {
	GetTarget(ctx context.Context, clusterRef nauth.ClusterRef) (*nauth.ClusterTarget, error)
}

// AccountReader reads NAuth Account resources
// TODO: [#228] Remove outbound.AccountReader as an outbound port
type AccountReader interface {
	Get(ctx context.Context, accountRef domain.NamespacedName) (*v1alpha1.Account, error)
}

type AccountIDReader interface {
	// GetAccountID returns the NAuth Account ID for the given account reference.
	// Returns domain.ErrBadRequest if the accountRef is invalid.
	// Returns domain.ErrAccountNotFound if the Account does not exist.
	// Returns domain.ErrAccountNotReady if the Account is not ready or does not have an Account ID label.
	GetAccountID(ctx context.Context, accountRef domain.NamespacedName) (nauth.AccountID, error)
}
