package account

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

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/types"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretStorer interface {
	// TODO: Keys created should be immutable
	ApplySecret(ctx context.Context, owner *k8s.SecretOwner, meta metav1.ObjectMeta, valueMap map[string]string) error
	GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
	DeleteSecret(ctx context.Context, namespace string, name string) error
	DeleteSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) error
	LabelSecret(ctx context.Context, namespace, name string, labels map[string]string) error
}

type NatsClient interface {
	EnsureConnected(namespace string) error
	Disconnect()
	LookupAccountJWT(string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}

type AccountGetter interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *natsv1alpha1.Account, err error)
}

type AccountManager struct {
	accounts       AccountGetter
	natsClient     NatsClient
	secretStorer   SecretStorer
	nauthNamespace string
}

func NewAccountManager(accounts AccountGetter, natsClient NatsClient, secretStorer SecretStorer, opts ...func(*AccountManager)) *AccountManager {
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
			types.LabelAccountID:  accountPublicKey,
			types.LabelSecretType: types.SecretTypeAccountRoot,
			types.LabelManaged:    types.LabelManagedValue,
		},
	}
	accountSeed, _ := accountKeyPair.Seed()
	accountSecretValue := map[string]string{types.DefaultSecretKeyName: string(accountSeed)}

	if err := a.secretStorer.ApplySecret(ctx, nil, accountSecretMeta, accountSecretValue); err != nil {
		return err
	}

	accountSigningKeyPair, _ := nkeys.CreateAccount()
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
	accountSigningSecretMeta := metav1.ObjectMeta{
		Name:      getAccountSignSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			types.LabelAccountID:  accountPublicKey,
			types.LabelSecretType: types.SecretTypeAccountSign,
			types.LabelManaged:    types.LabelManagedValue,
		},
	}
	accountSigningSeed, _ := accountSigningKeyPair.Seed()
	accountSignSeedSecretValue := map[string]string{types.DefaultSecretKeyName: string(accountSigningSeed)}

	if err := a.secretStorer.ApplySecret(ctx, nil, accountSigningSecretMeta, accountSignSeedSecretValue); err != nil {
		return err
	}

	if state.Labels == nil {
		state.Labels = make(map[string]string, 2)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()
	state.GetLabels()[types.LabelAccountID] = accountPublicKey
	state.GetLabels()[types.LabelAccountSignedBy] = operatorSigningPublicKey

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

	return nil
}

func (a *AccountManager) UpdateAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	accountID := state.GetLabels()[types.LabelAccountID]
	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return err
	}

	if sysAccountID, err := a.getSystemAccountID(ctx, a.nauthNamespace); err != nil || sysAccountID == accountID {
		if err != nil {
			return fmt.Errorf("failed to get system account ID: %w", err)
		}

		return fmt.Errorf("updating system account is not supported, consider '%s: %s'", types.LabelManagementPolicy, types.LabelManagementPolicyObserveValue)
	}

	accountSecret, ok := secrets[types.SecretTypeAccountRoot]
	if !ok {
		return fmt.Errorf("secret for account not found")
	}
	accountSeed, ok := accountSecret[types.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account was malformed")
	}
	accountKeyPair, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair from seed: %w", err)
	}
	accountPublicKey, _ := accountKeyPair.PublicKey()

	accountSigningSecret, ok := secrets[types.SecretTypeAccountSign]
	if !ok {
		return fmt.Errorf("secret for account signing not found")
	}
	accountSigningSeed, ok := accountSigningSecret[types.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("secret for account signing was malformed")
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair for signing from seed: %w", err)
	}
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()

	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()
	state.GetLabels()[types.LabelAccountSignedBy] = operatorSigningPublicKey

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

	return nil
}

func (a *AccountManager) ImportAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountID := state.GetLabels()[types.LabelAccountID]
	if accountID == "" {
		return fmt.Errorf("account ID is missing for account %s", state.GetName())
	}

	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return fmt.Errorf("failed to get secrets for account %s: %w", accountID, err)
	}

	accountRootSecret, ok := secrets[types.SecretTypeAccountRoot]
	if !ok {
		return fmt.Errorf("account root secret not found for account %s", accountID)
	}
	accountRootSeed, ok := accountRootSecret[types.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("account root seed secret for account %s is malformed", accountID)
	}
	accountRootKeyPair, err := nkeys.FromSeed([]byte(accountRootSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair for account %s from seed: %w", accountID, err)
	}
	accountRootPublicKey, err := accountRootKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get account key pair for account %s from seed: %w", accountID, err)
	}
	if accountRootPublicKey != accountID {
		return fmt.Errorf("account root seed does not match account ID: expected %s, got %s", accountID, accountRootPublicKey)
	}

	accountSigningSecret, ok := secrets[types.SecretTypeAccountSign]
	if !ok {
		return fmt.Errorf("account sign secret not found for account %s", accountID)
	}
	accountSigningSeed, ok := accountSigningSecret[types.DefaultSecretKeyName]
	if !ok {
		return fmt.Errorf("account sign secret for account %s is malformed", accountID)
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return fmt.Errorf("failed to get account key pair for signing from seed for account %s: %w", accountID, err)
	}
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()
	accountJWT, err := a.natsClient.LookupAccountJWT(accountID)
	if err != nil {
		return fmt.Errorf("failed to lookup account jwt for account %s: %w", accountID, err)
	}
	if len(accountJWT) == 0 {
		return fmt.Errorf("account jwt for account %s not found", accountID)
	}
	accountClaims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return fmt.Errorf("failed to decode account jwt for account %s: %w", accountID, err)
	}

	state.GetLabels()[types.LabelAccountSignedBy] = operatorSigningPublicKey
	state.Status.Claims = convertNatsAccountClaims(accountClaims)
	state.Status.SigningKey.Name = accountSigningPublicKey

	return nil
}

func (a *AccountManager) DeleteAccount(ctx context.Context, state *natsv1alpha1.Account) error {
	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountID := state.GetLabels()[types.LabelAccountID]

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
		types.LabelAccountID: accountID,
	}

	return a.secretStorer.DeleteSecretsByLabels(ctx, state.GetNamespace(), labels)
}

func (a AccountManager) getOperatorSigningKeyPair(ctx context.Context) (nkeys.KeyPair, error) {
	labels := map[string]string{
		types.LabelSecretType: types.SecretTypeOperatorSign,
	}
	operatorSecret, err := a.secretStorer.GetSecretsByLabels(ctx, a.nauthNamespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key: %w", err)
	}

	if len(operatorSecret.Items) < 1 {
		return nil, fmt.Errorf("missing operator signing key secret, make sure to label the secret with the label %s: %s", types.LabelSecretType, types.SecretTypeOperatorSign)
	}

	if len(operatorSecret.Items) > 1 {
		return nil, fmt.Errorf("multiple operator signing key secrets found, make sure only one secret has the label %s: %s", types.LabelSecretType, types.SecretTypeSystemAccountUserCreds)
	}

	seed, ok := operatorSecret.Items[0].Data[types.DefaultSecretKeyName]
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
		types.LabelAccountID: accountID,
		types.LabelManaged:   types.LabelManagedValue,
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
		secretType := secret.GetLabels()[types.LabelSecretType]
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

// Todo: Almost identical to the one in user/account.go - refactor ?
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
			secretName: fmt.Sprintf(types.DeprecatedSecretNameAccountRootTemplate, accountName),
			secretType: types.SecretTypeAccountRoot,
		},
		{
			secretName: fmt.Sprintf(types.DeprecatedSecretNameAccountSignTemplate, accountName),
			secretType: types.SecretTypeAccountSign,
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
				types.LabelAccountID:  accountID,
				types.LabelSecretType: secretType,
				types.LabelManaged:    types.LabelManagedValue,
			}
			if err := a.secretStorer.LabelSecret(ctx, namespace, secretName, labels); err != nil {
				logger.Info("unable to label secret", "secretName", secretName, "namespace", namespace, "secretType", secretType, "error", err)
			}
			accountSecret[types.LabelSecretType] = secretType
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
		secrets[res.secret[types.LabelSecretType]] = res.secret
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if len(secrets) < 2 {
		return nil, fmt.Errorf("missing one or more deprecated secret(s) for account: %s-%s", namespace, accountName)
	}

	return secrets, nil
}

func (a *AccountManager) getSystemAccountID(ctx context.Context, namespace string) (string, error) {
	labels := map[string]string{
		types.LabelSecretType: types.SecretTypeSystemAccountUserCreds,
	}
	secrets, err := a.secretStorer.GetSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return "", fmt.Errorf("unable to get system account creds by labels: %w", err)
	}
	if len(secrets.Items) != 1 {
		return "", fmt.Errorf("single system account creds secret required, %d found for account: %s", len(secrets.Items), namespace)
	}

	creds, ok := secrets.Items[0].Data[types.DefaultSecretKeyName]
	if !ok {
		return "", fmt.Errorf("operator credentials secret key (%s) missing", types.DefaultSecretKeyName)
	}

	sysUserJwt, err := jwt.ParseDecoratedJWT(creds)
	if err != nil {
		return "", fmt.Errorf("couldn't parse system user JWT from creds: %w", err)
	}
	sysUserJwtClaims, err := jwt.DecodeUserClaims(sysUserJwt)
	if err != nil {
		return "", fmt.Errorf("couldn't decode system user JWT claims: %w", err)
	}

	return sysUserJwtClaims.IssuerAccount, nil
}

func getAccountRootSecretName(accountName, accountID string) string {
	return fmt.Sprintf(SecretNameAccountRootTemplate, accountName, mustGenerateShortHashFromID(accountID))
}

func getAccountSignSecretName(accountName, accountID string) string {
	return fmt.Sprintf(SecretNameAccountSignTemplate, accountName, mustGenerateShortHashFromID(accountID))
}

func mustGenerateShortHashFromID(ID string) string {
	hasher := md5.New()
	_, err := io.WriteString(hasher, ID)
	if err != nil {
		panic(fmt.Sprintf("failed to generate hash from ID: %v", err))
	}

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
				account := importAccount.Labels[types.LabelAccountID]
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
