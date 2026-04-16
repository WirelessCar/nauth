package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/nats-io/jwt/v2"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountExportManager struct {
}

func NewAccountExportManager() *AccountExportManager {
	return &AccountExportManager{}
}

func (a *AccountExportManager) Resolve(ctx context.Context, state *v1alpha1.AccountExport, account *v1alpha1.Account) *domain.AccountExportResolution {
	result := &domain.AccountExportResolution{
		ObservedGeneration: state.Generation,
	}

	result.DesiredClaim, result.ValidationError = createClaim(ctx, state)

	if account == nil {
		result.BindingState = domain.AccountBindingStateMissing
		result.AdoptionState = domain.AccountAdoptionStateMissing
		return result
	}

	result.AccountID = account.GetLabel(v1alpha1.AccountLabelAccountID)
	result.BoundAccountID = state.GetLabel(v1alpha1.AccountExportLabelAccountID)

	if result.BoundAccountID == "" {
		result.BindingState = domain.AccountBindingStateMissing
	} else if result.BoundAccountID != result.AccountID {
		result.BindingState = domain.AccountBindingStateConflict
	} else {
		result.BindingState = domain.AccountBindingStateBound
	}

	adoption, err := findAdoption(ctx, state, account)
	if err != nil {
		result.AdoptionState = domain.AccountAdoptionStateMissing
		result.AdoptionError = err
	} else {
		result.Adoption = adoption

		adoptionGen := adoption.Status.DesiredClaimObservedGeneration
		sameGeneration := adoptionGen != nil && *adoptionGen == state.Generation
		if adoption.Status.Status == "True" && sameGeneration {
			result.AdoptionState = domain.AccountAdoptionStateAdopted
		} else {
			result.AdoptionState = domain.AccountAdoptionStateNotAdopted
			if !sameGeneration {
				result.AdoptionError = fmt.Errorf("generation %d has not been adopted yet", state.Generation)
			} else {
				result.AdoptionError = fmt.Errorf("%s: %s", adoption.Status.Reason, adoption.Status.Message)
			}
		}
	}

	return result
}

func createClaim(ctx context.Context, state *v1alpha1.AccountExport) (*domain.AccountExportClaim, error) {
	exports := &jwt.Exports{}
	for _, r := range state.Spec.Rules {
		exports.Add(mapToJwtExport(r))
	}

	results := &jwt.ValidationResults{}
	exports.Validate(results)

	warnings := results.Warnings()
	if warnings != nil {
		log := logf.FromContext(ctx)
		log.Info("validation warnings for one or more exports: " + strings.Join(warnings, ", "))
	}

	if results.IsBlocking(false) {
		err := fmt.Errorf("validation failed for one or more exports")
		for _, e := range results.Errors() {
			err = errors.Join(err, e)
		}
		return nil, err
	}

	rules := make([]v1alpha1.AccountExportRule, len(state.Spec.Rules))
	for i := range state.Spec.Rules {
		state.Spec.Rules[i].DeepCopyInto(&rules[i])
	}

	return &domain.AccountExportClaim{Rules: rules}, nil
}

func mapToJwtExport(r v1alpha1.AccountExportRule) *jwt.Export {
	// TODO: [#11] Move v1alpha/jwt conversions to domain
	export := &jwt.Export{
		Name:         r.Name,
		Subject:      jwt.Subject(r.Subject),
		Type:         mapExportType(r.Type),
		ResponseType: jwt.ResponseType(r.ResponseType),
	}

	if r.ResponseThreshold != nil {
		export.ResponseThreshold = *r.ResponseThreshold
	}
	if r.AccountTokenPosition != nil {
		export.AccountTokenPosition = *r.AccountTokenPosition
	}
	if r.Advertise != nil {
		export.Advertise = *r.Advertise
	}
	if r.AllowTrace != nil {
		export.AllowTrace = *r.AllowTrace
	}
	if r.Latency != nil {
		export.Latency = &jwt.ServiceLatency{
			Sampling: jwt.SamplingRate(r.Latency.Sampling),
			Results:  jwt.Subject(r.Latency.Results),
		}
	}

	return export
}

func mapExportType(t v1alpha1.ExportType) jwt.ExportType {
	switch t {
	case v1alpha1.Service:
		return jwt.Service
	case v1alpha1.Stream:
		return jwt.Stream
	}

	return jwt.Unknown
}

func findAdoption(ctx context.Context, state *v1alpha1.AccountExport, account *v1alpha1.Account) (*v1alpha1.AccountAdoption, error) {
	if found, adoption := findAdoptionByUID(account, state.UID); found {
		return &adoption, nil
	}

	return nil, fmt.Errorf("account export is not yet processed by account")
}

func findAdoptionByUID(account *v1alpha1.Account, uid types.UID) (bool, v1alpha1.AccountAdoption) {
	if account.Status.Adoptions == nil {
		return false, v1alpha1.AccountAdoption{}
	}

	for _, adoption := range account.Status.Adoptions.Exports {
		if adoption.UID == uid {
			return true, adoption
		}
	}
	return false, v1alpha1.AccountAdoption{}
}

var _ inbound.AccountExportManager = (*AccountExportManager)(nil)
