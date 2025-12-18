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
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretClient interface {
	// TODO: Keys created should be immutable
	Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error
	Get(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
	Delete(ctx context.Context, namespace string, name string) error
	DeleteByLabels(ctx context.Context, namespace string, labels map[string]string) error
	Label(ctx context.Context, namespace, name string, labels map[string]string) error
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

type Manager struct {
	accounts       AccountGetter
	natsClient     NatsClient
	secretClient   SecretClient
	nauthNamespace string
}

func NewManager(accounts AccountGetter, natsClient NatsClient, secretClient SecretClient, opts ...func(*Manager)) *Manager {
	manager := &Manager{
		accounts:     accounts,
		natsClient:   natsClient,
		secretClient: secretClient,
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

func WithNamespace(namespace string) func(*Manager) {
	return func(manager *Manager) {
		manager.nauthNamespace = namespace
	}
}

func (a *Manager) valid() bool {
	if a.accounts == nil {
		return false
	}

	if a.natsClient == nil {
		return false
	}

	if a.secretClient == nil {
		return false
	}

	if a.nauthNamespace == "" {
		return false
	}

	return true
}

func (a *Manager) Create(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	accountKeyPair, _ := nkeys.CreateAccount()
	accountPublicKey, _ := accountKeyPair.PublicKey()
	accountSecretMeta := metav1.ObjectMeta{
		Name:      getAccountRootSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelAccountID:  accountPublicKey,
			k8s.LabelSecretType: k8s.SecretTypeAccountRoot,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	accountSeed, _ := accountKeyPair.Seed()
	accountSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(accountSeed)}

	if err := a.secretClient.Apply(ctx, nil, accountSecretMeta, accountSecretValue); err != nil {
		return nil, err
	}

	accountSigningKeyPair, _ := nkeys.CreateAccount()
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()
	accountSigningSecretMeta := metav1.ObjectMeta{
		Name:      getAccountSignSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelAccountID:  accountPublicKey,
			k8s.LabelSecretType: k8s.SecretTypeAccountSign,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	accountSigningSeed, _ := accountSigningKeyPair.Seed()
	accountSignSeedSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(accountSigningSeed)}

	if err := a.secretClient.Apply(ctx, nil, accountSigningSecretMeta, accountSignSeedSecretValue); err != nil {
		return nil, err
	}

	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	signedJwt, err := newClaimsBuilder(state, accountPublicKey).
		accountLimits().
		natsLimits().
		jetStreamLimits().
		exports().
		imports(ctx, a.accounts).
		signingKey(accountSigningPublicKey).
		encode(operatorSigningKeyPair)
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()
	err = a.natsClient.UploadAccountJWT(signedJwt)
	if err != nil {
		return nil, fmt.Errorf("failed to upload account jwt: %w", err)
	}

	// Return immutable result - controller will apply to state
	return &controller.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: operatorSigningPublicKey,
		Claims: &natsv1alpha1.AccountClaims{
			AccountLimits:   state.Spec.AccountLimits,
			Exports:         state.Spec.Exports,
			Imports:         state.Spec.Imports,
			JetStreamLimits: state.Spec.JetStreamLimits,
			NatsLimits:      state.Spec.NatsLimits,
		},
	}, nil
}

func (a *Manager) Update(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]
	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return nil, err
	}

	if sysAccountID, err := a.getSystemAccountID(ctx, a.nauthNamespace); err != nil || sysAccountID == accountID {
		if err != nil {
			return nil, fmt.Errorf("failed to get system account ID: %w", err)
		}

		return nil, fmt.Errorf("updating system account is not supported, consider '%s: %s'", k8s.LabelManagementPolicy, k8s.LabelManagementPolicyObserveValue)
	}

	accountSecret, ok := secrets[k8s.SecretTypeAccountRoot]
	if !ok {
		return nil, fmt.Errorf("secret for account not found")
	}
	accountSeed, ok := accountSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for account was malformed")
	}
	accountKeyPair, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair from seed: %w", err)
	}
	accountPublicKey, _ := accountKeyPair.PublicKey()

	accountSigningSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("secret for account signing not found")
	}
	accountSigningSeed, ok := accountSigningSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for account signing was malformed")
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair for signing from seed: %w", err)
	}
	accountSigningPublicKey, _ := accountSigningKeyPair.PublicKey()

	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	signedJwt, err := newClaimsBuilder(state, accountPublicKey).
		accountLimits().
		natsLimits().
		jetStreamLimits().
		exports().
		imports(ctx, a.accounts).
		signingKey(accountSigningPublicKey).
		encode(operatorSigningKeyPair)
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to sign account jwt for %s: %w", accountName, err)
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()

	err = a.natsClient.UploadAccountJWT(signedJwt)
	if err != nil {
		return nil, fmt.Errorf("failed to upload account jwt: %w", err)
	}

	// Return immutable result - controller will apply to state
	return &controller.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims: &natsv1alpha1.AccountClaims{
			AccountLimits:   state.Spec.AccountLimits,
			Exports:         state.Spec.Exports,
			Imports:         state.Spec.Imports,
			JetStreamLimits: state.Spec.JetStreamLimits,
			NatsLimits:      state.Spec.NatsLimits,
		},
	}, nil
}

func (a *Manager) Import(ctx context.Context, state *natsv1alpha1.Account) (*controller.AccountResult, error) {
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorSigningPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountID := state.GetLabels()[k8s.LabelAccountID]
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s", state.GetName())
	}

	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for account %s: %w", accountID, err)
	}

	accountRootSecret, ok := secrets[k8s.SecretTypeAccountRoot]
	if !ok {
		return nil, fmt.Errorf("account root secret not found for account %s", accountID)
	}
	accountRootSeed, ok := accountRootSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("account root seed secret for account %s is malformed", accountID)
	}
	accountRootKeyPair, err := nkeys.FromSeed([]byte(accountRootSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair for account %s from seed: %w", accountID, err)
	}
	accountRootPublicKey, err := accountRootKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair for account %s from seed: %w", accountID, err)
	}
	if accountRootPublicKey != accountID {
		return nil, fmt.Errorf("account root seed does not match account ID: expected %s, got %s", accountID, accountRootPublicKey)
	}

	accountSigningSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("account sign secret not found for account %s", accountID)
	}
	accountSigningSeed, ok := accountSigningSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("account sign secret for account %s is malformed", accountID)
	}
	_, err = nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair for signing from seed for account %s: %w", accountID, err)
	}

	err = a.natsClient.EnsureConnected(a.nauthNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer a.natsClient.Disconnect()
	accountJWT, err := a.natsClient.LookupAccountJWT(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup account jwt for account %s: %w", accountID, err)
	}
	if len(accountJWT) == 0 {
		return nil, fmt.Errorf("account jwt for account %s not found", accountID)
	}
	accountClaims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to decode account jwt for account %s: %w", accountID, err)
	}

	// Return immutable result - controller will apply to state
	claims := convertNatsAccountClaims(accountClaims)
	return &controller.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &claims,
	}, nil
}

func (a *Manager) Delete(ctx context.Context, state *natsv1alpha1.Account) error {
	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
	operatorSigningKeyPair, err := a.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}
	operatorPublicKey, _ := operatorSigningKeyPair.PublicKey()

	accountID := state.GetLabels()[k8s.LabelAccountID]

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
		k8s.LabelAccountID: accountID,
	}

	return a.secretClient.DeleteByLabels(ctx, state.GetNamespace(), labels)
}

func (a *Manager) getOperatorSigningKeyPair(ctx context.Context) (nkeys.KeyPair, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeOperatorSign,
	}
	operatorSecret, err := a.secretClient.GetByLabels(ctx, a.nauthNamespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key: %w", err)
	}

	if len(operatorSecret.Items) < 1 {
		return nil, fmt.Errorf("missing operator signing key secret, make sure to label the secret with the label %s: %s", k8s.LabelSecretType, k8s.SecretTypeOperatorSign)
	}

	if len(operatorSecret.Items) > 1 {
		return nil, fmt.Errorf("multiple operator signing key secrets found, make sure only one secret has the label %s: %s", k8s.LabelSecretType, k8s.SecretTypeSystemAccountUserCreds)
	}

	seed, ok := operatorSecret.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for operator signing key seed was malformed")
	}

	return nkeys.FromSeed(seed)
}

func (a *Manager) getAccountSecrets(ctx context.Context, namespace, accountID, accountName string) (map[string]map[string]string, error) {
	if secrets, err := a.getAccountSecretsByAccountID(ctx, namespace, accountName, accountID); err == nil {
		return secrets, nil
	}

	secrets, err := a.getDeprecatedAccountSecretsByName(ctx, namespace, accountName, accountID)
	if err != nil {
		return nil, err
	}

	return secrets, nil
}

func (a *Manager) getAccountSecretsByAccountID(ctx context.Context, namespace, accountName, accountID string) (map[string]map[string]string, error) {
	labels := map[string]string{
		k8s.LabelAccountID: accountID,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}
	k8sSecrets, err := a.secretClient.GetByLabels(ctx, namespace, labels)
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
		secretType := secret.GetLabels()[k8s.LabelSecretType]
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

// Todo: Almost identical to the one in user/user.go - refactor ?
func (a *Manager) getDeprecatedAccountSecretsByName(ctx context.Context, namespace, accountName, accountID string) (map[string]map[string]string, error) {
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
			secretName: fmt.Sprintf(k8s.DeprecatedSecretNameAccountRootTemplate, accountName),
			secretType: k8s.SecretTypeAccountRoot,
		},
		{
			secretName: fmt.Sprintf(k8s.DeprecatedSecretNameAccountSignTemplate, accountName),
			secretType: k8s.SecretTypeAccountSign,
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

			accountSecret, err := a.secretClient.Get(ctx, namespace, secretName)
			if err != nil {
				result.err = err
				ch <- result
				return
			}

			labels := map[string]string{
				k8s.LabelAccountID:  accountID,
				k8s.LabelSecretType: secretType,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			if err := a.secretClient.Label(ctx, namespace, secretName, labels); err != nil {
				logger.Info("unable to label secret", "secretName", secretName, "namespace", namespace, "secretType", secretType, "error", err)
			}
			accountSecret[k8s.LabelSecretType] = secretType
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
		secrets[res.secret[k8s.LabelSecretType]] = res.secret
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	if len(secrets) < 2 {
		return nil, fmt.Errorf("missing one or more deprecated secret(s) for account: %s-%s", namespace, accountName)
	}

	return secrets, nil
}

func (a *Manager) getSystemAccountID(ctx context.Context, namespace string) (string, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
	}
	secrets, err := a.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return "", fmt.Errorf("unable to get system account creds by labels: %w", err)
	}
	if len(secrets.Items) != 1 {
		return "", fmt.Errorf("single system account creds secret required, %d found for account: %s", len(secrets.Items), namespace)
	}

	creds, ok := secrets.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return "", fmt.Errorf("operator credentials secret key (%s) missing", k8s.DefaultSecretKeyName)
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
