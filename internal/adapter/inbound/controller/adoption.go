package controller

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

func findExportAdoptionByUID(adoptions *v1alpha1.AccountAdoptions, uid types.UID) *v1alpha1.AccountAdoption {
	if adoptions == nil {
		return nil
	}
	return findAdoptionByUID(adoptions.Exports, uid)
}

func findImportAdoptionByUID(adoptions *v1alpha1.AccountAdoptions, uid types.UID) *v1alpha1.AccountAdoption {
	if adoptions == nil {
		return nil
	}
	return findAdoptionByUID(adoptions.Imports, uid)
}

func findAdoptionByUID(adoptions []v1alpha1.AccountAdoption, uid types.UID) *v1alpha1.AccountAdoption {
	if adoptions == nil {
		return nil
	}

	for _, adoption := range adoptions {
		if adoption.UID == uid {
			return &adoption
		}
	}

	return nil
}
