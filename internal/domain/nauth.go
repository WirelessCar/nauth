package domain

import "github.com/WirelessCar/nauth/api/v1alpha1"

type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	NatsClusterRef  *NamespacedName
	Claims          *v1alpha1.AccountClaims
}
