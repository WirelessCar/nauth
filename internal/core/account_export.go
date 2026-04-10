package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
)

type AccountExportManager struct {
}

func NewAccountExportManager() *AccountExportManager {
	return &AccountExportManager{}
}

func (a *AccountExportManager) CreateClaim(ctx context.Context, state *v1alpha1.AccountExport) (*domain.AccountExportClaim, error) {
	// TODO: [#22] Implement AccountExportManager.CreateClaim
	return nil, fmt.Errorf("[#22] AccountExportManager.CreateClaim not implemented")
}

var _ inbound.AccountExportManager = (*AccountExportManager)(nil)
