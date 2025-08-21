package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	natsv1alpha1 "github.com/WirelessCar-WDP/nauth/api/v1alpha1"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"
	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountManager struct {
	accounts       ports.AccountGetter
	natsClient     ports.NATSClient
	secretStorer   ports.SecretStorer
	nauthNamespace string
}

func NewAccountManager(accounts ports.AccountGetter, natsClient ports.NATSClient, secretStorer ports.SecretStorer, opts ...func(*AccountManager)) *AccountManager {

	manager := &AccountManager{
		accounts:     accounts,
		natsClient:   natsClient,
		secretStorer: secretStorer,
	}

	for _, opt := range opts {
		opt(manager)
	}

	if manager.nauthNamespace == "" {
		controllerNamespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			log.Fatalf("Failed create account manager. Failed to read namespace: %v", err)
		}
		manager.nauthNamespace = string(controllerNamespace)
	}

	if !manager.valid() {
		log.Fatalf("Failed to crate Account manager. Missing required fields.")
		return nil
	}

	return manager
}

func WithNamespace(namespace string) func(*AccountManager) {
	return func(manager *AccountManager) {
		manager.nauthNamespace = namespace
	}
}

func (a *AccountManager) valid() bool {
	if a.accounts == nil {
		return false
	}

	if a.natsClient == nil {
		return false
	}

	if a.secretStorer == nil {
		return false
	}

	if a.nauthNamespace == "" {
		return false
	}

	return true
}

func (a *AccountManager) RefreshState(ctx context.Context, observed *types.Account, desired *natsv1alpha1.Account) error {
	return nil
}

func (a *AccountManager) CreateAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())

	operatorSecret, err := a.secretStorer.GetSecret(ctx, a.nauthNamespace, "operator-op-sign")
	if err != nil {
		return fmt.Errorf("failed to get operator signing key: %w", err)
	}

	seed, ok := operatorSecret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for operator signing key seed was malformed")
	}

	operatorSigningKeyPair, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountKeyPair, _ := nkeys.CreateAccount()
	accountSigningKeyPair, _ := nkeys.CreateAccount()

	accountPublicKey, _ := accountKeyPair.PublicKey()
	accountSeed, _ := accountKeyPair.Seed()
	secretName := state.GetAccountSecretName()
	secretValue := map[string]string{domain.DefaultSecretKeyName: string(accountSeed)}

	err = a.secretStorer.ApplySecret(ctx, nil, state.GetNamespace(), secretName, secretValue)
	if err != nil {
		return err
	}

	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
	accountSigningSeed, _ := accountSigningKeyPair.Seed()
	secretName = state.GetAccountSignSecretName()
	secretValue = map[string]string{domain.DefaultSecretKeyName: string(accountSigningSeed)}

	err = a.secretStorer.ApplySecret(ctx, nil, state.GetNamespace(), secretName, secretValue)
	if err != nil {
		return err
	}

	if state.Labels == nil {
		state.Labels = map[string]string{}
	}
	state.GetLabels()[domain.LabelAccountId] = accountPublicKey
	state.GetLabels()[domain.LabelAccountSignedBy] = operatorSigningPublicKey

	signedJwt, err := newAccountClaimsBuilder(state, accountPublicKey).
		accountLimits().
		natsLimits().
		jetStreamLimits().
		exports().
		imports(ctx, a).
		signingKey(accountSigningPublicKey).
		encode(operatorSigningKeyPair)
	if err != nil {
		return fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	state.Status.Claims = natsv1alpha1.AccountClaims{
		AccountLimits:   state.Spec.AccountLimits,
		Exports:         state.Spec.Exports,
		Imports:         state.Spec.Imports,
		JetStreamLimits: state.Spec.JetStreamLimits,
		NatsLimits:      state.Spec.NatsLimits,
	}
	state.Status.SigningKey.Name = accountSigningPublicKey
	state.Status.SigningKey.CreationDate = metav1.Now()

	err = a.natsClient.EnsureConnected(a.nauthNamespace, "operator-sau-creds")
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()

	err = a.natsClient.UploadAccountJWT(signedJwt)
	if err != nil {
		return fmt.Errorf("failed to upload account jwt: %w", err)
	}

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (a *AccountManager) UpdateAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())

	operatorSecret, err := a.secretStorer.GetSecret(ctx, a.nauthNamespace, "operator-op-sign")
	if err != nil {
		return fmt.Errorf("failed to get operator signing key: %w", err)
	}

	seed, ok := operatorSecret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for operator root seed was malformed")
	}

	operatorSigningKeyPair, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return fmt.Errorf("failed to get operator root key pair from seed: %w", err)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	secretName := state.GetAccountSecretName()
	secret, err := a.secretStorer.GetSecret(ctx, state.GetNamespace(), secretName)
	if err != nil {
		return fmt.Errorf("failed to get account key: %w", err)
	}
	accountSeed, ok := secret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account was malformed")
	}
	accountKeyPair, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair from seed: %w", err)
	}
	accountPublicKey, _ := accountKeyPair.PublicKey()

	secretName = state.GetAccountSignSecretName()
	secret, err = a.secretStorer.GetSecret(ctx, state.GetNamespace(), secretName)
	if err != nil {
		return fmt.Errorf("failed to get account signing key: %w", err)
	}
	accountSigningSeed, ok := secret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account signing was malformed")
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair for signing from seed: %w", err)
	}
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()

	if state.Labels == nil {
		state.Labels = map[string]string{}
	}
	state.GetLabels()[domain.LabelAccountId] = accountPublicKey
	state.GetLabels()[domain.LabelAccountSignedBy] = operatorSigningPublicKey

	signedJwt, err := newAccountClaimsBuilder(state, accountPublicKey).
		accountLimits().
		natsLimits().
		jetStreamLimits().
		exports().
		imports(ctx, a).
		signingKey(accountSigningPublicKey).
		encode(operatorSigningKeyPair)
	if err != nil {
		return fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	state.Status.Claims = natsv1alpha1.AccountClaims{
		AccountLimits:   state.Spec.AccountLimits,
		Exports:         state.Spec.Exports,
		Imports:         state.Spec.Imports,
		JetStreamLimits: state.Spec.JetStreamLimits,
		NatsLimits:      state.Spec.NatsLimits,
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace, "operator-sau-creds")
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()

	err = a.natsClient.UploadAccountJWT(signedJwt)
	if err != nil {
		return fmt.Errorf("failed to upload account jwt: %w", err)
	}

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (a *AccountManager) DeleteAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())

	operatorSecret, err := a.secretStorer.GetSecret(ctx, a.nauthNamespace, "operator-op-sign")
	if err != nil {
		return fmt.Errorf("failed to get operator signing key: %w", err)
	}

	operatorSeed, ok := operatorSecret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for operator root seed was malformed")
	}

	operatorKeyPair, err := nkeys.FromSeed([]byte(operatorSeed))
	if err != nil {
		return fmt.Errorf("failed to get operator root key pair from seed: %w", err)
	}

	operatorPublicKey, _ := operatorKeyPair.PublicKey()

	// Delete is done by signing a jwt with a list of accounts to be deleted
	deleteClaim := jwt.NewGenericClaims(operatorPublicKey)
	deleteClaim.Data["accounts"] = []string{state.GetLabels()[domain.LabelAccountId]}
	deleteJwt, err := deleteClaim.Encode(operatorKeyPair)

	if err != nil {
		return fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace, "operator-sau-creds")
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()

	err = a.natsClient.DeleteAccountJWT(deleteJwt)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	return nil
}

type accountClaimBuilder struct {
	accountState *natsv1alpha1.Account
	claim        *jwt.AccountClaims
	errs         []error
}

func newAccountClaimsBuilder(accountState *natsv1alpha1.Account, accountPublicKey string) *accountClaimBuilder {
	claim := jwt.NewAccountClaims(accountPublicKey)
	claim.Limits = jwt.OperatorLimits{}

	return &accountClaimBuilder{
		accountState: accountState,
		claim:        claim,
		errs:         make([]error, 0),
	}
}

func (b *accountClaimBuilder) accountLimits() *accountClaimBuilder {
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

func (b *accountClaimBuilder) natsLimits() *accountClaimBuilder {
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

func (b *accountClaimBuilder) jetStreamLimits() *accountClaimBuilder {
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

func (b *accountClaimBuilder) exports() *accountClaimBuilder {
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

func (b *accountClaimBuilder) imports(ctx context.Context, accountManager *AccountManager) *accountClaimBuilder {
	state := b.accountState
	log := logf.FromContext(ctx)

	if state.Spec.Imports != nil {
		imports := jwt.Imports{}

		for _, importClaim := range state.Spec.Imports {
			importAccount, err := accountManager.accounts.Get(ctx, importClaim.AccountRef.Name, importClaim.AccountRef.Namespace)
			if err != nil {
				b.errs = append(b.errs, err)
				log.Error(err, "failed to get account for import", "namespace", importClaim.AccountRef.Namespace, "account", importClaim.AccountRef.Name, "import", importClaim.Name)
			} else {
				account := importAccount.Labels[domain.LabelAccountId]
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
	}

	return b
}

func (b *accountClaimBuilder) signingKey(signingKey string) *accountClaimBuilder {
	b.claim.SigningKeys.Add(signingKey)
	return b
}

func (b *accountClaimBuilder) encode(operatorSigningKeyPair nkeys.KeyPair) (string, error) {
	if err := errors.Join(b.errs...); err != nil {
		return "", err
	}

	signedJwt, err := b.claim.Encode(operatorSigningKeyPair)
	if err != nil {
		return "", err
	}

	return signedJwt, nil
}
