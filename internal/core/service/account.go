package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

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
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	accountKeyPair, _ := nkeys.CreateAccount()
	accountPublicKey, _ := accountKeyPair.PublicKey()
	accountSecretMeta := metav1.ObjectMeta{
		Name:      getAccountRootSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			domain.LabelAccountID:  accountPublicKey,
			domain.LabelSecretType: domain.SecretTypeAccountRoot,
			domain.LabelManaged:    domain.LabelManagedValue,
		},
	}
	accountSeed, _ := accountKeyPair.Seed()
	accountSecretValue := map[string]string{domain.DefaultSecretKeyName: string(accountSeed)}

	if err := a.secretStorer.ApplySecret(ctx, nil, accountSecretMeta, accountSecretValue); err != nil {
		return err
	}

	accountSigningKeyPair, _ := nkeys.CreateAccount()
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
	accountSigningSecretMeta := metav1.ObjectMeta{
		Name:      getAccountSignSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			domain.LabelAccountID:  accountPublicKey,
			domain.LabelSecretType: domain.SecretTypeAccountSign,
			domain.LabelManaged:    domain.LabelManagedValue,
		},
	}
	accountSigningSeed, _ := accountSigningKeyPair.Seed()
	accountSignSeedSecretValue := map[string]string{domain.DefaultSecretKeyName: string(accountSigningSeed)}

	if err := a.secretStorer.ApplySecret(ctx, nil, accountSigningSecretMeta, accountSignSeedSecretValue); err != nil {
		return err
	}

	if state.Labels == nil {
		state.Labels = make(map[string]string, 2)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()
	state.GetLabels()[domain.LabelAccountID] = accountPublicKey
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
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
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

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
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
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	accountID := state.GetLabels()[domain.LabelAccountID]
	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return err
	}

	accountSecret, ok := secrets[domain.SecretTypeAccountRoot]
	if !ok {
		return fmt.Errorf("secret for account not found")
	}
	accountSeed, ok := accountSecret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account was malformed")
	}
	accountKeyPair, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair from seed: %w", err)
	}
	accountPublicKey, _ := accountKeyPair.PublicKey()

	accountSigningSecret, ok := secrets[domain.SecretTypeAccountSign]
	if !ok {
		return fmt.Errorf("secret for account signing not found")
	}
	accountSigningSeed, ok := accountSigningSecret[domain.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account signing was malformed")
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair for signing from seed: %w", err)
	}
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()

	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()
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
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	state.Status.Claims = natsv1alpha1.AccountClaims{
		AccountLimits:   state.Spec.AccountLimits,
		Exports:         state.Spec.Exports,
		Imports:         state.Spec.Imports,
		JetStreamLimits: state.Spec.JetStreamLimits,
		NatsLimits:      state.Spec.NatsLimits,
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
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
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountID := state.GetLabels()[domain.LabelAccountID]

	// Delete is done by signing a jwt with a list of accounts to be deleted
	deleteClaim := jwt.NewGenericClaims(operatorPublicKey)
	deleteClaim.Data["accounts"] = []string{accountID}

	deleteJwt, err := deleteClaim.Encode(operatorSigningKeyPair)
	if err != nil {
		return fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()

	err = a.natsClient.DeleteAccountJWT(deleteJwt)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	labels := map[string]string{
		domain.LabelAccountID: accountID,
	}
	a.secretStorer.DeleteSecretsByLabels(ctx, state.GetNamespace(), labels)

	return nil
}

func (a AccountManager) getOperatorSigningKeyPair(ctx context.Context) (nkeys.KeyPair, error) {
	labels := map[string]string{
		domain.LabelSecretType: domain.SecretTypeOperatorSign,
	}
	operatorSecret, err := a.secretStorer.GetSecretsByLabels(ctx, a.nauthNamespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key: %w", err)
	}

	if len(operatorSecret.Items) < 1 {
		return nil, fmt.Errorf("missing operator signing key secret, make sure to label the secret with the label %s: %s", domain.LabelSecretType, domain.SecretTypeOperatorSign)
	}

	if len(operatorSecret.Items) > 1 {
		return nil, fmt.Errorf("multiple operator signing key secrets found, make sure only one secret has the label %s: %s", domain.LabelSecretType, domain.SecretTypeSystemAccountUserCreds)
	}

	seed, ok := operatorSecret.Items[0].Data[domain.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for operator signing key seed was malformed")
	}

	return nkeys.FromSeed(seed)
}

func (a AccountManager) getAccountSecrets(ctx context.Context, namespace, accountID, accountName string) (map[string]map[string]string, error) {
	if secrets, err := a.getAccountSecretsByAccountID(ctx, namespace, accountName, accountID); err == nil {
		return secrets, nil
	}

	secrets, err := a.getDeprecatedAccountSecretsByName(ctx, namespace, accountName, accountID)
	if err != nil {
		return nil, err
	}

	return secrets, nil
}

func (a AccountManager) getAccountSecretsByAccountID(ctx context.Context, namespace, accountName, accountID string) (map[string]map[string]string, error) {
	labels := map[string]string{
		domain.LabelAccountID: accountID,
		domain.LabelManaged:   domain.LabelManagedValue,
	}
	k8sSecrets, err := a.secretStorer.GetSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	if len(k8sSecrets.Items) < 2 {
		return nil, fmt.Errorf("missing one or more secret(s) for account: %s-%s", namespace, accountName)
	}

	if len(k8sSecrets.Items) > 2 {
		return nil, fmt.Errorf("more than 2 secrets found for account: %s-%s", namespace, accountName)
	}

	secrets := make(map[string]map[string]string, len(k8sSecrets.Items))
	for _, secret := range k8sSecrets.Items {
		secretType := secret.GetLabels()[domain.LabelSecretType]
		if _, ok := secrets[secretType]; ok {
			return nil, fmt.Errorf("multiple secrets of type (%s) found for account: %s-%s", secretType, namespace, accountName)
		}
		secretData := make(map[string]string, len(secret.Data))
		for k, v := range secret.Data {
			secretData[k] = string(v)
		}
		secrets[secretType] = secretData
	}
	return secrets, nil
}

func (a AccountManager) getDeprecatedAccountSecretsByName(ctx context.Context, namespace, accountName, accountID string) (map[string]map[string]string, error) {
	logger := logf.FromContext(ctx)

	type goRoutineResult struct {
		secret map[string]string
		err    error
	}
	var wg sync.WaitGroup
	ch := make(chan goRoutineResult, 2)

	for _, s := range []struct {
		secretName string
		secretType string
	}{
		{
			secretName: fmt.Sprintf(domain.DeprecatedSecretNameAccountRootTemplate, accountName),
			secretType: domain.SecretTypeAccountRoot,
		},
		{
			secretName: fmt.Sprintf(domain.DeprecatedSecretNameAccountSignTemplate, accountName),
			secretType: domain.SecretTypeAccountSign,
		},
	} {
		wg.Add(1)
		go func(secretName, secretType string) {
			result := goRoutineResult{}
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					result.err = fmt.Errorf("recovered panicked go routine from trying to get secret %s-%s of type %s: %v", namespace, secretName, secretType, r)
					ch <- result
				}
			}()

			accountSecret, err := a.secretStorer.GetSecret(ctx, namespace, secretName)
			if err != nil {
				result.err = err
				ch <- result
				return
			}

			labels := map[string]string{
				domain.LabelAccountID:  accountID,
				domain.LabelSecretType: secretType,
				domain.LabelManaged:    domain.LabelManagedValue,
			}
			if err := a.secretStorer.LabelSecret(ctx, namespace, secretName, labels); err != nil {
				logger.Info("unable to label secret", "secretName", secretName, "namespace", namespace, "secretType", secretType, "error", err)
			}
			accountSecret[domain.LabelSecretType] = secretType
			result.secret = accountSecret
			ch <- result
		}(s.secretName, s.secretType)
	}

	wg.Wait()
	close(ch)

	var errs []error
	secrets := make(map[string]map[string]string, 2)

	for res := range ch {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		secrets[res.secret[domain.LabelSecretType]] = res.secret
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if len(secrets) < 2 {
		return nil, fmt.Errorf("missing one or more deprecated secret(s) for account: %s-%s", namespace, accountName)
	}

	return secrets, nil
}

func getAccountRootSecretName(accountName, accountID string) string {
	return fmt.Sprintf(domain.SecretNameAccountRootTemplate, accountName, generateShortHashFromID(accountID))
}

func getAccountSignSecretName(accountName, accountID string) string {
	return fmt.Sprintf(domain.SecretNameAccountSignTemplate, accountName, generateShortHashFromID(accountID))
}

func generateShortHashFromID(ID string) string {
	hasher := md5.New()
	io.WriteString(hasher, ID)
	hash := hex.EncodeToString(hasher.Sum(nil))
	if len(hash) > 6 {
		return hash[:6]
	}
	return hash
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
				account := importAccount.Labels[domain.LabelAccountID]
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

func (b *accountClaimBuilder) signingKey(signingKey string) *accountClaimBuilder {
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
