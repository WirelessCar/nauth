package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"

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
	imports := toJWTImports(group.Imports)
	if err := validateJWTImports(b.claim.Subject, imports); err != nil {
		return err
	}

	result := jwt.Imports(mergeJWTItems(b.claim.Imports, imports, true))
	err := validateJWTImports(b.claim.Subject, result)
	if err != nil {
		return err
	}
	b.claim.Imports = result
	return nil
}

func (b *accountClaimsBuilder) addExportGroup(group nauth.ExportGroup) error {
	exports := toJWTExports(group.Exports)
	if err := validateJWTExports(exports); err != nil {
		return err
	}

	result := jwt.Exports(mergeJWTItems(b.claim.Exports, exports, true))
	err := validateJWTExports(result)
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
		signingKeys := make(nauth.SigningKeys, 0, len(claims.SigningKeys))
		for key := range claims.SigningKeys {
			signingKey := nauth.SigningKey{
				Key: key,
				// TODO: [#140] Map scope
			}
			signingKeys = append(signingKeys, &signingKey)
		}
		// Sort by key to ensure predictable, and human-searchable, order.
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

func validateExports(exports nauth.Exports) error {
	return validateJWTExports(toJWTExports(exports))
}

func validateJWTExports(exports jwt.Exports) error {
	valResults := &jwt.ValidationResults{}
	exports.Validate(valResults)
	if valResults.IsBlocking(false) {
		return errors.Join(valResults.Errors()...)
	}
	return nil
}

func validateImports(importAccountID nauth.AccountID, imports nauth.Imports) error {
	return validateJWTImports(string(importAccountID), toJWTImports(imports))
}

func validateJWTImports(importAccountID string, imports jwt.Imports) error {
	valResults := &jwt.ValidationResults{}
	imports.Validate(importAccountID, valResults)
	if valResults.IsBlocking(false) {
		return errors.Join(valResults.Errors()...)
	}
	return nil
}

func mergeJWTItems[T jwt.Import | jwt.Export](existing []*T, additions []*T, mergeDuplicates bool) []*T {
	result := existing
	for _, a := range additions {
		if a == nil {
			continue
		}
		add := true
		if mergeDuplicates {
			for _, e := range result {
				if e != nil && reflect.DeepEqual(*e, *a) {
					add = false
					break
				}
			}
		}
		if add {
			result = append(result, a)
		}
	}
	return result
}

func toJWTImports(sources nauth.Imports) jwt.Imports {
	result := make(jwt.Imports, len(sources))
	for i, s := range sources {
		result[i] = toJWTImport(*s)
	}
	return result
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

func toJWTExports(exports nauth.Exports) jwt.Exports {
	result := make(jwt.Exports, len(exports))
	for i, s := range exports {
		result[i] = toJWTExport(*s)
	}
	return result
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

func toJWTExportType(source nauth.ExportType) jwt.ExportType {
	switch source {
	case nauth.ExportTypeService:
		return jwt.Service
	case nauth.ExportTypeStream:
		return jwt.Stream
	default:
		return jwt.Unknown
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
