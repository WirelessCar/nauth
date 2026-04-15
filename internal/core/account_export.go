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
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountExportManager struct {
}

func NewAccountExportManager() *AccountExportManager {
	return &AccountExportManager{}
}

func (a *AccountExportManager) CreateClaim(ctx context.Context, state *v1alpha1.AccountExport) (*domain.AccountExportClaim, error) {
	exports := &jwt.Exports{}
	for _, r := range state.Spec.Rules {
		exports.Add(mapToJwtExport(r))
	}

	results := &jwt.ValidationResults{}
	// NOTE: validation in jwt.Exports does not detect duplicate subjects, if the type is different.
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

var _ inbound.AccountExportManager = (*AccountExportManager)(nil)
