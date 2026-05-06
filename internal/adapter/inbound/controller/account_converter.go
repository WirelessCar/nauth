package controller

import (
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const GroupNameInline = "inline"

type ResolveAccountIDFn func(accountRef domain.NamespacedName) (accountID nauth.AccountID, err error)

func toNAuthAccountLimits(source *v1alpha1.AccountLimits) *nauth.AccountLimits {
	if source == nil {
		return nil
	}
	return &nauth.AccountLimits{
		Imports:         source.Imports,
		Exports:         source.Exports,
		WildcardExports: source.WildcardExports,
		Conn:            source.Conn,
		LeafNodeConn:    source.LeafNodeConn,
	}
}

func toNAuthJetStreamLimits(source *v1alpha1.JetStreamLimits) *nauth.JetStreamLimits {
	if source == nil {
		return nil
	}
	return &nauth.JetStreamLimits{
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

func toNAuthNatsLimits(source *v1alpha1.NatsLimits) *nauth.NatsLimits {
	if source == nil {
		return nil
	}
	return &nauth.NatsLimits{
		Subs:    source.Subs,
		Data:    source.Data,
		Payload: source.Payload,
	}
}

func toNAuthClusterRef(source *v1alpha1.NatsClusterRef, defaultNamespace string) (*nauth.ClusterRef, error) {
	if source == nil {
		return nil, nil
	}
	namespacedName := domain.NewNamespacedName(source.Namespace, source.Name)
	if namespacedName.Namespace == "" {
		namespacedName.Namespace = defaultNamespace
	}
	if err := namespacedName.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NatsClusterRef: %w", err)
	}
	result, err := nauth.NewClusterRef(namespacedName.String())
	if err != nil {
		return nil, fmt.Errorf("invalid NatsClusterRef: %w", err)
	}

	return &result, nil
}

func toNAuthExportGroup(groupRef nauth.Ref, required bool, sources v1alpha1.Exports) (*nauth.ExportGroup, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	result := nauth.ExportGroup{
		Ref:      groupRef,
		Required: required,
	}
	for i, source := range sources {
		exportType, err := toNAuthExportType(source.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert export type for export at index %d: %w", i, err)
		}
		responseType, err := toNAuthResponseType(source.ResponseType)
		if err != nil {
			return nil, fmt.Errorf("failed to convert response type for export at index %d: %w", i, err)
		}
		result.Exports = append(result.Exports, &nauth.Export{
			Name:                 source.Name,
			Subject:              nauth.Subject(source.Subject),
			Type:                 exportType,
			TokenReq:             source.TokenReq,
			Revocations:          nauth.RevocationList(source.Revocations),
			ResponseType:         responseType,
			ResponseThreshold:    source.ResponseThreshold,
			Latency:              toNAuthServiceLatency(source.Latency),
			AccountTokenPosition: source.AccountTokenPosition,
			Advertise:            source.Advertise,
			AllowTrace:           source.AllowTrace,
		})
	}
	return &result, nil
}

func toNAuthExportGroups(exports *v1alpha1.AccountExportList) (nauth.ExportGroups, []*adoptionRef, error) {
	itemCount := len(exports.Items)
	groups := make(nauth.ExportGroups, 0, itemCount)
	refs := make([]*adoptionRef, 0, itemCount)

	for i, exp := range exports.Items {
		adpRef := newAdoptionRef(exp.ObjectMeta, nil)

		claim := exp.Status.DesiredClaim
		if claim != nil {
			adpRef.ObservedGenerationDesiredClaim = &claim.ObservedGeneration
			nauthExports := make(nauth.Exports, 0, len(claim.Rules))
			for _, rule := range claim.Rules {
				to, err := toNAuthExportFromRule(rule)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to convert export rule for export at index %d in %q: %w", i, exp.Name, err)
				}
				nauthExports = append(nauthExports, to)
			}
			groups = append(groups, &nauth.ExportGroup{
				Ref:     adpRef.Ref,
				Name:    exp.Name,
				Exports: nauthExports,
			})
		}
		refs = append(refs, &adpRef)
	}
	return groups, refs, nil
}

func toNAuthImportGroup(groupRef nauth.Ref, required bool, sources v1alpha1.Imports, reader ResolveAccountIDFn) (*nauth.ImportGroup, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	result := nauth.ImportGroup{
		Ref:      groupRef,
		Required: required,
	}
	for i, source := range sources {
		accountRef := domain.NewNamespacedName(source.AccountRef.Namespace, source.AccountRef.Name)
		var err error
		accountID, err := reader(accountRef)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve account ID for inline import at index %d and account ref %q: %w", i, accountRef, err)
		}
		if accountID == "" {
			return nil, fmt.Errorf("account ID is missing for inline import at index %d and account ref %q", i, accountRef)
		}
		exportType, err := toNAuthExportType(source.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert export type for inline import at index %d: %w", i, err)
		}
		result.Imports = append(result.Imports, &nauth.Import{
			AccountID:    accountID,
			Name:         source.Name,
			Subject:      nauth.Subject(source.Subject),
			LocalSubject: nauth.Subject(source.LocalSubject),
			Type:         exportType,
			Share:        source.Share,
			AllowTrace:   source.AllowTrace,
		})
	}
	return &result, nil
}

func toNAuthImportGroups(imports *v1alpha1.AccountImportList) (nauth.ImportGroups, []*adoptionRef, error) {
	itemCount := len(imports.Items)
	groups := make(nauth.ImportGroups, 0, itemCount)
	refs := make([]*adoptionRef, 0, itemCount)

	for _, imp := range imports.Items {
		adpRef := newAdoptionRef(imp.ObjectMeta, nil)

		claim := imp.Status.DesiredClaim
		if claim != nil {
			adpRef.ObservedGenerationDesiredClaim = &claim.ObservedGeneration
			nauthImports := make(nauth.Imports, 0, len(claim.Rules))
			for i, rule := range claim.Rules {
				to, err := toNAuthImportFromRule(rule)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to convert import rule for import at index %d in %q: %w", i, imp.Name, err)
				}
				nauthImports = append(nauthImports, to)
			}
			groups = append(groups, &nauth.ImportGroup{
				Ref:     adpRef.Ref,
				Name:    imp.Name,
				Imports: nauthImports,
			})
		}
		refs = append(refs, &adpRef)
	}
	return groups, refs, nil
}

func toNAuthServiceLatency(source *v1alpha1.ServiceLatency) *nauth.ServiceLatency {
	if source == nil {
		return nil
	}

	return &nauth.ServiceLatency{
		Sampling: nauth.SamplingRate(source.Sampling),
		Results:  nauth.Subject(source.Results),
	}
}

func toNAuthResponseType(source v1alpha1.ResponseType) (nauth.ResponseType, error) {
	if source == "" {
		return "", nil
	}
	var result nauth.ResponseType
	switch source {
	case v1alpha1.ResponseTypeSingleton:
		result = nauth.ResponseTypeSingleton
	case v1alpha1.ResponseTypeStream:
		result = nauth.ResponseTypeStream
	case v1alpha1.ResponseTypeChunked:
		result = nauth.ResponseTypeChunked
	default:
		return "", fmt.Errorf("unknown v1alpha1.ResponseType: %s", source)
	}
	return result, nil
}

func toNAuthExportType(source v1alpha1.ExportType) (nauth.ExportType, error) {
	if source == "" {
		return "", nil
	}
	var result nauth.ExportType
	switch source {
	case v1alpha1.Stream:
		result = nauth.ExportTypeStream
	case v1alpha1.Service:
		result = nauth.ExportTypeService
	default:
		return "", fmt.Errorf("unknown v1alpha1.ExportType: %s", source)
	}
	return result, nil
}

func toNAuthImportsFromRules(exportAccountID string, sources []v1alpha1.AccountImportRule) (nauth.Imports, error) {
	target := make(nauth.Imports, len(sources))
	for i, rule := range sources {
		exportType, err := toNAuthExportType(rule.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert import rule at index %d: %w", i, err)
		}
		imp := nauth.Import{
			AccountID:    nauth.AccountID(exportAccountID),
			Name:         rule.Name,
			Subject:      nauth.Subject(rule.Subject),
			LocalSubject: nauth.Subject(rule.LocalSubject),
			Type:         exportType,
		}
		if rule.Share != nil {
			imp.Share = *rule.Share
		}
		if rule.AllowTrace != nil {
			imp.AllowTrace = *rule.AllowTrace
		}
		target[i] = &imp
	}
	return target, nil
}

func toAPIAccountImportRuleDerived(source nauth.Import) (*v1alpha1.AccountImportRuleDerived, error) {
	exportType, err := toAPIExportType(source.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to convert export type for import rule: %w", err)
	}
	return &v1alpha1.AccountImportRuleDerived{
		Account: string(source.AccountID),
		AccountImportRule: v1alpha1.AccountImportRule{
			Name:         source.Name,
			Subject:      v1alpha1.Subject(source.Subject),
			LocalSubject: v1alpha1.RenamingSubject(source.LocalSubject),
			Type:         exportType,
			Share:        &source.Share,
			AllowTrace:   &source.AllowTrace,
		},
	}, nil
}

func toNAuthExportFromRule(source v1alpha1.AccountExportRule) (*nauth.Export, error) {
	exportType, err := toNAuthExportType(source.Type)
	if err != nil {
		return nil, err
	}
	responseType, err := toNAuthResponseType(source.ResponseType)
	if err != nil {
		return nil, err
	}
	result := nauth.Export{
		Name:         source.Name,
		Subject:      nauth.Subject(source.Subject),
		Type:         exportType,
		ResponseType: responseType,
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
	return &result, nil
}

func toNAuthImportFromRule(source v1alpha1.AccountImportRuleDerived) (*nauth.Import, error) {
	exportType, err := toNAuthExportType(source.Type)
	if err != nil {
		return nil, err
	}
	result := nauth.Import{
		AccountID:    nauth.AccountID(source.Account),
		Name:         source.Name,
		Subject:      nauth.Subject(source.Subject),
		LocalSubject: nauth.Subject(source.LocalSubject),
		Type:         exportType,
	}
	if source.Share != nil {
		result.Share = *source.Share
	}
	if source.AllowTrace != nil {
		result.AllowTrace = *source.AllowTrace
	}
	return &result, nil
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

func toAPIAccountClaims(claims *nauth.AccountClaims) (*v1alpha1.AccountClaims, error) {
	if claims == nil {
		return nil, nil
	}

	exports, err := toAPIExports(claims.Exports)
	if err != nil {
		return nil, fmt.Errorf("failed to convert exports: %w", err)
	}
	imports, err := toAPIImports(claims.Imports)
	if err != nil {
		return nil, fmt.Errorf("failed to convert imports: %w", err)
	}
	return &v1alpha1.AccountClaims{
		AccountLimits:    toAPIAccountLimits(claims.AccountLimits),
		DisplayName:      claims.DisplayName,
		SigningKeys:      toAPISigningKeys(claims.SigningKeys),
		Exports:          exports,
		Imports:          imports,
		JetStreamEnabled: claims.JetStreamEnabled,
		JetStreamLimits:  toAPIAJetStreamLimits(claims.JetStreamLimits),
		NatsLimits:       toAPINatsLimits(claims.NatsLimits),
	}, nil
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

func toAPIImports(imports nauth.Imports) (v1alpha1.Imports, error) {
	result := make(v1alpha1.Imports, len(imports))
	for i, imp := range imports {
		exportType, err := toAPIExportType(imp.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert export type for import at index %d: %w", i, err)
		}
		result[i] = &v1alpha1.Import{
			Account:      string(imp.AccountID),
			Name:         imp.Name,
			Subject:      v1alpha1.Subject(imp.Subject),
			LocalSubject: v1alpha1.RenamingSubject(imp.LocalSubject),
			Type:         exportType,
			Share:        imp.Share,
			AllowTrace:   imp.AllowTrace,
		}
	}
	return result, nil
}

func toAPIExports(exports nauth.Exports) (v1alpha1.Exports, error) {
	result := make(v1alpha1.Exports, len(exports))
	for i, exp := range exports {
		exportType, err := toAPIExportType(exp.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to convert export type for export at index %d: %w", i, err)
		}
		responseType, err := toAPIResponseType(exp.ResponseType)
		if err != nil {
			return nil, fmt.Errorf("failed to convert response type for export at index %d: %w", i, err)
		}
		export := v1alpha1.Export{
			Name:                 exp.Name,
			Subject:              v1alpha1.Subject(exp.Subject),
			Type:                 exportType,
			ResponseType:         responseType,
			ResponseThreshold:    exp.ResponseThreshold,
			AccountTokenPosition: exp.AccountTokenPosition,
			Advertise:            exp.Advertise,
			AllowTrace:           exp.AllowTrace,
			Latency:              toAPIServiceLatency(exp.Latency),
		}
		result[i] = &export
	}
	return result, nil
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

func toAPIExportType(source nauth.ExportType) (v1alpha1.ExportType, error) {
	if source == "" {
		return "", nil
	}
	var result v1alpha1.ExportType
	switch source {
	case nauth.ExportTypeStream:
		result = v1alpha1.Stream
	case nauth.ExportTypeService:
		result = v1alpha1.Service
	default:
		return "", fmt.Errorf("unsupported nauth.ExportType: %s", source)
	}
	return result, nil
}

func toAPIResponseType(source nauth.ResponseType) (v1alpha1.ResponseType, error) {
	if source == "" {
		return "", nil
	}
	var result v1alpha1.ResponseType
	switch source {
	case nauth.ResponseTypeSingleton:
		result = v1alpha1.ResponseTypeSingleton
	case nauth.ResponseTypeStream:
		result = v1alpha1.ResponseTypeStream
	case nauth.ResponseTypeChunked:
		result = v1alpha1.ResponseTypeChunked
	default:
		return "", fmt.Errorf("unsupported nauth.ResponseType: %s", source)
	}
	return result, nil
}
