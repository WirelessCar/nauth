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
