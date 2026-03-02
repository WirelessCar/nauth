package account

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Manager struct {
	clusterConfigResolver ClusterConfigResolver
	accountResolver       ports.NauthAccountResolver
	secretClient          ports.SecretClient
	natsClient            ports.NatsClient
}

func NewManager(
	clusterConfigResolver ClusterConfigResolver,
	accounts ports.NauthAccountResolver,
	secretClient ports.SecretClient,
	natsClient ports.NatsClient,
) (*Manager, error) {
	m := &Manager{
		clusterConfigResolver: clusterConfigResolver,
		accountResolver:       accounts,
		secretClient:          secretClient,
		natsClient:            natsClient,
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func (a *Manager) validate() error {
	if a.clusterConfigResolver == nil {
		return errors.New("cluster config resolver is required")
	}
	if a.accountResolver == nil {
		return errors.New("account resolver is required")
	}

	if a.secretClient == nil {
		return errors.New("secret client is required")
	}

	if a.natsClient == nil {
		return errors.New("NATS client is required")
	}

	return nil
}

func (a *Manager) Create(ctx context.Context, state *v1alpha1.Account) (*controller.AccountResult, error) {
	var accountPublicKey string
	var accountSigningPublicKey string
	var accountKeyPair nkeys.KeyPair
	var accountSigningKeyPair nkeys.KeyPair

	cluster, err := a.resolveClusterConfig(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster config: %w", err)
	}
	secrets, err := a.getAccountSecretsByAccountName(ctx, state.GetNamespace(), state.GetName())
	if err == nil {
		accountSecret := secrets[k8s.SecretTypeAccountRoot]
		if len(accountSecret) == 0 {
			return nil, fmt.Errorf("no account root secret found during creation: %w", err)
		}
		accountSeed := accountSecret[k8s.DefaultSecretKeyName]
		if len(accountSeed) == 0 {
			return nil, fmt.Errorf("no account root seed secret key '%s' found during creation: %w", k8s.DefaultSecretKeyName, err)
		}
		accountKeyPair, err = nkeys.FromSeed([]byte(accountSeed))
		if err != nil {
			return nil, fmt.Errorf("failed to get account key pair from existing seed during creation: %w", err)
		}
		accountPublicKey, err = accountKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get account public key from existing secret during creation: %w", err)
		}

		accountSigningSecret := secrets[k8s.SecretTypeAccountSign]
		if len(accountSigningSecret) == 0 {
			return nil, fmt.Errorf("no account signing secret found during creation: %w", err)
		}
		accountSigningSeed := accountSigningSecret[k8s.DefaultSecretKeyName]
		if len(accountSigningSeed) == 0 {
			return nil, fmt.Errorf("no account signing secret key '%s' found during creation: %w", k8s.DefaultSecretKeyName, err)
		}
		accountSigningKeyPair, err = nkeys.FromSeed([]byte(accountSigningSeed))
		if err != nil {
			return nil, fmt.Errorf("failed to get account signing key pair from existing seed during creation: %w", err)
		}
		accountSigningPublicKey, err = accountSigningKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get account signing public key from existing secret during creation: %w", err)
		}
	} else {
		accountKeyPair, err = nkeys.CreateAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to create account key pair during creation: %w", err)
		}
		accountPublicKey, err = accountKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get account public key during creation: %w", err)
		}

		accountSigningKeyPair, err = nkeys.CreateAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to create account signing key pair during creation: %w", err)
		}
		accountSigningPublicKey, err = accountSigningKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get account signing public key during creation: %w", err)
		}
	}

	accountSecretMeta := metav1.ObjectMeta{
		Name:      getAccountRootSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelAccountID:   accountPublicKey,
			k8s.LabelAccountName: state.GetName(),
			k8s.LabelSecretType:  k8s.SecretTypeAccountRoot,
			k8s.LabelManaged:     k8s.LabelManagedValue,
		},
	}
	accountSeed, err := accountKeyPair.Seed()
	if err != nil {
		return nil, fmt.Errorf("failed to get account seed during creation: %w", err)
	}
	accountSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(accountSeed)}

	if err := a.secretClient.Apply(ctx, nil, accountSecretMeta, accountSecretValue); err != nil {
		return nil, err
	}

	accountSigningSecretMeta := metav1.ObjectMeta{
		Name:      getAccountSignSecretName(state.GetName(), accountPublicKey),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelAccountID:   accountPublicKey,
			k8s.LabelAccountName: state.GetName(),
			k8s.LabelSecretType:  k8s.SecretTypeAccountSign,
			k8s.LabelManaged:     k8s.LabelManagedValue,
		},
	}
	accountSigningSeed, err := accountSigningKeyPair.Seed()
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing seed during creation: %w", err)
	}
	accountSignSeedSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(accountSigningSeed)}

	if err := a.secretClient.Apply(ctx, nil, accountSigningSecretMeta, accountSignSeedSecretValue); err != nil {
		return nil, err
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during creation: %w", err)
	}

	natsClaims, err := newClaimsBuilder(ctx, getDisplayName(state), state.Spec, accountPublicKey, a.accountResolver).
		signingKey(accountSigningPublicKey).
		build()
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to build NATS account claims for %s during creation: %w", accountName, err)
	}
	signedJwt, err := natsClaims.Encode(cluster.OperatorSigningKey)
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to sign account jwt for %s during creation: %w", accountName, err)
	}

	conn, err := a.natsClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster during creation: %w", err)
	}

	defer conn.Disconnect()
	err = conn.UploadAccountJWT(signedJwt)
	if err != nil {
		return nil, fmt.Errorf("failed to upload account jwt during creation: %w", err)
	}

	// Return immutable result - controller will apply to state
	nauthClaims := convertNatsAccountClaims(natsClaims)
	return &controller.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func (a *Manager) Update(ctx context.Context, state *v1alpha1.Account) (*controller.AccountResult, error) {
	cluster, err := a.resolveClusterConfig(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster config: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]
	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return nil, err
	}

	sysAccountID := cluster.SystemAdminCreds.AccountID
	if sysAccountID == accountID {
		return nil, fmt.Errorf("updating system account is not supported, consider '%s: %s'", k8s.LabelManagementPolicy, k8s.LabelManagementPolicyObserveValue)
	}

	accountSecret, ok := secrets[k8s.SecretTypeAccountRoot]
	if !ok {
		return nil, fmt.Errorf("existing secret for account root not found during update")
	}
	accountSeed, ok := accountSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("existing secret for account root was malformed during update")
	}
	accountKeyPair, err := nkeys.FromSeed([]byte(accountSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair from existing seed during update: %w", err)
	}
	accountPublicKey, err := accountKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account public key from existing seed during update: %w", err)
	}

	accountSigningSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("existing secret for account signing not found during update")
	}
	accountSigningSeed, ok := accountSigningSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("existing secret for account signing was malformed during update")
	}
	accountSigningKeyPair, err := nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing key pair from existing seed during update: %w", err)
	}
	accountSigningPublicKey, err := accountSigningKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing public key from existing seed during update: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during update: %w", err)
	}

	natsClaims, err := newClaimsBuilder(ctx, getDisplayName(state), state.Spec, accountPublicKey, a.accountResolver).
		signingKey(accountSigningPublicKey).
		build()
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to build NATS account claims for %s during update: %w", accountName, err)
	}
	signedJwt, err := natsClaims.Encode(cluster.OperatorSigningKey)
	if err != nil {
		accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to sign account jwt for %s during update: %w", accountName, err)
	}

	conn, err := a.natsClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster during update: %w", err)
	}
	defer conn.Disconnect()

	err = conn.UploadAccountJWT(signedJwt)
	if err != nil {
		return nil, fmt.Errorf("failed to upload account jwt during update: %w", err)
	}

	nauthClaims := convertNatsAccountClaims(natsClaims)
	return &controller.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func (a *Manager) Import(ctx context.Context, state *v1alpha1.Account) (*controller.AccountResult, error) {
	cluster, err := a.resolveClusterConfig(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster config: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during import: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s during import", state.GetName())
	}

	secrets, err := a.getAccountSecrets(ctx, state.GetNamespace(), accountID, state.GetName())
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for account %s during import: %w", accountID, err)
	}

	accountRootSecret, ok := secrets[k8s.SecretTypeAccountRoot]
	if !ok {
		return nil, fmt.Errorf("existing account root secret not found for account %s during import", accountID)
	}
	accountRootSeed, ok := accountRootSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("existing account root seed secret for account %s is malformed during import", accountID)
	}
	accountRootKeyPair, err := nkeys.FromSeed([]byte(accountRootSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account key pair for account %s from existing seed during import: %w", accountID, err)
	}
	accountRootPublicKey, err := accountRootKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account public key for account %s from existing seed during import: %w", accountID, err)
	}
	if accountRootPublicKey != accountID {
		return nil, fmt.Errorf("account root seed does not match account ID during import: expected %s, got %s", accountID, accountRootPublicKey)
	}

	accountSigningSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("existing account signing secret not found for account %s during import", accountID)
	}
	accountSigningSeed, ok := accountSigningSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("existing account signing secret for account %s is malformed during import", accountID)
	}
	_, err = nkeys.FromSeed([]byte(accountSigningSeed))
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing key pair from existing seed for account %s during import: %w", accountID, err)
	}

	conn, err := a.natsClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster during import: %w", err)
	}
	defer conn.Disconnect()
	accountJWT, err := conn.LookupAccountJWT(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup account jwt for account %s during import: %w", accountID, err)
	}
	if len(accountJWT) == 0 {
		return nil, fmt.Errorf("account jwt for account %s not found during import", accountID)
	}
	natsClaims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to decode account jwt for account %s during import: %w", accountID, err)
	}

	nauthClaims := convertNatsAccountClaims(natsClaims)
	return &controller.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func (a *Manager) Delete(ctx context.Context, state *v1alpha1.Account) error {
	cluster, err := a.resolveClusterConfig(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to resolve cluster config: %w", err)
	}

	accountName := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
	operatorPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get operator signing public key during deletion: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]

	// Delete is done by signing a jwt with a list of accounts to be deleted
	deleteClaim := jwt.NewGenericClaims(operatorPublicKey)
	deleteClaim.Data["accounts"] = []string{accountID}

	deleteJwt, err := deleteClaim.Encode(cluster.OperatorSigningKey)
	if err != nil {
		return fmt.Errorf("failed to sign account jwt for %s during deletion: %w", accountName, err)
	}

	conn, err := a.natsClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster during deletion: %w", err)
	}
	defer conn.Disconnect()

	err = conn.DeleteAccountJWT(deleteJwt)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	labels := map[string]string{
		k8s.LabelAccountID:   accountID,
		k8s.LabelAccountName: state.GetName(),
	}

	return a.secretClient.DeleteByLabels(ctx, state.GetNamespace(), labels)
}

func (a *Manager) resolveClusterConfig(ctx context.Context, account *v1alpha1.Account) (*ClusterConfig, error) {
	natsClusterRef := account.Spec.NatsClusterRef
	if natsClusterRef != nil && natsClusterRef.Namespace == "" {
		natsClusterRef = natsClusterRef.DeepCopy()
		natsClusterRef.Namespace = account.GetNamespace()
	}

	return a.clusterConfigResolver.GetClusterConfig(ctx, natsClusterRef)
}

func (a *Manager) getAccountSecrets(ctx context.Context, namespace, accountID, accountName string) (map[string]map[string]string, error) {
	secretsByAccountID, errByAccountID := a.getAccountSecretsByAccountID(ctx, namespace, accountID)
	if errByAccountID == nil {
		return secretsByAccountID, nil
	}
	err := fmt.Errorf("failed to get account secrets by id = %s: %w", accountID, errByAccountID)

	secretsByAccountName, errByAccountName := a.getAccountSecretsByAccountName(ctx, namespace, accountName)
	if errByAccountName == nil {
		return secretsByAccountName, nil
	}
	err = errors.Join(err, fmt.Errorf("failed to get account secrets by account name = %s: %w", accountName, errByAccountName))

	secretsBySecretName, errBySecretName := a.getDeprecatedAccountSecretsByName(ctx, namespace, accountName, accountID)
	if errBySecretName == nil {
		return secretsBySecretName, nil
	}
	err = errors.Join(err, fmt.Errorf("failed to get account secrets by secret name (deprecated) for account name = %s: %w", accountName, errBySecretName))

	return nil, err
}

func (a *Manager) getAccountSecretsByAccountID(ctx context.Context, namespace, accountID string) (map[string]map[string]string, error) {
	labels := map[string]string{
		k8s.LabelAccountID: accountID,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}
	k8sSecrets, err := a.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	return a.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (a *Manager) getAccountSecretsByAccountName(ctx context.Context, namespace, accountName string) (map[string]map[string]string, error) {
	labels := map[string]string{
		k8s.LabelAccountName: accountName,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}
	k8sSecrets, err := a.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	return a.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (a *Manager) getAccountSecretsFromK8sSecrets(k8sSecrets *v1.SecretList) (map[string]map[string]string, error) {
	if len(k8sSecrets.Items) != 2 {
		return nil, fmt.Errorf("expected 2 secrets, got %d", len(k8sSecrets.Items))
	}

	secrets := make(map[string]map[string]string, len(k8sSecrets.Items))
	for _, secret := range k8sSecrets.Items {
		secretType := secret.GetLabels()[k8s.LabelSecretType]
		if _, ok := secrets[secretType]; ok {
			return nil, fmt.Errorf("multiple secrets of type '%s' found", secretType)
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

func getAccountRootSecretName(accountName, accountID string) string {
	return fmt.Sprintf(SecretNameAccountRootTemplate, accountName, mustGenerateShortHashFromID(accountID))
}

func getAccountSignSecretName(accountName, accountID string) string {
	return fmt.Sprintf(SecretNameAccountSignTemplate, accountName, mustGenerateShortHashFromID(accountID))
}

func getDisplayName(account *v1alpha1.Account) string {
	if account.Spec.DisplayName != "" {
		return account.Spec.DisplayName
	}
	return fmt.Sprintf("%s/%s", account.GetNamespace(), account.GetName())
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

// Compile-time assertion that Manager implements the controller.AccountManager interface
var _ controller.AccountManager = (*Manager)(nil)
