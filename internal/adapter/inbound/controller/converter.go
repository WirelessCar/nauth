package controller

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

func toNAuthImports(exportAccountID string, sources []v1alpha1.AccountImportRule) nauth.Imports {
	target := make(nauth.Imports, len(sources))
	for i, rule := range sources {
		imp := nauth.Import{
			AccountID:    nauth.AccountID(exportAccountID),
			Name:         rule.Name,
			Subject:      nauth.Subject(rule.Subject),
			LocalSubject: nauth.Subject(rule.LocalSubject),
			Type:         toNAuthExportType(rule.Type),
		}
		if rule.Share != nil {
			imp.Share = *rule.Share
		}
		if rule.AllowTrace != nil {
			imp.AllowTrace = *rule.AllowTrace
		}
		target[i] = &imp
	}
	return target
}

func toAPIAccountImportRuleDerived(source nauth.Import) v1alpha1.AccountImportRuleDerived {
	return v1alpha1.AccountImportRuleDerived{
		Account: string(source.AccountID),
		AccountImportRule: v1alpha1.AccountImportRule{
			Name:         source.Name,
			Subject:      v1alpha1.Subject(source.Subject),
			LocalSubject: v1alpha1.RenamingSubject(source.LocalSubject),
			Type:         toAPIExportType(source.Type),
			Share:        &source.Share,
			AllowTrace:   &source.AllowTrace,
		},
	}
}

func toNAuthExportFromRule(source v1alpha1.AccountExportRule) *nauth.Export {
	result := nauth.Export{
		Name:         source.Name,
		Subject:      nauth.Subject(source.Subject),
		Type:         toNAuthExportType(source.Type),
		ResponseType: toNAuthResponseType(source.ResponseType),
	}
	if source.ResponseThreshold != nil {
		result.ResponseThreshold = *source.ResponseThreshold
	}
	if source.AccountTokenPosition != nil {
		result.AccountTokenPosition = *source.AccountTokenPosition
	}
	if source.Advertise != nil {
		result.Advertise = *source.Advertise
	}
	if source.AllowTrace != nil {
		result.AllowTrace = *source.AllowTrace
	}
	if source.Latency != nil {
		result.Latency = &nauth.ServiceLatency{
			Sampling: nauth.SamplingRate(source.Latency.Sampling),
			Results:  nauth.Subject(source.Latency.Results),
		}
	}
	return &result
}

func toNAuthExportType(source v1alpha1.ExportType) nauth.ExportType {
	switch source {
	case v1alpha1.Stream:
		return nauth.ExportTypeStream
	case v1alpha1.Service:
		return nauth.ExportTypeService
	default:
		return nauth.ExportTypeUnknown
	}
}

func toNAuthResponseType(responseType v1alpha1.ResponseType) nauth.ResponseType {
	switch responseType {
	case v1alpha1.ResponseTypeStream:
		return nauth.ResponseTypeStream
	case v1alpha1.ResponseTypeSingleton:
		return nauth.ResponseTypeSingleton
	case v1alpha1.ResponseTypeChunked:
		return nauth.ResponseTypeChunked
	default:
		return ""
	}
}
