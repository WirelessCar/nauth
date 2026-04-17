package domain

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
)

type AccountResources struct {
	Account v1alpha1.Account
	Exports []v1alpha1.AccountExport
}

type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *v1alpha1.AccountClaims
	ClaimsHash      string
	Adoptions       *v1alpha1.AccountAdoptions
}

type AccountExportClaim struct {
	Rules []v1alpha1.AccountExportRule
}
