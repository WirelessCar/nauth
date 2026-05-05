package core

import (
	"context"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
)

// TODO: [#11] Move conversions to controller layer

const tmpGroupNameInline = "inline"

type tmpResolveAccountIDFn func(accountRef domain.NamespacedName) (accountID nauth.AccountID, err error)

func tmpToAccountRequest(ctx context.Context, state v1alpha1.Account, accountIDReader outbound.AccountIDReader) (*nauth.AccountRequest, error) {
	accountID := nauth.AccountID(state.GetLabel(v1alpha1.AccountLabelAccountID))
	accountRef := domain.NewNamespacedName(state.Namespace, state.Name)

	accountIDResolver := tmpCachedAccountIDReader(ctx, accountIDReader)

	clusterRef, err := tmpToClusterRef(state.Spec.NatsClusterRef, state.Namespace)
	if err != nil {
		return nil, err
	}

	var exportGroups nauth.ExportGroups
	inlineExportGroup := tmpToNAuthExportGroup(tmpGroupNameInline, true, state.Spec.Exports)
	if inlineExportGroup != nil {
		exportGroups = nauth.ExportGroups{inlineExportGroup}
	}

	var importGroups nauth.ImportGroups
	inlineImportGroup, err := tmpToNAuthImportGroup(tmpGroupNameInline, true, state.Spec.Imports, accountIDResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to convert inline imports: %w", err)
	}
	if inlineImportGroup != nil {
		importGroups = nauth.ImportGroups{inlineImportGroup}
	}

	result := nauth.AccountRequest{
		AccountRef:       accountRef,
		AccountID:        accountID,
		ClaimsHash:       state.Status.ClaimsHash,
		DisplayName:      state.Spec.DisplayName,
		ClusterRef:       clusterRef,
		AccountLimits:    tmpToNAuthAccountLimits(state.Spec.AccountLimits),
		JetStreamEnabled: state.Spec.JetStreamEnabled,
		JetStreamLimits:  tmpToNAuthJetStreamLimits(state.Spec.JetStreamLimits),
		NatsLimits:       tmpToNAuthNatsLimits(state.Spec.NatsLimits),
		ExportGroups:     exportGroups,
		ImportGroups:     importGroups,
	}
	return &result, nil
}

func tmpCachedAccountIDReader(ctx context.Context, accountIDReader outbound.AccountIDReader) tmpResolveAccountIDFn {
	cache := make(map[domain.NamespacedName]nauth.AccountID)
	return func(accountRef domain.NamespacedName) (nauth.AccountID, error) {
		var accountID nauth.AccountID
		var cached bool
		if accountID, cached = cache[accountRef]; !cached {
			var err error
			accountID, err = accountIDReader.GetAccountID(ctx, accountRef)
			if err != nil {
				return "", fmt.Errorf("failed to resolve account ID: %w", err)
			}
			cache[accountRef] = accountID
		}
		if accountID == "" {
			return "", fmt.Errorf("account ID label %s is missing for account %q", v1alpha1.AccountLabelAccountID, accountRef)
		}
		return accountID, nil
	}
}

func tmpToNAuthAccountLimits(source *v1alpha1.AccountLimits) *nauth.AccountLimits {
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

func tmpToNAuthJetStreamLimits(source *v1alpha1.JetStreamLimits) *nauth.JetStreamLimits {
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

func tmpToNAuthNatsLimits(source *v1alpha1.NatsLimits) *nauth.NatsLimits {
	if source == nil {
		return nil
	}
	return &nauth.NatsLimits{
		Subs:    source.Subs,
		Data:    source.Data,
		Payload: source.Payload,
	}
}

func tmpToClusterRef(source *v1alpha1.NatsClusterRef, defaultNamespace string) (*nauth.ClusterRef, error) {
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

func tmpToNAuthExportGroup(groupRef nauth.Ref, required bool, sources v1alpha1.Exports) *nauth.ExportGroup {
	if len(sources) == 0 {
		return nil
	}
	result := nauth.ExportGroup{
		Ref:      groupRef,
		Required: required,
	}
	for _, source := range sources {
		result.Exports = append(result.Exports, &nauth.Export{
			Name:                 source.Name,
			Subject:              nauth.Subject(source.Subject),
			Type:                 tmpToNAuthExportType(source.Type),
			TokenReq:             source.TokenReq,
			Revocations:          nauth.RevocationList(source.Revocations),
			ResponseType:         tmpToNAuthResponseType(source.ResponseType),
			ResponseThreshold:    source.ResponseThreshold,
			Latency:              tmpToNAuthServiceLatency(source.Latency),
			AccountTokenPosition: source.AccountTokenPosition,
			Advertise:            source.Advertise,
			AllowTrace:           source.AllowTrace,
		})
	}
	return &result
}

func tmpToNAuthImportGroup(groupRef nauth.Ref, required bool, sources v1alpha1.Imports, reader tmpResolveAccountIDFn) (*nauth.ImportGroup, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	result := nauth.ImportGroup{
		Ref:      groupRef,
		Required: required,
	}
	for _, source := range sources {
		accountRef := domain.NewNamespacedName(source.AccountRef.Namespace, source.AccountRef.Name)
		var err error
		accountID, err := reader(accountRef)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve account ID for inline import %s: %w", accountRef, err)
		}
		if accountID == "" {
			return nil, fmt.Errorf("account ID is missing for inline import %s", accountRef)
		}
		result.Imports = append(result.Imports, &nauth.Import{
			AccountID:    accountID,
			Name:         source.Name,
			Subject:      nauth.Subject(source.Subject),
			LocalSubject: nauth.Subject(source.LocalSubject),
			Type:         tmpToNAuthExportType(source.Type),
			Share:        source.Share,
			AllowTrace:   source.AllowTrace,
		})
	}
	return &result, nil
}

func tmpToNAuthResponseType(source v1alpha1.ResponseType) nauth.ResponseType {
	switch source {
	case v1alpha1.ResponseTypeSingleton:
		return nauth.ResponseTypeSingleton
	case v1alpha1.ResponseTypeStream:
		return nauth.ResponseTypeStream
	case v1alpha1.ResponseTypeChunked:
		return nauth.ResponseTypeChunked
	default:
		return ""
	}
}

func tmpToNAuthServiceLatency(source *v1alpha1.ServiceLatency) *nauth.ServiceLatency {
	if source == nil {
		return nil
	}

	return &nauth.ServiceLatency{
		Sampling: nauth.SamplingRate(source.Sampling),
		Results:  nauth.Subject(source.Results),
	}
}

func tmpToNAuthExportType(exportType v1alpha1.ExportType) nauth.ExportType {
	switch exportType {
	case v1alpha1.Stream:
		return nauth.ExportTypeStream
	case v1alpha1.Service:
		return nauth.ExportTypeService
	default:
		return nauth.ExportTypeUnknown
	}
}
