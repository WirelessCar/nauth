package controller

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

func findAdoptionByUID(account *v1alpha1.Account, uid types.UID) *v1alpha1.AccountAdoption {
	if account.Status.Adoptions == nil {
		return nil
	}

	for _, adoption := range account.Status.Adoptions.Exports {
		if adoption.UID == uid {
			return &adoption
		}
	}

	return nil
}
