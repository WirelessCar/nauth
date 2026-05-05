package controller

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type adoptionState struct {
	ObservedGeneration             int64
	Status                         metav1.ConditionStatus
	Reason                         string
	Message                        string
	DesiredClaimObservedGeneration *int64
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
