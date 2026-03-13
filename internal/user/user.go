package user

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Manager struct {
	accountsReader ports.AccountReader
	secretClient   ports.SecretClient
}

func NewManager(accountsReader ports.AccountReader, secretClient ports.SecretClient) *Manager {
	return &Manager{
		accountsReader: accountsReader,
		secretClient:   secretClient,
	}
}

func (u *Manager) CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error {
	userRef := domain.NewNamespacedName(state.Namespace, state.Name)
	accountRef := domain.NewNamespacedName(state.Namespace, state.Spec.AccountName)
	if err := accountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}
	account, err := u.accountsReader.Get(ctx, accountRef)
	if err != nil {
		return err
	}

	accountID := account.GetLabels()[k8s.LabelAccountID]
	if accountID == "" {
		return fmt.Errorf("account %s does not have an account ID yet", accountRef)
	}
	accountSigningKeyPair, err := u.getAccountSigningKeyPair(ctx, accountRef, accountID)
	if err != nil {
		return fmt.Errorf("failed to get signing key secret %s: %w", accountRef, err)
	}
	accountSigningKeyPublicKey, err := accountSigningKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get account signing public key: %w", err)
	}

	userKeyPair, err := nkeys.CreateUser()
	if err != nil {
		return fmt.Errorf("failed to create user key pair: %w", err)
	}
	userPublicKey, err := userKeyPair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get user public key: %w", err)
	}
	userSeed, err := userKeyPair.Seed()
	if err != nil {
		return fmt.Errorf("failed to get user seed: %w", err)
	}

	natsClaims := newClaimsBuilder(getDisplayName(state), state.Spec, userPublicKey, accountID).
		build()
	userJwt, err := natsClaims.Encode(accountSigningKeyPair)
	if err != nil {
		return fmt.Errorf("failed to sign user jwt for %s: %w", userRef, err)
	}

	userCreds, err := jwt.FormatUserConfig(userJwt, userSeed)
	if err != nil {
		return fmt.Errorf("failed to format user credentials: %w", err)
	}

	secretMeta := metav1.ObjectMeta{
		Name:      state.GetUserSecretName(),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelSecretType: k8s.SecretTypeUserCredentials,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	secretValue := map[string]string{
		k8s.UserCredentialSecretKeyName: string(userCreds),
	}
	err = u.secretClient.Apply(ctx, state, secretMeta, secretValue)
	if err != nil {
		return err
	}

	toNAuthUserClaims(natsClaims)
	state.Status.Claims = toNAuthUserClaims(natsClaims)

	if state.Labels == nil {
		state.Labels = make(map[string]string, 3)
	}

	state.GetLabels()[k8s.LabelUserID] = userPublicKey
	state.GetLabels()[k8s.LabelUserAccountID] = account.GetLabels()[k8s.LabelAccountID]
	state.GetLabels()[k8s.LabelUserSignedBy] = accountSigningKeyPublicKey

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (u *Manager) Delete(ctx context.Context, state *v1alpha1.User) error {
	log := logf.FromContext(ctx)
	log.Info("Delete user", "userName", state.GetName())

	secretRef := domain.NewNamespacedName(state.Namespace, state.GetUserSecretName())
	if err := secretRef.Validate(); err != nil {
		return fmt.Errorf("invalid secret reference %q: %w", secretRef, err)
	}
	err := u.secretClient.Delete(ctx, secretRef)
	if err != nil {
		return fmt.Errorf("failed to delete user secret %s: %w", secretRef, err)
	}

	return nil
}

func (u *Manager) getAccountSigningKeyPair(ctx context.Context, accountRef domain.NamespacedName, accountID string) (nkeys.KeyPair, error) {
	if keyPair, err := u.getAccountSigningKeyPairByAccountID(ctx, accountRef, accountID); err == nil {
		return keyPair, nil
	}

	keyPair, err := u.getDeprecatedAccountSigningKeyPair(ctx, accountRef, accountID)
	if err != nil {
		return nil, err
	}

	return keyPair, nil
}

func (u *Manager) getAccountSigningKeyPairByAccountID(ctx context.Context, accountRef domain.NamespacedName, accountID string) (nkeys.KeyPair, error) {
	labels := map[string]string{
		k8s.LabelAccountID:  accountID,
		k8s.LabelSecretType: k8s.SecretTypeAccountSign,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	}
	secrets, err := u.secretClient.GetByLabels(ctx, accountRef.GetNamespace(), labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing secret for account: %s due to %w", accountRef, err)
	}

	if len(secrets.Items) < 1 {
		return nil, fmt.Errorf("no signing secret found for account: %s", accountRef)
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("more than 1 signing secret found for account: %s", accountRef)
	}

	seed, ok := secrets.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for user credentials seed was malformed")
	}
	return nkeys.FromSeed(seed)
}

func getDisplayName(user *v1alpha1.User) string {
	if user.Spec.DisplayName != "" {
		return user.Spec.DisplayName
	}
	return fmt.Sprintf("%s/%s", user.GetNamespace(), user.GetName())
}

// Todo: Almost identical to the one in account/account.go - refactor ?
func (u *Manager) getDeprecatedAccountSigningKeyPair(ctx context.Context, accountRef domain.NamespacedName, accountID string) (nkeys.KeyPair, error) {
	logger := logf.FromContext(ctx)

	type goRoutineResult struct {
		secret map[string]string
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

			accountSecret, err := u.secretClient.Get(ctx, secretRef)
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
			if err := u.secretClient.Label(ctx, secretRef, labels); err != nil {
				logger.Info("unable to label secret", "secretRef", secretRef, "secretType", secretType, "error", err)
			}
			accountSecret[k8s.LabelSecretType] = secretType
			result.secret = accountSecret
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
		secrets[res.secret[k8s.LabelSecretType]] = res.secret
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	accountSignSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("no deprecated signing key found for account %s", accountRef)
	}

	accountSignSecretSeed, ok := accountSignSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("no deprecated signing key seed found for account %s", accountRef)
	}
	return nkeys.FromSeed([]byte(accountSignSecretSeed))
}
