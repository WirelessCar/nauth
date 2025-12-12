package account

import (
	"context"
	"errors"
	"fmt"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type claimsBuilder struct {
	accountState *natsv1alpha1.Account
	claim        *jwt.AccountClaims
	errs         []error
}

func newClaimsBuilder(accountState *natsv1alpha1.Account, accountPublicKey string) *claimsBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
	claim.Limits = jwt.OperatorLimits{}

	return &claimsBuilder{
		accountState: accountState,
		claim:        claim,
		errs:         make([]error, 0),
	}
}

func (b *claimsBuilder) accountLimits() *claimsBuilder {
	state := b.accountState
	accountLimits := jwt.AccountLimits{}
	accountLimits.Imports = -1
	accountLimits.Exports = -1
	accountLimits.WildcardExports = true
	accountLimits.Conn = -1
	accountLimits.LeafNodeConn = -1

	if state.Spec.AccountLimits != nil {
		if state.Spec.AccountLimits.Imports != nil {
			accountLimits.Imports = *state.Spec.AccountLimits.Imports
		}
		if state.Spec.AccountLimits.Exports != nil {
			accountLimits.Exports = *state.Spec.AccountLimits.Exports
		}
		if state.Spec.AccountLimits.WildcardExports != nil {
			accountLimits.WildcardExports = *state.Spec.AccountLimits.WildcardExports
		}
		if state.Spec.AccountLimits.Conn != nil {
			accountLimits.Conn = *state.Spec.AccountLimits.Conn
		}
		if state.Spec.AccountLimits.LeafNodeConn != nil {
			accountLimits.LeafNodeConn = *state.Spec.AccountLimits.LeafNodeConn
		}
	}

	b.claim.Limits.AccountLimits = accountLimits
	return b
}

func (b *claimsBuilder) natsLimits() *claimsBuilder {
	state := b.accountState

	natsLimits := jwt.NatsLimits{}
	natsLimits.Subs = -1
	natsLimits.Data = -1
	natsLimits.Payload = -1

	if state.Spec.NatsLimits != nil {
		if state.Spec.NatsLimits.Subs != nil {
			natsLimits.Subs = *state.Spec.NatsLimits.Subs
		}
		if state.Spec.NatsLimits.Data != nil {
			natsLimits.Data = *state.Spec.NatsLimits.Data
		}
		if state.Spec.NatsLimits.Payload != nil {
			natsLimits.Payload = *state.Spec.NatsLimits.Payload
		}
	}

	b.claim.Limits.NatsLimits = natsLimits
	return b
}

func (b *claimsBuilder) jetStreamLimits() *claimsBuilder {
	state := b.accountState
	jetStreamLimits := jwt.JetStreamLimits{}
	jetStreamLimits.MemoryStorage = -1
	jetStreamLimits.DiskStorage = -1
	jetStreamLimits.Streams = -1
	jetStreamLimits.Consumer = -1
	jetStreamLimits.MaxAckPending = -1
	jetStreamLimits.MemoryMaxStreamBytes = -1
	jetStreamLimits.DiskMaxStreamBytes = -1

	if state.Spec.JetStreamLimits != nil {
		if state.Spec.JetStreamLimits.MemoryStorage != nil {
			jetStreamLimits.MemoryStorage = *state.Spec.JetStreamLimits.MemoryStorage
		}
		if state.Spec.JetStreamLimits.DiskStorage != nil {
			jetStreamLimits.DiskStorage = *state.Spec.JetStreamLimits.DiskStorage
		}
		if state.Spec.JetStreamLimits.Streams != nil {
			jetStreamLimits.Streams = *state.Spec.JetStreamLimits.Streams
		}
		if state.Spec.JetStreamLimits.Consumer != nil {
			jetStreamLimits.Consumer = *state.Spec.JetStreamLimits.Consumer
		}
		if state.Spec.JetStreamLimits.MaxAckPending != nil {
			jetStreamLimits.MaxAckPending = *state.Spec.JetStreamLimits.MaxAckPending
		}
		if state.Spec.JetStreamLimits.MemoryMaxStreamBytes != nil {
			jetStreamLimits.MemoryMaxStreamBytes = *state.Spec.JetStreamLimits.MemoryMaxStreamBytes
		}
		if state.Spec.JetStreamLimits.DiskMaxStreamBytes != nil {
			jetStreamLimits.DiskMaxStreamBytes = *state.Spec.JetStreamLimits.DiskMaxStreamBytes
		}
		jetStreamLimits.MaxBytesRequired = state.Spec.JetStreamLimits.MaxBytesRequired
	}

	b.claim.Limits.JetStreamLimits = jetStreamLimits
	return b
}

func (b *claimsBuilder) exports() *claimsBuilder {
	state := b.accountState

	if state.Spec.Exports != nil {
		exports := jwt.Exports{}

		for _, export := range state.Spec.Exports {
			exportClaim := &jwt.Export{
				Name:         export.Name,
				Subject:      jwt.Subject(export.Subject),
				Type:         jwt.ExportType(export.Type.ToInt()),
				ResponseType: jwt.ResponseType(export.ResponseType),
				Revocations:  jwt.RevocationList(export.Revocations),
			}
			exports = append(exports, exportClaim)
		}
		b.claim.Exports = exports
	}

	return b
}

func (b *claimsBuilder) imports(ctx context.Context, accountGetter AccountGetter) *claimsBuilder {
	state := b.accountState
	log := logf.FromContext(ctx)

	if state.Spec.Imports != nil {
		imports := jwt.Imports{}

		for _, importClaim := range state.Spec.Imports {
			importAccount, err := accountGetter.Get(ctx, importClaim.AccountRef.Name, importClaim.AccountRef.Namespace)
			if err != nil {
				b.errs = append(b.errs, err)
				log.Error(err, "failed to get account for import", "namespace", importClaim.AccountRef.Namespace, "account", importClaim.AccountRef.Name, "import", importClaim.Name)
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
		b.claim.Imports = imports

		err := validateImports(imports)
		if err != nil {
			b.errs = append(b.errs, err)
			b.claim.Imports = nil
		}
	}

	return b
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

func (b *claimsBuilder) encode(operatorSigningKeyPair nkeys.KeyPair) (string, error) {
	if err := errors.Join(b.errs...); err != nil {
		return "", err
	}

	signedJwt, err := b.claim.Encode(operatorSigningKeyPair)
	if err != nil {
		return "", err
	}

	return signedJwt, nil
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
