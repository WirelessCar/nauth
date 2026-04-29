package core

import (
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
)

type AccountImportManager struct {
}

func NewAccountImportManager() *AccountImportManager {
	return &AccountImportManager{}
}

func (a AccountImportManager) ValidateImports(importAccountID nauth.AccountID, imports nauth.Imports) error {
	return validateImports(importAccountID, imports)
}

var _ inbound.AccountImportManager = (*AccountImportManager)(nil)
