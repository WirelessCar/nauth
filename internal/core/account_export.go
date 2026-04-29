package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/nats-io/jwt/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountExportManager struct {
}

func NewAccountExportManager() *AccountExportManager {
	return &AccountExportManager{}
}

func (a *AccountExportManager) ValidateRules(ctx context.Context, rules nauth.ExportRules) error {
	exports := &jwt.Exports{}
	for _, r := range rules {
		exports.Add(mapToJwtExport(r))
	}

	return validateRules(ctx, exports)
}

func mapToJwtExport(r nauth.ExportRule) *jwt.Export {
	export := &jwt.Export{
		Name:                 r.Name,
		Subject:              jwt.Subject(r.Subject),
		Type:                 mapExportType(r.Type),
		ResponseType:         jwt.ResponseType(r.ResponseType),
		AccountTokenPosition: r.AccountTokenPosition,
		Advertise:            r.Advertise,
		AllowTrace:           r.AllowTrace,
		ResponseThreshold:    r.ResponseThreshold,
	}
	if r.Latency != nil {
		export.Latency = &jwt.ServiceLatency{
			Sampling: jwt.SamplingRate(r.Latency.Sampling),
			Results:  jwt.Subject(r.Latency.Results),
		}
	}

	return export
}

func mapExportType(t nauth.ExportType) jwt.ExportType {
	switch t {
	case nauth.ExportTypeService:
		return jwt.Service
	case nauth.ExportTypeStream:
		return jwt.Stream
	}

	return jwt.Unknown
}

func validateRules(ctx context.Context, exports *jwt.Exports) error {
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
		return err
	}
	return nil
}

var _ inbound.AccountExportManager = (*AccountExportManager)(nil)
