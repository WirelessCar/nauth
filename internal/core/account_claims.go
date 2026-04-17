package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
	"k8s.io/apimachinery/pkg/util/json"
)

type accountClaimsBuilder struct {
	claim *jwt.AccountClaims
	errs  []error
}

func newAccountClaimsBuilder(
	ctx context.Context,
	displayName string,
	spec v1alpha1.AccountSpec,
	accountPublicKey string,
	accountReader outbound.AccountReader,
) *accountClaimsBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
	claim.Name = displayName
	claim.Limits = jwt.OperatorLimits{}
	errs := make([]error, 0)

	// Account Limits
	{
		accountLimits := jwt.AccountLimits{}
		accountLimits.Imports = -1
		accountLimits.Exports = -1
		accountLimits.WildcardExports = true
		accountLimits.Conn = -1
		accountLimits.LeafNodeConn = -1

		if spec.AccountLimits != nil {
			if spec.AccountLimits.Imports != nil {
				accountLimits.Imports = *spec.AccountLimits.Imports
			}
			if spec.AccountLimits.Exports != nil {
				accountLimits.Exports = *spec.AccountLimits.Exports
			}
			if spec.AccountLimits.WildcardExports != nil {
				accountLimits.WildcardExports = *spec.AccountLimits.WildcardExports
			}
			if spec.AccountLimits.Conn != nil {
				accountLimits.Conn = *spec.AccountLimits.Conn
			}
			if spec.AccountLimits.LeafNodeConn != nil {
				accountLimits.LeafNodeConn = *spec.AccountLimits.LeafNodeConn
			}
		}
		claim.Limits.AccountLimits = accountLimits
	}

	// NATS Limits
	{
		natsLimits := jwt.NatsLimits{}
		natsLimits.Subs = -1
		natsLimits.Data = -1
		natsLimits.Payload = -1

		if spec.NatsLimits != nil {
			if spec.NatsLimits.Subs != nil {
				natsLimits.Subs = *spec.NatsLimits.Subs
			}
			if spec.NatsLimits.Data != nil {
				natsLimits.Data = *spec.NatsLimits.Data
			}
			if spec.NatsLimits.Payload != nil {
				natsLimits.Payload = *spec.NatsLimits.Payload
			}
		}

		claim.Limits.NatsLimits = natsLimits
	}

	// JetStream Limits
	{
		jetStreamLimits := jwt.JetStreamLimits{}
		jetStreamLimits.MemoryStorage = -1
		jetStreamLimits.DiskStorage = -1
		jetStreamLimits.Streams = -1
		jetStreamLimits.Consumer = -1
		jetStreamLimits.MaxAckPending = -1
		jetStreamLimits.MemoryMaxStreamBytes = -1
		jetStreamLimits.DiskMaxStreamBytes = -1

		if spec.JetStreamLimits != nil {
			if spec.JetStreamLimits.MemoryStorage != nil {
				jetStreamLimits.MemoryStorage = *spec.JetStreamLimits.MemoryStorage
			}
			if spec.JetStreamLimits.DiskStorage != nil {
				jetStreamLimits.DiskStorage = *spec.JetStreamLimits.DiskStorage
			}
			if spec.JetStreamLimits.Streams != nil {
				jetStreamLimits.Streams = *spec.JetStreamLimits.Streams
			}
			if spec.JetStreamLimits.Consumer != nil {
				jetStreamLimits.Consumer = *spec.JetStreamLimits.Consumer
			}
			if spec.JetStreamLimits.MaxAckPending != nil {
				jetStreamLimits.MaxAckPending = *spec.JetStreamLimits.MaxAckPending
			}
			if spec.JetStreamLimits.MemoryMaxStreamBytes != nil {
				jetStreamLimits.MemoryMaxStreamBytes = *spec.JetStreamLimits.MemoryMaxStreamBytes
			}
			if spec.JetStreamLimits.DiskMaxStreamBytes != nil {
				jetStreamLimits.DiskMaxStreamBytes = *spec.JetStreamLimits.DiskMaxStreamBytes
			}
			jetStreamLimits.MaxBytesRequired = spec.JetStreamLimits.MaxBytesRequired
		}

		claim.Limits.JetStreamLimits = jetStreamLimits
	}

	// Exports
	if spec.Exports != nil {
		exports := make(jwt.Exports, 0, len(spec.Exports))

		for _, export := range spec.Exports {
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
			exports = append(exports, exportClaim)
		}
		claim.Exports = exports
	}

	// Imports
	if spec.Imports != nil {
		imports := jwt.Imports{}

		for _, importClaim := range spec.Imports {
			accountRef := domain.NewNamespacedName(importClaim.AccountRef.Namespace, importClaim.AccountRef.Name)
			// TODO: [#228] Extract Import Account ID lookup to controller layer
			importAccount, err := accountReader.Get(ctx, accountRef)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get account for import %q (account: %q): %w",
					importClaim.Name,
					accountRef,
					err))
			} else {
				account := importAccount.GetLabel(v1alpha1.AccountLabelAccountID)
				claim := &jwt.Import{
					Name:         importClaim.Name,
					Subject:      jwt.Subject(importClaim.Subject),
					Type:         jwt.ExportType(importClaim.Type.ToInt()),
					Account:      account,
					LocalSubject: jwt.RenamingSubject(importClaim.LocalSubject),
				}
				imports = append(imports, claim)
			}
		}
		claim.Imports = imports
	}

	return &accountClaimsBuilder{
		claim: claim,
		errs:  errs,
	}
}

func (b *accountClaimsBuilder) addExportRuleGroup(rules []v1alpha1.AccountExportRule) error {
	tmpClaim := *b.claim
	for _, rule := range rules {
		export := jwt.Export{
			Name:         rule.Name,
			Subject:      jwt.Subject(rule.Subject),
			Type:         toJWTExportType(rule.Type),
			ResponseType: jwt.ResponseType(rule.ResponseType),
		}
		if rule.ResponseThreshold != nil {
			export.ResponseThreshold = *rule.ResponseThreshold
		}
		if rule.Latency != nil {
			export.Latency = toJWTServiceLatency(*rule.Latency)
		}
		if rule.AccountTokenPosition != nil {
			export.AccountTokenPosition = *rule.AccountTokenPosition
		}
		if rule.Advertise != nil {
			export.Advertise = *rule.Advertise
		}
		if rule.AllowTrace != nil {
			export.AllowTrace = *rule.AllowTrace
		}
		tmpClaim.Exports = appendExportIfMissing(tmpClaim.Exports, export)
	}
	validationResults := &jwt.ValidationResults{}
	tmpClaim.Exports.Validate(validationResults)
	validationErrors := validationResults.Errors()
	if len(validationErrors) != 0 {
		return fmt.Errorf("rules adoption failed: %w", errors.Join(validationErrors...))
	}
	b.claim.Exports = tmpClaim.Exports
	return nil
}

func (b *accountClaimsBuilder) addSigningKey(signingKey string) *accountClaimsBuilder {
	b.claim.SigningKeys.Add(signingKey)
	return b
}

func (b *accountClaimsBuilder) build() (*jwt.AccountClaims, error) {
	if err := errors.Join(b.errs...); err != nil {
		return nil, err
	}

	return b.claim, nil
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

func convertNatsAccountClaims(claims *jwt.AccountClaims) v1alpha1.AccountClaims {
	if claims == nil {
		return v1alpha1.AccountClaims{}
	}

	out := v1alpha1.AccountClaims{}
	out.DisplayName = claims.Name

	// AccountLimits
	{
		imports := claims.Limits.Imports
		exports := claims.Limits.Exports
		wildcards := claims.Limits.WildcardExports
		conn := claims.Limits.Conn
		leaf := claims.Limits.LeafNodeConn
		out.AccountLimits = &v1alpha1.AccountLimits{
			Imports:         &imports,
			Exports:         &exports,
			WildcardExports: &wildcards,
			Conn:            &conn,
			LeafNodeConn:    &leaf,
		}
	}

	// NatsLimits
	{
		subs := claims.Limits.Subs
		data := claims.Limits.Data
		payload := claims.Limits.Payload
		out.NatsLimits = &v1alpha1.NatsLimits{
			Subs:    &subs,
			Data:    &data,
			Payload: &payload,
		}
	}

	// JetStreamLimits
	{
		mem := claims.Limits.MemoryStorage
		disk := claims.Limits.DiskStorage
		streams := claims.Limits.Streams
		consumer := claims.Limits.Consumer
		maxAck := claims.Limits.MaxAckPending
		memMax := claims.Limits.MemoryMaxStreamBytes
		diskMax := claims.Limits.DiskMaxStreamBytes
		out.JetStreamLimits = &v1alpha1.JetStreamLimits{
			MemoryStorage:        &mem,
			DiskStorage:          &disk,
			Streams:              &streams,
			Consumer:             &consumer,
			MaxAckPending:        &maxAck,
			MemoryMaxStreamBytes: &memMax,
			DiskMaxStreamBytes:   &diskMax,
			MaxBytesRequired:     claims.Limits.MaxBytesRequired,
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

func appendExportIfMissing(exports jwt.Exports, export jwt.Export) jwt.Exports {
	for _, existing := range exports {
		if existing != nil && reflect.DeepEqual(export, *existing) {
			return exports
		}
	}
	return append(exports, &export)
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
