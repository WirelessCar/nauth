package controller

import (
	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func toNAuthImportFromRule(source v1alpha1.AccountImportRuleDerived) *nauth.Import {
	result := nauth.Import{
		AccountID:    nauth.AccountID(source.Account),
		Name:         source.Name,
		Subject:      nauth.Subject(source.Subject),
		LocalSubject: nauth.Subject(source.LocalSubject),
		Type:         toNAuthExportType(source.Type),
	}
	if source.Share != nil {
		result.Share = *source.Share
	}
	if source.AllowTrace != nil {
		result.AllowTrace = *source.AllowTrace
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

func toAPIAdoptions(adoptions *nauth.AccountAdoptions, adoptionRefs accountAdoptionRefs) *v1alpha1.AccountAdoptions {
	if adoptions == nil {
		return nil
	}

	return &v1alpha1.AccountAdoptions{
		Exports: toAccountAdoptions(adoptionRefs.exports, adoptions.Exports),
		Imports: toAccountAdoptions(adoptionRefs.imports, adoptions.Imports),
	}
}

func toAccountAdoptions(refs []*adoptionRef, adoptionResults *nauth.AdoptionResults) []v1alpha1.AccountAdoption {
	accountAdoptions := make([]v1alpha1.AccountAdoption, 0, len(refs))

	for _, adpRef := range refs {
		var status v1alpha1.AccountAdoptionStatus
		adpResult := adoptionResults.Get(adpRef.Ref)
		if adpResult != nil && adpResult.IsSuccessful() {
			status = v1alpha1.AccountAdoptionStatus{
				Status:                         metav1.ConditionTrue,
				Reason:                         conditionReasonOK,
				Message:                        conditionMessageAdopted,
				DesiredClaimObservedGeneration: adpRef.ObservedGenerationDesiredClaim,
			}
		} else {
			status = v1alpha1.AccountAdoptionStatus{
				Status:                         metav1.ConditionFalse,
				Reason:                         conditionReasonNOK,
				DesiredClaimObservedGeneration: adpRef.ObservedGenerationDesiredClaim,
			}
			if adpResult == nil {
				if adpRef.ObservedGenerationDesiredClaim == nil {
					status.Message = "Adoption pending: no desired claim"
				} else {
					status.Message = "WARN: No adoption result reported"
				}
			} else if failure := adpResult.Failure; failure != "" {
				status.Reason = string(failure)
				status.Message = adpResult.Message
			} else {
				status.Message = conditionMessageAdopted
			}
		}
		accountAdoptions = append(accountAdoptions, v1alpha1.AccountAdoption{
			Name:               adpRef.Name,
			UID:                adpRef.UID,
			ObservedGeneration: adpRef.ObservedGeneration,
			Status:             status,
		})
	}
	return accountAdoptions
}

func toAPIAccountClaims(claims *nauth.AccountClaims) *v1alpha1.AccountClaims {
	if claims == nil {
		return nil
	}

	return &v1alpha1.AccountClaims{
		AccountLimits:    toAPIAccountLimits(claims.AccountLimits),
		DisplayName:      claims.DisplayName,
		SigningKeys:      toAPISigningKeys(claims.SigningKeys),
		Exports:          toAPIExports(claims.Exports),
		Imports:          toAPIImports(claims.Imports),
		JetStreamEnabled: claims.JetStreamEnabled,
		JetStreamLimits:  toAPIAJetStreamLimits(claims.JetStreamLimits),
		NatsLimits:       toAPINatsLimits(claims.NatsLimits),
	}
}

func toAPIAccountLimits(source *nauth.AccountLimits) *v1alpha1.AccountLimits {
	if source == nil {
		return nil
	}

	return &v1alpha1.AccountLimits{
		Imports:         source.Imports,
		Exports:         source.Exports,
		WildcardExports: source.WildcardExports,
		Conn:            source.Conn,
		LeafNodeConn:    source.LeafNodeConn,
	}
}

func toAPIAJetStreamLimits(source *nauth.JetStreamLimits) *v1alpha1.JetStreamLimits {
	if source == nil {
		return nil
	}

	return &v1alpha1.JetStreamLimits{
		MemoryStorage:        source.MemoryStorage,
		DiskStorage:          source.DiskStorage,
		Streams:              source.Streams,
		Consumer:             source.Consumer,
		MaxAckPending:        source.MaxAckPending,
		MemoryMaxStreamBytes: source.MemoryMaxStreamBytes,
		DiskMaxStreamBytes:   source.DiskMaxStreamBytes,
		MaxBytesRequired:     source.MaxBytesRequired,
	}
}

func toAPINatsLimits(source *nauth.NatsLimits) *v1alpha1.NatsLimits {
	if source == nil {
		return nil
	}

	return &v1alpha1.NatsLimits{
		Subs:    source.Subs,
		Data:    source.Data,
		Payload: source.Payload,
	}
}

func toAPISigningKeys(keys nauth.SigningKeys) v1alpha1.SigningKeys {
	result := make(v1alpha1.SigningKeys, len(keys))
	for i, key := range keys {
		result[i] = &v1alpha1.SigningKey{
			Key: key.Key,
			// TODO: [#140] map Signing Key scope
		}
	}
	return result
}

func toAPIImports(imports nauth.Imports) v1alpha1.Imports {
	result := make(v1alpha1.Imports, len(imports))
	for i, imp := range imports {
		result[i] = &v1alpha1.Import{
			Account:      string(imp.AccountID),
			Name:         imp.Name,
			Subject:      v1alpha1.Subject(imp.Subject),
			LocalSubject: v1alpha1.RenamingSubject(imp.LocalSubject),
			Type:         toAPIExportType(imp.Type),
			Share:        imp.Share,
			AllowTrace:   imp.AllowTrace,
		}
	}
	return result
}

func toAPIExports(exports nauth.Exports) v1alpha1.Exports {
	result := make(v1alpha1.Exports, len(exports))
	for i, exp := range exports {
		export := v1alpha1.Export{
			Name:                 exp.Name,
			Subject:              v1alpha1.Subject(exp.Subject),
			Type:                 toAPIExportType(exp.Type),
			ResponseType:         v1alpha1.ResponseType(exp.ResponseType),
			ResponseThreshold:    exp.ResponseThreshold,
			AccountTokenPosition: exp.AccountTokenPosition,
			Advertise:            exp.Advertise,
			AllowTrace:           exp.AllowTrace,
			Latency:              toAPIServiceLatency(exp.Latency),
		}
		result[i] = &export
	}
	return result
}

func toAPIServiceLatency(latency *nauth.ServiceLatency) *v1alpha1.ServiceLatency {
	if latency == nil {
		return nil
	}

	return &v1alpha1.ServiceLatency{
		Sampling: v1alpha1.SamplingRate(latency.Sampling),
		Results:  v1alpha1.Subject(latency.Results),
	}
}

func toAPIExportType(exportType nauth.ExportType) v1alpha1.ExportType {
	switch exportType {
	case nauth.ExportTypeStream:
		return v1alpha1.Stream
	case nauth.ExportTypeService:
		return v1alpha1.Service
	default:
		return v1alpha1.Stream
	}
}
