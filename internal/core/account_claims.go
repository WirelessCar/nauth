package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	"k8s.io/apimachinery/pkg/util/json"
)

type resolveAccountIDFn func(accountRef domain.NamespacedName) (accountID string, err error)

type accountClaimsBuilder struct {
	jetStreamRequested *bool
	claim              *jwt.AccountClaims
	errs               []error
}

func newAccountClaimsBuilder(
	displayName string,
	accountPublicKey string,
	jetStreamEnabled *bool,
) *accountClaimsBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
	claim.Name = displayName
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

func (b *accountClaimsBuilder) accountLimits(limits *v1alpha1.AccountLimits) *accountClaimsBuilder {
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

func (b *accountClaimsBuilder) natsLimits(limits *v1alpha1.NatsLimits) *accountClaimsBuilder {
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

func (b *accountClaimsBuilder) jetStreamLimits(limits *v1alpha1.JetStreamLimits) *accountClaimsBuilder {
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

func (b *accountClaimsBuilder) exports(exports v1alpha1.Exports) *accountClaimsBuilder {
	for _, export := range exports {
		exportClaim := &jwt.Export{
			Name:                 export.Name,
			Subject:              jwt.Subject(export.Subject),
			Type:                 toJWTExportType(export.Type),
			TokenReq:             export.TokenReq,
			Revocations:          jwt.RevocationList(export.Revocations),
			ResponseType:         jwt.ResponseType(export.ResponseType),
			ResponseThreshold:    export.ResponseThreshold,
			AccountTokenPosition: export.AccountTokenPosition,
			Advertise:            export.Advertise,
			AllowTrace:           export.AllowTrace,
		}
		if export.Latency != nil {
			exportClaim.Latency = toJWTServiceLatency(*export.Latency)
		}
		b.claim.Exports.Add(exportClaim)
	}
	return b
}

func (b *accountClaimsBuilder) imports(imports v1alpha1.Imports, resolveAccountIDFn resolveAccountIDFn) *accountClaimsBuilder {
	for _, imp := range imports {
		accountRef := domain.NewNamespacedName(imp.AccountRef.Namespace, imp.AccountRef.Name)
		exportAccountID, err := resolveAccountIDFn(accountRef)
		if err != nil {
			b.errs = append(b.errs, fmt.Errorf("failed to resolve account ID for import %q (account: %q): %w",
				imp.Name,
				accountRef,
				err))
		} else {
			jwtImport := &jwt.Import{
				Name:         imp.Name,
				Subject:      jwt.Subject(imp.Subject),
				Type:         jwt.ExportType(imp.Type.ToInt()),
				Account:      exportAccountID,
				LocalSubject: jwt.RenamingSubject(imp.LocalSubject),
				Share:        imp.Share,
				AllowTrace:   imp.AllowTrace,
			}
			result, mergeErr := mergeImports(b.claim.Subject, b.claim.Imports, jwt.Imports{jwtImport})
			if mergeErr != nil {
				b.errs = append(b.errs, fmt.Errorf("failed to add import %q: %w", imp.Name, mergeErr))
			}
			b.claim.Imports = result
		}
	}
	return b
}

func (b *accountClaimsBuilder) addExportRuleGroup(rules []v1alpha1.AccountExportRule) error {
	jwtExports := make(jwt.Exports, len(rules))
	for i, rule := range rules {
		jwtExport := jwt.Export{
			Name:         rule.Name,
			Subject:      jwt.Subject(rule.Subject),
			Type:         toJWTExportType(rule.Type),
			ResponseType: jwt.ResponseType(rule.ResponseType),
		}
		if rule.ResponseThreshold != nil {
			jwtExport.ResponseThreshold = *rule.ResponseThreshold
		}
		if rule.Latency != nil {
			jwtExport.Latency = toJWTServiceLatency(*rule.Latency)
		}
		if rule.AccountTokenPosition != nil {
			jwtExport.AccountTokenPosition = *rule.AccountTokenPosition
		}
		if rule.Advertise != nil {
			jwtExport.Advertise = *rule.Advertise
		}
		if rule.AllowTrace != nil {
			jwtExport.AllowTrace = *rule.AllowTrace
		}
		jwtExports[i] = &jwtExport
	}
	result, err := mergeExports(b.claim.Exports, jwtExports)
	if err != nil {
		return fmt.Errorf("failed to append export rule group: %w", err)
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

func toPtrDefNil[V int64 | bool](value V, defaultValue V) *V {
	if value != defaultValue {
		return &value
	}
	return nil
}

func convertNatsAccountClaims(claims *jwt.AccountClaims) v1alpha1.AccountClaims {
	if claims == nil {
		return v1alpha1.AccountClaims{}
	}

	claimsDefaults := jwt.NewAccountClaims("N/A")
	out := v1alpha1.AccountClaims{}
	out.DisplayName = claims.Name

	jetStreamEnabled := claims.Limits.IsJSEnabled()
	out.JetStreamEnabled = &jetStreamEnabled

	// AccountLimits
	{
		source := claims.Limits.AccountLimits
		if !source.IsUnlimited() {
			defaults := claimsDefaults.Limits.AccountLimits
			out.AccountLimits = &v1alpha1.AccountLimits{}
			out.AccountLimits.Imports = toPtrDefNil(source.Imports, defaults.Imports)
			out.AccountLimits.Exports = toPtrDefNil(source.Exports, defaults.Exports)
			out.AccountLimits.WildcardExports = toPtrDefNil(source.WildcardExports, defaults.WildcardExports)
			out.AccountLimits.Conn = toPtrDefNil(source.Conn, defaults.Conn)
			out.AccountLimits.LeafNodeConn = toPtrDefNil(source.LeafNodeConn, defaults.LeafNodeConn)
		}
	}

	// NatsLimits
	{
		source := claims.Limits.NatsLimits
		if !source.IsUnlimited() {
			defaults := claimsDefaults.Limits.NatsLimits
			out.NatsLimits = &v1alpha1.NatsLimits{}
			out.NatsLimits.Data = toPtrDefNil(source.Data, defaults.Data)
			out.NatsLimits.Subs = toPtrDefNil(source.Subs, defaults.Subs)
			out.NatsLimits.Payload = toPtrDefNil(source.Payload, defaults.Payload)
		}
	}

	// JetStreamLimits
	{
		source := claims.Limits.JetStreamLimits
		defaults := claimsDefaults.Limits.JetStreamLimits
		if source != defaults {
			out.JetStreamLimits = &v1alpha1.JetStreamLimits{}
			out.JetStreamLimits.MemoryStorage = toPtrDefNil(source.MemoryStorage, defaults.MemoryStorage)
			out.JetStreamLimits.DiskStorage = toPtrDefNil(source.DiskStorage, defaults.DiskStorage)
			out.JetStreamLimits.Streams = toPtrDefNil(source.Streams, defaults.Streams)
			out.JetStreamLimits.Consumer = toPtrDefNil(source.Consumer, defaults.Consumer)
			out.JetStreamLimits.MaxAckPending = toPtrDefNil(source.MaxAckPending, defaults.MaxAckPending)
			out.JetStreamLimits.MemoryMaxStreamBytes = toPtrDefNil(source.MemoryMaxStreamBytes, defaults.MemoryMaxStreamBytes)
			out.JetStreamLimits.DiskMaxStreamBytes = toPtrDefNil(source.DiskMaxStreamBytes, defaults.DiskMaxStreamBytes)
			out.JetStreamLimits.MaxBytesRequired = toPtrDefNil(source.MaxBytesRequired, defaults.MaxBytesRequired)
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
		exports := make(v1alpha1.Exports, 0, len(claims.Exports))
		for _, e := range claims.Exports {
			if e == nil {
				continue
			}
			var et v1alpha1.ExportType
			switch e.Type {
			case jwt.Stream:
				et = v1alpha1.Stream
			case jwt.Service:
				et = v1alpha1.Service
			default:
				et = v1alpha1.Stream
			}

			var latency *v1alpha1.ServiceLatency
			if e.Latency != nil {
				latency = &v1alpha1.ServiceLatency{
					Sampling: v1alpha1.SamplingRate(e.Latency.Sampling),
					Results:  v1alpha1.Subject(e.Latency.Results),
				}
			}

			export := &v1alpha1.Export{
				Name:                 e.Name,
				Subject:              v1alpha1.Subject(e.Subject),
				Type:                 et,
				TokenReq:             e.TokenReq,
				Revocations:          v1alpha1.RevocationList(e.Revocations),
				ResponseType:         v1alpha1.ResponseType(e.ResponseType),
				ResponseThreshold:    e.ResponseThreshold,
				Latency:              latency,
				AccountTokenPosition: e.AccountTokenPosition,
				Advertise:            e.Advertise,
				AllowTrace:           e.AllowTrace,
			}
			exports = append(exports, export)
		}
		out.Exports = exports
	}

	// Imports
	if len(claims.Imports) > 0 {
		imports := make(v1alpha1.Imports, 0, len(claims.Imports))
		for _, i := range claims.Imports {
			if i == nil {
				continue
			}
			var it v1alpha1.ExportType
			switch i.Type {
			case jwt.Stream:
				it = v1alpha1.Stream
			case jwt.Service:
				it = v1alpha1.Service
			default:
				it = v1alpha1.Stream
			}
			imp := &v1alpha1.Import{
				Name:         i.Name,
				Subject:      v1alpha1.Subject(i.Subject),
				Account:      i.Account,
				LocalSubject: v1alpha1.RenamingSubject(i.LocalSubject),
				Type:         it,
				Share:        i.Share,
				AllowTrace:   i.AllowTrace,
			}
			imports = append(imports, imp)
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

func validateImportRules(importAccountID string, rules []v1alpha1.AccountImportRuleDerived) error {
	_, err := mergeImportRules(importAccountID, nil, rules)
	return err
}

func mergeImportRules(importAccountID string, existing jwt.Imports, rules []v1alpha1.AccountImportRuleDerived) (jwt.Imports, error) {
	jwtImports := make(jwt.Imports, len(rules))
	for i, rule := range rules {
		jwtImport := jwt.Import{
			Account:      rule.Account,
			Name:         rule.Name,
			Subject:      jwt.Subject(rule.Subject),
			Type:         toJWTExportType(rule.Type),
			LocalSubject: jwt.RenamingSubject(rule.LocalSubject),
		}
		if rule.Share != nil {
			jwtImport.Share = *rule.Share
		}
		if rule.AllowTrace != nil {
			jwtImport.AllowTrace = *rule.AllowTrace
		}
		jwtImports[i] = &jwtImport
	}
	result, err := mergeImports(importAccountID, existing, jwtImports)
	if err != nil {
		return existing, err
	}
	return result, nil
}

func mergeImports(importAccountID string, existing jwt.Imports, extras jwt.Imports) (jwt.Imports, error) {
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

func toJWTExportType(source v1alpha1.ExportType) jwt.ExportType {
	var result jwt.ExportType
	switch source {
	case v1alpha1.Stream:
		result = jwt.Stream
	case v1alpha1.Service:
		result = jwt.Service
	default:
		result = jwt.Stream
	}
	return result
}

func toJWTServiceLatency(source v1alpha1.ServiceLatency) *jwt.ServiceLatency {
	return &jwt.ServiceLatency{
		Sampling: jwt.SamplingRate(source.Sampling),
		Results:  jwt.Subject(source.Results),
	}
}
