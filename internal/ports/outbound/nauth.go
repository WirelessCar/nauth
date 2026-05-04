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
	Get(ctx context.Context, accountRef domain.NamespacedName) (account *v1alpha1.Account, err error)
}
