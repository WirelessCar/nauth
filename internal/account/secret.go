package account

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Secrets struct {
	Root nkeys.KeyPair
	Sign nkeys.KeyPair
}

type SecretManager interface {
	ApplyRootSecret(ctx context.Context, namespace, accountName string, rootKeyPair nkeys.KeyPair) error
	ApplySignSecret(ctx context.Context, namespace, accountName, accountID string, signKeyPair nkeys.KeyPair) error
	DeleteAll(ctx context.Context, namespace, accountName, accountID string) error
	GetSecrets(ctx context.Context, namespace, accountName, accountID string) (*Secrets, error)
}

type secretManager struct {
	secretClient ports.SecretClient
}

func NewSecretManager(secretClient ports.SecretClient) SecretManager {
	if secretClient == nil {
		panic("secret client cannot be nil")
	}

	return &secretManager{
		secretClient: secretClient,
	}
}

func (m *secretManager) ApplyRootSecret(ctx context.Context, namespace, accountName string, rootKeyPair nkeys.KeyPair) error {
	accountID, err := rootKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get public key from account root secret: %w", err)
	}
	return m.applySecret(ctx, namespace, accountName, accountID, SecretNameAccountRootTemplate, k8s.SecretTypeAccountRoot, rootKeyPair)
}

func (m *secretManager) ApplySignSecret(ctx context.Context, namespace, accountName, accountID string, signKeyPair nkeys.KeyPair) error {
	return m.applySecret(ctx, namespace, accountName, accountID, SecretNameAccountSignTemplate, k8s.SecretTypeAccountSign, signKeyPair)
}

func (m *secretManager) applySecret(ctx context.Context, namespace, accountName, accountID, nameTemplate, secretType string, keyPair nkeys.KeyPair) error {
	if namespace == "" {
		return fmt.Errorf("account namespace cannot be empty")
	}
	if accountName == "" {
		return fmt.Errorf("account name cannot be empty")
	}
	if accountID == "" {
		return fmt.Errorf("account ID cannot be empty")
	}

	secretName := fmt.Sprintf(nameTemplate, accountName, mustGenerateShortHashFromID(accountID))
	secretMeta := metav1.ObjectMeta{
		Name:      secretName,
		Namespace: namespace,
		Labels: map[string]string{
			k8s.LabelAccountID:   accountID,
			k8s.LabelAccountName: accountName,
			k8s.LabelSecretType:  secretType,
			k8s.LabelManaged:     k8s.LabelManagedValue,
		},
	}
	seed, err := keyPair.Seed()
	if err != nil {
		return fmt.Errorf("failed to get seed from key pair: %w", err)
	}
	accountSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(seed)}

	if err = m.secretClient.Apply(ctx, nil, secretMeta, accountSecretValue); err != nil {
		return fmt.Errorf("unable to apply secret: %w", err)
	}
	return nil
}

func (m *secretManager) DeleteAll(ctx context.Context, namespace, accountName, accountID string) error {
	labels := map[string]string{
		k8s.LabelAccountID:   accountID,
		k8s.LabelAccountName: accountName,
	}
	// TODO: Consider looking up secrets and then deleting them explicitly
	// or delete first by only AccountID label and then by AccountName label, to ensure secrets are deleted even if
	// they are not labelled correctly. This also allows for better error handling and logging of which secrets were
	// attempted to be deleted.
	// TODO: Consider secrets labelled nauth.io/managed=true, should we only delete those?
	return m.secretClient.DeleteByLabels(ctx, namespace, labels)
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

func (m *secretManager) GetSecrets(ctx context.Context, namespace, accountName, accountID string) (*Secrets, error) {
	var err error
	if accountID != "" {
		secretsByAccountID, errByAccountID := m.getAccountSecretsByAccountID(ctx, namespace, accountID)
		if errByAccountID == nil {
			return m.validatedResult(secretsByAccountID, accountID)
		}
		err = fmt.Errorf("failed to get account secrets by account ID %q: %w", accountID, errByAccountID)
	}

	secretsByAccountName, errByAccountName := m.getAccountSecretsByAccountName(ctx, namespace, accountName)
	if errByAccountName == nil {
		return m.validatedResult(secretsByAccountName, accountID)
	}
	err = errors.Join(err, fmt.Errorf("failed to get account secrets by account name %q: %w", accountName, errByAccountName))

	secretsBySecretName, errBySecretName := m.getDeprecatedAccountSecretsByName(ctx, namespace, accountName, accountID)
	if errBySecretName == nil {
		return m.validatedResult(secretsBySecretName, accountID)
	}
	err = errors.Join(err, fmt.Errorf("failed to get account secrets by secret name (deprecated) for account name %q: %w", accountName, errBySecretName))

	return nil, err
}

func (m *secretManager) validatedResult(result *Secrets, accountID string) (*Secrets, error) {
	rootPublicKey, err := result.Root.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("unable to validate result, failed to get public key from account root secret: %w", err)
	}
	if accountID != "" && rootPublicKey != accountID {
		return nil, fmt.Errorf("account root public key (%s) in found secret does not match expected account ID (%s)", rootPublicKey, accountID)
	}
	return result, nil
}

func (m *secretManager) getAccountSecretsByAccountID(ctx context.Context, namespace, accountID string) (*Secrets, error) {
	labels := map[string]string{
		k8s.LabelAccountID: accountID,
		k8s.LabelManaged:   k8s.LabelManagedValue,
	}
	k8sSecrets, err := m.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	return m.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (m *secretManager) getAccountSecretsByAccountName(ctx context.Context, namespace, accountName string) (*Secrets, error) {
	labels := map[string]string{
		k8s.LabelAccountName: accountName,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}
	k8sSecrets, err := m.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	return m.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (m *secretManager) getAccountSecretsFromK8sSecrets(k8sSecrets *v1.SecretList) (*Secrets, error) {
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

	return m.toAccountSecrets(secrets)
}

func (m *secretManager) getDeprecatedAccountSecretsByName(ctx context.Context, namespace, accountName, accountID string) (*Secrets, error) {
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
					result.err = fmt.Errorf("recovered panicked go routine from trying to get secret %s/%s of type %s: %v", namespace, secretName, secretType, r)
					ch <- result
				}
			}()

			accountSecret, err := m.secretClient.Get(ctx, namespace, secretName)
			if err != nil {
				result.err = err
				ch <- result
				return
			}

			labels := map[string]string{
				k8s.LabelAccountID:  accountID, // TODO: We are not adding AccountName label, hence those secrets will currently not be deleted on DeleteAll
				k8s.LabelSecretType: secretType,
				k8s.LabelManaged:    k8s.LabelManagedValue,
			}
			if err := m.secretClient.Label(ctx, namespace, secretName, labels); err != nil {
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
		return nil, fmt.Errorf("missing one or more deprecated secret(s) for account: %s/%s", namespace, accountName)
	}

	return m.toAccountSecrets(secrets)
}

func (m *secretManager) toAccountSecrets(secrets map[string]map[string]string) (*Secrets, error) {
	root, err := m.toKeyPair(secrets, k8s.SecretTypeAccountRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve account root key pair: %w", err)
	}
	sign, err := m.toKeyPair(secrets, k8s.SecretTypeAccountSign)
	if err != nil {
		return nil, fmt.Errorf("resolve account signing key pair: %w", err)
	}

	return &Secrets{
		Root: root,
		Sign: sign,
	}, nil
}

func (m *secretManager) toKeyPair(secrets map[string]map[string]string, secretType string) (nkeys.KeyPair, error) {
	secret, ok := secrets[secretType]
	if !ok {
		return nil, fmt.Errorf("secret of type '%s' not found", secretType)
	}
	seed, ok := secret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret of type '%s' does not contain key '%s'", secretType, k8s.DefaultSecretKeyName)
	}
	keyPair, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return nil, fmt.Errorf("create key pair from secret of type '%s': %w", secretType, err)
	}
	return keyPair, nil
}
