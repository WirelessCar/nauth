package core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	SecretLabelAccountID   = "account.nauth.io/id"
	SecretLabelAccountName = "account.nauth.io/name"
)

type Secrets struct {
	Root nkeys.KeyPair
	Sign nkeys.KeyPair
}

type secretManager interface {
	ApplyRootSecret(ctx context.Context, accountRef domain.NamespacedName, rootKeyPair nkeys.KeyPair) error
	ApplySignSecret(ctx context.Context, accountRef domain.NamespacedName, accountID string, signKeyPair nkeys.KeyPair) error
	DeleteAll(ctx context.Context, accountRef domain.NamespacedName, accountID string) error
	GetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string) (*Secrets, bool, error)
}

type secretManagerImpl struct {
	secretClient outbound.SecretClient
}

func newSecretManagerImpl(secretClient outbound.SecretClient) (*secretManagerImpl, error) {
	if secretClient == nil {
		return nil, fmt.Errorf("secret client is required")
	}

	return &secretManagerImpl{
		secretClient: secretClient,
	}, nil
}

func (m *secretManagerImpl) ApplyRootSecret(ctx context.Context, accountRef domain.NamespacedName, rootKeyPair nkeys.KeyPair) error {
	accountID, err := rootKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get public key from account root secret: %w", err)
	}
	return m.applyAccountSecret(ctx, accountRef, accountID, SecretNameAccountRootTemplate, k8s.SecretTypeAccountRoot, rootKeyPair)
}

func (m *secretManagerImpl) ApplySignSecret(ctx context.Context, accountRef domain.NamespacedName, accountID string, signKeyPair nkeys.KeyPair) error {
	return m.applyAccountSecret(ctx, accountRef, accountID, SecretNameAccountSignTemplate, k8s.SecretTypeAccountSign, signKeyPair)
}

func (m *secretManagerImpl) applyAccountSecret(ctx context.Context, accountRef domain.NamespacedName, accountID, nameTemplate, secretType string, keyPair nkeys.KeyPair) error {
	if err := accountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference %s: %w", accountRef, err)
	}
	if accountID == "" {
		return fmt.Errorf("account ID cannot be empty")
	}

	secretName := fmt.Sprintf(nameTemplate, accountRef.Name, mustGenerateShortHashFromID(accountID))
	secretMeta := metav1.ObjectMeta{
		Name:      secretName,
		Namespace: accountRef.Namespace,
		Labels: map[string]string{
			SecretLabelAccountID:   accountID,
			SecretLabelAccountName: accountRef.Name,
			k8s.LabelSecretType:    secretType,
			k8s.LabelManaged:       k8s.LabelManagedValue,
		},
	}
	seed, err := keyPair.Seed()
	if err != nil {
		return fmt.Errorf("failed to get seed from key pair: %w", err)
	}
	accountSecretValue := map[string]string{k8s.DefaultSecretKeyName: string(seed)}

	// Intentionally do not set an owner reference on account secrets. If the Account resource is deleted by mistake,
	// the secrets should remain so the same account can be recreated from the preserved root seed.
	if err = m.secretClient.Apply(ctx, nil, secretMeta, accountSecretValue); err != nil {
		return fmt.Errorf("unable to apply secret: %w", err)
	}
	return nil
}

func (m *secretManagerImpl) DeleteAll(ctx context.Context, accountRef domain.NamespacedName, accountID string) error {
	if err := accountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference %s: %w", accountRef, err)
	}

	labels := map[string]string{
		SecretLabelAccountID:   accountID,
		SecretLabelAccountName: accountRef.Name,
	}
	// TODO: Consider looking up secrets and then deleting them explicitly
	// or delete first by only AccountID label and then by AccountName label, to ensure secrets are deleted even if
	// they are not labelled correctly. This also allows for better error handling and logging of which secrets were
	// attempted to be deleted.
	// TODO: Consider secrets labelled nauth.io/managed=true, should we only delete those?
	return m.secretClient.DeleteByLabels(ctx, accountRef.GetNamespace(), labels)
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

func (m *secretManagerImpl) GetSecrets(ctx context.Context, accountRef domain.NamespacedName, accountID string) (*Secrets, bool, error) {
	var err error
	if err = accountRef.Validate(); err != nil {
		return nil, false, fmt.Errorf("invalid account reference %s: %w", accountRef, err)
	}
	if accountID != "" {
		secretsByAccountID, found, errByAccountID := m.getAccountSecretsByAccountID(ctx, accountRef.GetNamespace(), accountID)
		if errByAccountID == nil && found {
			result, err := m.validatedResult(secretsByAccountID, accountID)
			return result, true, err
		}
		if errByAccountID != nil {
			err = fmt.Errorf("failed to get account secrets by account ID %q: %w", accountID, errByAccountID)
		}
	}

	secretsByAccountName, found, errByAccountName := m.getAccountSecretsByAccountName(ctx, accountRef)
	if errByAccountName == nil && found {
		result, err := m.validatedResult(secretsByAccountName, accountID)
		return result, true, err
	}
	if errByAccountName != nil {
		err = errors.Join(err, fmt.Errorf("failed to get account secrets by account name %q: %w", accountRef.Name, errByAccountName))
	}

	secretsBySecretName, found, errBySecretName := m.getDeprecatedAccountSecretsByName(ctx, accountRef, accountID)
	if errBySecretName == nil && found {
		result, err := m.validatedResult(secretsBySecretName, accountID)
		return result, true, err
	}
	if errBySecretName != nil {
		err = errors.Join(err, fmt.Errorf("failed to get account secrets by secret name (deprecated) for account name %q: %w", accountRef.Name, errBySecretName))
	}

	return nil, false, err
}

func (m *secretManagerImpl) validatedResult(result *Secrets, accountID string) (*Secrets, error) {
	rootPublicKey, err := result.Root.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("unable to validate result, failed to get public key from account root secret: %w", err)
	}
	if accountID != "" && rootPublicKey != accountID {
		return nil, fmt.Errorf("account root public key (%s) in found secret does not match expected account ID (%s)", rootPublicKey, accountID)
	}
	return result, nil
}

func (m *secretManagerImpl) getAccountSecretsByAccountID(ctx context.Context, namespace domain.Namespace, accountID string) (*Secrets, bool, error) {
	labels := map[string]string{
		SecretLabelAccountID: accountID,
		k8s.LabelManaged:     k8s.LabelManagedValue,
	}
	k8sSecrets, err := m.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, false, err
	}

	return m.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (m *secretManagerImpl) getAccountSecretsByAccountName(ctx context.Context, accountRef domain.NamespacedName) (*Secrets, bool, error) {
	labels := map[string]string{
		SecretLabelAccountName: accountRef.Name,
		k8s.LabelManaged:       k8s.LabelManagedValue,
	}
	k8sSecrets, err := m.secretClient.GetByLabels(ctx, accountRef.GetNamespace(), labels)
	if err != nil {
		return nil, false, err
	}

	return m.getAccountSecretsFromK8sSecrets(k8sSecrets)
}

func (m *secretManagerImpl) getAccountSecretsFromK8sSecrets(k8sSecrets *v1.SecretList) (*Secrets, bool, error) {
	if len(k8sSecrets.Items) != 2 {
		return nil, false, nil
	}

	secrets := make(map[string]map[string]string, len(k8sSecrets.Items))
	for _, secret := range k8sSecrets.Items {
		secretType := secret.GetLabels()[k8s.LabelSecretType]
		if _, ok := secrets[secretType]; ok {
			return nil, false, fmt.Errorf("multiple secrets of type '%s' found", secretType)
		}

		secretData := make(map[string]string, len(secret.Data))
		for k, v := range secret.Data {
			secretData[k] = string(v)
		}
		secrets[secretType] = secretData
	}

	result, err := m.toAccountSecrets(secrets)
	if err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func (m *secretManagerImpl) getDeprecatedAccountSecretsByName(ctx context.Context, accountRef domain.NamespacedName, accountID string) (*Secrets, bool, error) {
	logger := logf.FromContext(ctx)

	type goRoutineResult struct {
		secret map[string]string
		found  bool
		err    error
	}
	var wg sync.WaitGroup
	ch := make(chan goRoutineResult, 2)

	namespace := accountRef.GetNamespace()
	for _, s := range []struct {
		secretRef  domain.NamespacedName
		secretType string
	}{
		{
			secretRef:  namespace.WithName(fmt.Sprintf(k8s.DeprecatedSecretNameAccountRootTemplate, accountRef.Name)),
			secretType: k8s.SecretTypeAccountRoot,
		},
		{
			secretRef:  namespace.WithName(fmt.Sprintf(k8s.DeprecatedSecretNameAccountSignTemplate, accountRef.Name)),
			secretType: k8s.SecretTypeAccountSign,
		},
	} {
		wg.Add(1)
		go func(secretRef domain.NamespacedName, secretType string) {
			result := goRoutineResult{}
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					result.err = fmt.Errorf("recovered panicked go routine from trying to get secret %s of type %s: %v", secretRef, secretType, r)
					ch <- result
				}
			}()

			accountSecret, found, err := m.secretClient.Get(ctx, secretRef)
			if err != nil {
				result.err = err
				ch <- result
				return
			}
			if !found {
				ch <- result
				return
			}

			labels := map[string]string{
				SecretLabelAccountID: accountID, // TODO: We are not adding AccountName label, hence those secrets will currently not be deleted on DeleteAll
				k8s.LabelSecretType:  secretType,
				k8s.LabelManaged:     k8s.LabelManagedValue,
			}
			if err := m.secretClient.Label(ctx, secretRef, labels); err != nil {
				logger.Info("unable to label secret", "secretRef", secretRef, "namespace", namespace, "secretType", secretType, "error", err)
			}
			accountSecret[k8s.LabelSecretType] = secretType
			result.secret = accountSecret
			result.found = true
			ch <- result
		}(s.secretRef, s.secretType)
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
		if !res.found {
			continue
		}
		secrets[res.secret[k8s.LabelSecretType]] = res.secret
	}

	if len(errs) > 0 {
		return nil, false, errors.Join(errs...)
	}

	if len(secrets) < 2 {
		return nil, false, nil
	}

	result, err := m.toAccountSecrets(secrets)
	if err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func (m *secretManagerImpl) toAccountSecrets(secrets map[string]map[string]string) (*Secrets, error) {
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

func (m *secretManagerImpl) toKeyPair(secrets map[string]map[string]string, secretType string) (nkeys.KeyPair, error) {
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

var _ secretManager = (*secretManagerImpl)(nil)
