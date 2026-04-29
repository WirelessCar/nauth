package domain

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

type AccountResources struct {
	Account      v1alpha1.Account   `json:"account,omitempty"`
	ExportGroups nauth.ExportGroups `json:"exportGroups,omitempty"`
}

type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *nauth.AccountClaims
	ClaimsHash      string
	Adoptions       *nauth.AccountAdoptions
}
