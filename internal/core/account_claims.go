package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/nats-io/jwt/v2"
	"k8s.io/apimachinery/pkg/util/json"
)

type accountClaimsBuilder struct {
	jetStreamRequested *bool
	claim              *jwt.AccountClaims
	errs               []error
}

func newAccountClaimsBuilder(
	accountPublicKey string,
	jetStreamEnabled *bool,
) *accountClaimsBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
	if jetStreamEnabled == nil || *jetStreamEnabled {
		// TODO: [#245] Switch to opt-in (enabled != nil && enabled) once we are ready to release a breaking change
		// Initialize claims with unlimited JetStream (to comply with current NAuth behaviour, later this will be due to explicit request)
		claim.Limits.DiskStorage = jwt.NoLimit
		claim.Limits.MemoryStorage = jwt.NoLimit
		claim.Limits.Streams = jwt.NoLimit
		claim.Limits.Consumer = jwt.NoLimit
		claim.Limits.MaxAckPending = jwt.NoLimit
	}

	return &accountClaimsBuilder{
		jetStreamRequested: jetStreamEnabled,
		claim:              claim,
	}
}

func (b *accountClaimsBuilder) displayName(name string) *accountClaimsBuilder {
	b.claim.Name = name
	return b
}

func (b *accountClaimsBuilder) accountLimits(limits *nauth.AccountLimits) *accountClaimsBuilder {
	if limits != nil {
		if limits.Imports != nil {
			b.claim.Limits.Imports = *limits.Imports
		}
		if limits.Exports != nil {
			b.claim.Limits.Exports = *limits.Exports
		}
		if limits.WildcardExports != nil {
			b.claim.Limits.WildcardExports = *limits.WildcardExports
		}
		if limits.Conn != nil {
			b.claim.Limits.Conn = *limits.Conn
		}
		if limits.LeafNodeConn != nil {
			b.claim.Limits.LeafNodeConn = *limits.LeafNodeConn
		}
	}
	return b
}

func (b *accountClaimsBuilder) natsLimits(limits *nauth.NatsLimits) *accountClaimsBuilder {
	if limits != nil {
		if limits.Subs != nil {
			b.claim.Limits.Subs = *limits.Subs
		}
		if limits.Data != nil {
			b.claim.Limits.Data = *limits.Data
		}
		if limits.Payload != nil {
			b.claim.Limits.Payload = *limits.Payload
		}
	}
	return b
}

func (b *accountClaimsBuilder) jetStreamLimits(limits *nauth.JetStreamLimits) *accountClaimsBuilder {
	if limits != nil {
		if limits.MemoryStorage != nil {
			b.claim.Limits.MemoryStorage = *limits.MemoryStorage
		}
		if limits.DiskStorage != nil {
			b.claim.Limits.DiskStorage = *limits.DiskStorage
		}
		if limits.Streams != nil {
			b.claim.Limits.Streams = *limits.Streams
		}
		if limits.Consumer != nil {
			b.claim.Limits.Consumer = *limits.Consumer
		}
		if limits.MaxAckPending != nil {
			b.claim.Limits.MaxAckPending = *limits.MaxAckPending
		}
		if limits.MemoryMaxStreamBytes != nil {
			b.claim.Limits.MemoryMaxStreamBytes = *limits.MemoryMaxStreamBytes
		}
		if limits.DiskMaxStreamBytes != nil {
			b.claim.Limits.DiskMaxStreamBytes = *limits.DiskMaxStreamBytes
		}
		if limits.MaxBytesRequired != nil {
			b.claim.Limits.MaxBytesRequired = *limits.MaxBytesRequired
		}
	}
	return b
}

func (b *accountClaimsBuilder) addImportGroup(group nauth.ImportGroup) error {
	for _, i := range group.Imports {
		jwtImport := toJWTImport(*i)
		result, err := mergeJWTImports(b.claim.Subject, b.claim.Imports, jwt.Imports{jwtImport})
		if err != nil {
			return fmt.Errorf("failed to add import %q from group %q: %w", i.Name, group.Name, err)
		}
		b.claim.Imports = result
	}
	return nil
}

func (b *accountClaimsBuilder) addExportGroup(group nauth.ExportGroup) error {
	jwtExports := make(jwt.Exports, len(group.Exports))
	for i, e := range group.Exports {
		jwtExports[i] = toJWTExport(*e)
	}
	result, err := mergeExports(b.claim.Exports, jwtExports)
	if err != nil {
		return err
	}
	b.claim.Exports = result
	return nil
}

func (b *accountClaimsBuilder) signingKey(signingKey string) *accountClaimsBuilder {
	b.claim.SigningKeys.Add(signingKey)
	return b
}

func (b *accountClaimsBuilder) build() (*jwt.AccountClaims, error) {
	if err := validateJetStreamLimits(b.jetStreamRequested, b.claim.Limits); err != nil {
		b.errs = append(b.errs, err)
	}
	if err := errors.Join(b.errs...); err != nil {
		return nil, err
	}

	return b.claim, nil
}

func validateJetStreamLimits(jetStreamExpected *bool, limits jwt.OperatorLimits) error {
	// Note: Those error messages must be validated in tests as this is a very implicit legacy behavior in NATS JWT lib
	if jetStreamExpected != nil {
		if *jetStreamExpected && !limits.IsJSEnabled() {
			return fmt.Errorf("ambiguous JetStream config; requested to be enabled, but no allowed MemoryStorage or DiskStorage supplied")
		}
		if !*jetStreamExpected && limits.IsJSEnabled() {
			return fmt.Errorf("ambiguous JetStream config; requested to be disabled, but supplied MemoryStorage and/or DiskStorage would implicitly enables it")
		}
	}
	return nil
}

func hashSignedAccountJWTClaims(accountJWT string) (string, error) {
	claims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return "", fmt.Errorf("failed to decode account JWT claims for hashing: %w", err)
	}
	// Exclude unstable JWT metadata so equivalent account content hashes the same across reconciles.
	claims.IssuedAt = 0
	claims.ID = ""

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func toPointerDefaultNil[V int64 | bool](value V, defaultValue V) *V {
	if value != defaultValue {
		return &value
	}
	return nil
}

func convertNatsAccountClaims(claims *jwt.AccountClaims) nauth.AccountClaims {
	if claims == nil {
		return nauth.AccountClaims{}
	}

	claimsDefaults := jwt.NewAccountClaims("N/A")
	out := nauth.AccountClaims{}
	out.DisplayName = claims.Name

	jetStreamEnabled := claims.Limits.IsJSEnabled()
	out.JetStreamEnabled = &jetStreamEnabled

	// AccountLimits
	{
		source := claims.Limits.AccountLimits
		if !source.IsUnlimited() {
			defaults := claimsDefaults.Limits.AccountLimits
			out.AccountLimits = &nauth.AccountLimits{}
			out.AccountLimits.Imports = toPointerDefaultNil(source.Imports, defaults.Imports)
			out.AccountLimits.Exports = toPointerDefaultNil(source.Exports, defaults.Exports)
			out.AccountLimits.WildcardExports = toPointerDefaultNil(source.WildcardExports, defaults.WildcardExports)
			out.AccountLimits.Conn = toPointerDefaultNil(source.Conn, defaults.Conn)
			out.AccountLimits.LeafNodeConn = toPointerDefaultNil(source.LeafNodeConn, defaults.LeafNodeConn)
		}
	}

	// NatsLimits
	{
		source := claims.Limits.NatsLimits
		if !source.IsUnlimited() {
			defaults := claimsDefaults.Limits.NatsLimits
			out.NatsLimits = &nauth.NatsLimits{}
			out.NatsLimits.Data = toPointerDefaultNil(source.Data, defaults.Data)
			out.NatsLimits.Subs = toPointerDefaultNil(source.Subs, defaults.Subs)
			out.NatsLimits.Payload = toPointerDefaultNil(source.Payload, defaults.Payload)
		}
	}

	// JetStreamLimits
	{
		source := claims.Limits.JetStreamLimits
		defaults := claimsDefaults.Limits.JetStreamLimits
		if source != defaults {
			out.JetStreamLimits = &nauth.JetStreamLimits{}
			out.JetStreamLimits.MemoryStorage = toPointerDefaultNil(source.MemoryStorage, defaults.MemoryStorage)
			out.JetStreamLimits.DiskStorage = toPointerDefaultNil(source.DiskStorage, defaults.DiskStorage)
			out.JetStreamLimits.Streams = toPointerDefaultNil(source.Streams, defaults.Streams)
			out.JetStreamLimits.Consumer = toPointerDefaultNil(source.Consumer, defaults.Consumer)
			out.JetStreamLimits.MaxAckPending = toPointerDefaultNil(source.MaxAckPending, defaults.MaxAckPending)
			out.JetStreamLimits.MemoryMaxStreamBytes = toPointerDefaultNil(source.MemoryMaxStreamBytes, defaults.MemoryMaxStreamBytes)
			out.JetStreamLimits.DiskMaxStreamBytes = toPointerDefaultNil(source.DiskMaxStreamBytes, defaults.DiskMaxStreamBytes)
			out.JetStreamLimits.MaxBytesRequired = toPointerDefaultNil(source.MaxBytesRequired, defaults.MaxBytesRequired)
		}
	}

	// Signing Keys
	if len(claims.SigningKeys) > 0 {
		signingKeys := make(v1alpha1.SigningKeys, 0, len(claims.SigningKeys))
		for key := range claims.SigningKeys {
			signingKey := v1alpha1.SigningKey{
				Key: key,
			}
			signingKeys = append(signingKeys, &signingKey)
			// TODO: [https://github.com/WirelessCar/nauth/issues/140] Populate optional *UserScope
		}
		// Sort by key to ensure predictable, and human searchable, order.
		sort.Slice(signingKeys, func(i, j int) bool {
			return signingKeys[i].Key < signingKeys[j].Key
		})
		out.SigningKeys = signingKeys
	}

	// Exports
	if len(claims.Exports) > 0 {
		exports := make(nauth.Exports, 0, len(claims.Exports))
		for _, e := range claims.Exports {
			if e == nil {
				continue
			}
			exports = append(exports, toNAuthExport(*e))
		}
		out.Exports = exports
	}

	// Imports
	if len(claims.Imports) > 0 {
		imports := make(nauth.Imports, 0, len(claims.Imports))
		for _, i := range claims.Imports {
			if i == nil {
				continue
			}
			imports = append(imports, toNAuthImport(*i))
		}
		out.Imports = imports
	}

	return out
}

// Helpers

func mergeExports(existing jwt.Exports, extras jwt.Exports) (jwt.Exports, error) {
	tmpExports := existing
	appendIfMissing := func(haystack jwt.Exports, needle jwt.Export) jwt.Exports {
		for _, e := range haystack {
			if e != nil && reflect.DeepEqual(*e, needle) {
				return haystack
			}
		}
		return append(haystack, &needle)
	}
	for _, e := range extras {
		tmpExports = appendIfMissing(tmpExports, *e)
	}
	valResults := &jwt.ValidationResults{}
	tmpExports.Validate(valResults)
	validationErrors := valResults.Errors()
	if len(validationErrors) != 0 {
		return existing, errors.Join(validationErrors...)
	}
	return tmpExports, nil
}

func validateImports(importAccountID string, imports nauth.Imports) error {
	_, err := mergeImports(importAccountID, nil, imports)
	return err
}

func mergeImports(importAccountID string, existing jwt.Imports, imports nauth.Imports) (jwt.Imports, error) {
	jwtImports := make(jwt.Imports, len(imports))
	for i, imp := range imports {
		jwtImports[i] = toJWTImport(*imp)
	}
	result, err := mergeJWTImports(importAccountID, existing, jwtImports)
	if err != nil {
		return existing, err
	}
	return result, nil
}

func toJWTImport(source nauth.Import) *jwt.Import {
	return &jwt.Import{
		Account:      string(source.AccountID),
		Name:         source.Name,
		Subject:      jwt.Subject(source.Subject),
		Type:         toJWTExportType(source.Type),
		LocalSubject: jwt.RenamingSubject(source.LocalSubject),
		Share:        source.Share,
		AllowTrace:   source.AllowTrace,
	}
}

func toJWTExport(source nauth.Export) *jwt.Export {
	return &jwt.Export{
		Name:                 source.Name,
		Subject:              jwt.Subject(source.Subject),
		Type:                 toJWTExportType(source.Type),
		TokenReq:             source.TokenReq,
		Revocations:          jwt.RevocationList(source.Revocations),
		ResponseType:         toJWTResponseType(source.ResponseType),
		ResponseThreshold:    source.ResponseThreshold,
		AccountTokenPosition: source.AccountTokenPosition,
		Advertise:            source.Advertise,
		AllowTrace:           source.AllowTrace,
		Latency:              toJWTServiceLatency(source.Latency),
	}
}

func toJWTResponseType(source nauth.ResponseType) jwt.ResponseType {
	switch source {
	case nauth.ResponseTypeSingleton:
		return jwt.ResponseTypeSingleton
	case nauth.ResponseTypeChunked:
		return jwt.ResponseTypeChunked
	case nauth.ResponseTypeStream:
		return jwt.ResponseTypeStream
	default:
		return ""
	}
}

func toNAuthImport(source jwt.Import) *nauth.Import {
	return &nauth.Import{
		AccountID:    nauth.AccountID(source.Account),
		Name:         source.Name,
		Subject:      nauth.Subject(source.Subject),
		Type:         toNAuthExportType(source.Type),
		LocalSubject: nauth.Subject(source.LocalSubject),
		Share:        source.Share,
		AllowTrace:   source.AllowTrace,
	}
}

func toNAuthExport(source jwt.Export) *nauth.Export {
	return &nauth.Export{
		Name:                 source.Name,
		Subject:              nauth.Subject(source.Subject),
		Type:                 toNAuthExportType(source.Type),
		TokenReq:             source.TokenReq,
		Revocations:          nauth.RevocationList(source.Revocations),
		ResponseType:         toNAuthResponseType(source.ResponseType),
		ResponseThreshold:    source.ResponseThreshold,
		AccountTokenPosition: source.AccountTokenPosition,
		Latency:              toNAuthServiceLatency(source.Latency),
		Advertise:            source.Advertise,
		AllowTrace:           source.AllowTrace,
	}
}

func toNAuthResponseType(source jwt.ResponseType) nauth.ResponseType {
	switch source {
	case jwt.ResponseTypeSingleton:
		return nauth.ResponseTypeSingleton
	case jwt.ResponseTypeChunked:
		return nauth.ResponseTypeChunked
	case jwt.ResponseTypeStream:
		return nauth.ResponseTypeStream
	default:
		return ""
	}
}

func toNAuthServiceLatency(source *jwt.ServiceLatency) *nauth.ServiceLatency {
	if source == nil {
		return nil
	}
	return &nauth.ServiceLatency{
		Sampling: nauth.SamplingRate(source.Sampling),
		Results:  nauth.Subject(source.Results),
	}
}

func mergeJWTImports(importAccountID string, existing jwt.Imports, extras jwt.Imports) (jwt.Imports, error) {
	tmpResult := existing
	appendIfMissing := func(haystack jwt.Imports, needle jwt.Import) jwt.Imports {
		for _, e := range haystack {
			if e != nil && reflect.DeepEqual(*e, needle) {
				return haystack
			}
		}
		return append(haystack, &needle)
	}
	for _, e := range extras {
		if e != nil {
			tmpResult = appendIfMissing(tmpResult, *e)
		}
	}
	valResults := &jwt.ValidationResults{}
	tmpResult.Validate(importAccountID, valResults)
	validationErrors := valResults.Errors()
	if len(validationErrors) != 0 {
		return existing, errors.Join(validationErrors...)
	}
	return tmpResult, nil
}

func toJWTExportType(source nauth.ExportType) jwt.ExportType {
	switch source {
	case nauth.ExportTypeService:
		return jwt.Service
	case nauth.ExportTypeStream:
		return jwt.Stream
	default:
		return jwt.Stream
	}
}

func toJWTServiceLatency(source *nauth.ServiceLatency) *jwt.ServiceLatency {
	if source == nil {
		return nil
	}
	return &jwt.ServiceLatency{
		Sampling: jwt.SamplingRate(source.Sampling),
		Results:  jwt.Subject(source.Results),
	}
}

func toNAuthExportType(source jwt.ExportType) nauth.ExportType {
	switch source {
	case jwt.Service:
		return nauth.ExportTypeService
	case jwt.Stream:
		return nauth.ExportTypeStream
	default:
		return nauth.ExportTypeUnknown
	}
}
