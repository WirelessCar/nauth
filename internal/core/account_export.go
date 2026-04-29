package core

import (
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
)

type AccountExportManager struct {
}

func NewAccountExportManager() *AccountExportManager {
	return &AccountExportManager{}
}

func (a *AccountExportManager) ValidateExports(exports nauth.Exports) error {
	return validateExports(exports)
}

var _ inbound.AccountExportManager = (*AccountExportManager)(nil)
