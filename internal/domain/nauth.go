package domain

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
)

type AccountResources struct {
	Account v1alpha1.Account         `json:"account,omitempty"`
	Exports []v1alpha1.AccountExport `json:"exports,omitempty"`
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

type AccountExportResolution struct {
	ObservedGeneration int64
	AccountID          string
	BindingState       AccountBindingState
	BoundAccountID     string
	DesiredClaim       *AccountExportClaim
	ValidationError    error
	AdoptionState      AccountAdoptionState
	Adoption           *v1alpha1.AccountAdoption
	AdoptionError      error
}

type AccountBindingState int
type AccountAdoptionState int

const (
	AccountBindingStateMissing AccountBindingState = iota
	AccountBindingStateConflict
	AccountBindingStateBound
)
const (
	AccountAdoptionStateMissing AccountAdoptionState = iota
	AccountAdoptionStateNotAdopted
	AccountAdoptionStateAdopted
)

func (r AccountExportResolution) AccountIdBound() bool {
	return r.BoundAccountID != ""
}

func (r AccountExportResolution) AccountIdConflict() bool {
	return r.AccountIdBound() && r.BoundAccountID != r.AccountID
}

func (r AccountExportResolution) ValidRules() bool {
	return r.ValidationError == nil
}
