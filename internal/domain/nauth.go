package domain

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
)

type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *v1alpha1.AccountClaims
}
type AccountExportClaim struct {
	Rules []v1alpha1.AccountExportRule
}
