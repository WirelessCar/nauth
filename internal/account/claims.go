package account

import (
	"context"
	"errors"
	"fmt"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
)

type claimsBuilder struct {
	claim *jwt.AccountClaims
	errs  []error
}

func newClaimsBuilder(
	ctx context.Context,
	spec natsv1alpha1.AccountSpec,
	accountPublicKey string,
	accountGetter AccountGetter,
) *claimsBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
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
		exports := jwt.Exports{}

		for _, export := range spec.Exports {
			var targetType jwt.ExportType
			switch export.Type {
			case natsv1alpha1.Stream:
				targetType = jwt.Stream
			case natsv1alpha1.Service:
				targetType = jwt.Service
			default:
				targetType = jwt.Stream
			}

			var latency *jwt.ServiceLatency
			if export.Latency != nil {
				latency = &jwt.ServiceLatency{
					Sampling: jwt.SamplingRate(export.Latency.Sampling),
					Results:  jwt.Subject(export.Latency.Results),
				}
			}

			exportClaim := &jwt.Export{
				Name:                 export.Name,
				Subject:              jwt.Subject(export.Subject),
				Type:                 targetType,
				TokenReq:             export.TokenReq,
				Revocations:          jwt.RevocationList(export.Revocations),
				ResponseType:         jwt.ResponseType(export.ResponseType),
				ResponseThreshold:    export.ResponseThreshold,
				Latency:              latency,
				AccountTokenPosition: export.AccountTokenPosition,
				Advertise:            export.Advertise,
				AllowTrace:           export.AllowTrace,
			}
			exports = append(exports, exportClaim)
		}
		claim.Exports = exports
	}

	// Imports
	if spec.Imports != nil {
		imports := jwt.Imports{}

		for _, importClaim := range spec.Imports {
			importAccount, err := accountGetter.Get(ctx, importClaim.AccountRef.Name, importClaim.AccountRef.Namespace)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get account for import %q (namespace: %q, account: %q): %w",
					importClaim.Name,
					importClaim.AccountRef.Namespace,
					importClaim.AccountRef.Name,
					err))
			} else {
				account := importAccount.Labels[k8s.LabelAccountID]
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

		err := validateImports(imports)
		if err != nil {
			errs = append(errs, err)
			claim.Imports = nil
		}
	}

	return &claimsBuilder{
		claim: claim,
		errs:  errs,
	}
}

func (b *claimsBuilder) signingKey(signingKey string) *claimsBuilder {
	b.claim.SigningKeys.Add(signingKey)
	return b
}

func validateImports(imports jwt.Imports) error {
	seenSubjects := make(map[string]bool, len(imports))

	for _, importClaim := range imports {
		subject := string(importClaim.Subject)
		if importClaim.LocalSubject != "" {
			subject = string(importClaim.LocalSubject)
		}

		if seenSubjects[subject] {
			return fmt.Errorf("conflicting import subject found: %s", subject)
		}
		seenSubjects[subject] = true
	}

	return nil
}

func (b *claimsBuilder) build() (*jwt.AccountClaims, error) {
	if err := errors.Join(b.errs...); err != nil {
		return nil, err
	}

	return b.claim, nil
}

func convertNatsAccountClaims(claims *jwt.AccountClaims) natsv1alpha1.AccountClaims {
	if claims == nil {
		return natsv1alpha1.AccountClaims{}
	}

	out := natsv1alpha1.AccountClaims{}

	// AccountLimits
	{
		imports := claims.Limits.Imports
		exports := claims.Limits.Exports
		wildcards := claims.Limits.WildcardExports
		conn := claims.Limits.Conn
		leaf := claims.Limits.LeafNodeConn
		out.AccountLimits = &natsv1alpha1.AccountLimits{
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
		out.NatsLimits = &natsv1alpha1.NatsLimits{
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
		out.JetStreamLimits = &natsv1alpha1.JetStreamLimits{
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

	// Exports
	if len(claims.Exports) > 0 {
		exports := make(natsv1alpha1.Exports, 0, len(claims.Exports))
		for _, e := range claims.Exports {
			if e == nil {
				continue
			}
			var et natsv1alpha1.ExportType
			switch e.Type {
			case jwt.Stream:
				et = natsv1alpha1.Stream
			case jwt.Service:
				et = natsv1alpha1.Service
			default:
				et = natsv1alpha1.Stream
			}

			var latency *natsv1alpha1.ServiceLatency
			if e.Latency != nil {
				latency = &natsv1alpha1.ServiceLatency{
					Sampling: natsv1alpha1.SamplingRate(e.Latency.Sampling),
					Results:  natsv1alpha1.Subject(e.Latency.Results),
				}
			}

			export := &natsv1alpha1.Export{
				Name:                 e.Name,
				Subject:              natsv1alpha1.Subject(e.Subject),
				Type:                 et,
				TokenReq:             e.TokenReq,
				Revocations:          natsv1alpha1.RevocationList(e.Revocations),
				ResponseType:         natsv1alpha1.ResponseType(e.ResponseType),
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
		imports := make(natsv1alpha1.Imports, 0, len(claims.Imports))
		for _, i := range claims.Imports {
			if i == nil {
				continue
			}
			var it natsv1alpha1.ExportType
			switch i.Type {
			case jwt.Stream:
				it = natsv1alpha1.Stream
			case jwt.Service:
				it = natsv1alpha1.Service
			default:
				it = natsv1alpha1.Stream
			}
			imp := &natsv1alpha1.Import{
				Name:         i.Name,
				Subject:      natsv1alpha1.Subject(i.Subject),
				Account:      i.Account,
				LocalSubject: natsv1alpha1.RenamingSubject(i.LocalSubject),
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
