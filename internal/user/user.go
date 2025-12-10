package user

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/types"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretStorer interface {
	ApplySecret(ctx context.Context, owner *k8s.SecretOwner, meta metav1.ObjectMeta, valueMap map[string]string) error
	GetSecret(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
	DeleteSecret(ctx context.Context, namespace string, name string) error
	LabelSecret(ctx context.Context, namespace, name string, labels map[string]string) error
}

type AccountGetter interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error)
}

type UserManager struct {
	accounts     AccountGetter
	secretStorer SecretStorer
}

func NewUserManager(accounts AccountGetter, secretStorer SecretStorer) *UserManager {
	return &UserManager{
		accounts:     accounts,
		secretStorer: secretStorer,
	}
}

func (u *UserManager) CreateOrUpdateUser(ctx context.Context, state *v1alpha1.User) error {
	account, err := u.accounts.Get(ctx, state.Spec.AccountName, state.Namespace)
	if err != nil {
		return err
	}

	accountID := account.GetLabels()[types.LabelAccountID]
	accountSigningKeyPair, err := u.getAccountSigningKeyPair(ctx, account.GetNamespace(), account.GetName(), accountID)
	if err != nil {
		return fmt.Errorf("failed to get signing key secret %s/%s: %w", account.GetNamespace(), account.GetName(), err)
	}
	accountSigningKeyPublicKey, _ := accountSigningKeyPair.PublicKey()

	userKeyPair, _ := nkeys.CreateUser()
	userPublicKey, _ := userKeyPair.PublicKey()
	userSeed, _ := userKeyPair.Seed()

	userJwt, err := newUserClaimsBuilder(state, userPublicKey).
		issuerAccount(*account).
		natsLimits().
		permissions().
		userLimits().
		encode(accountSigningKeyPair)
	if err != nil {
		return err
	}

	userCreds, _ := jwt.FormatUserConfig(userJwt, userSeed)

	secretOwner := &k8s.SecretOwner{
		Owner: state,
	}
	secretMeta := metav1.ObjectMeta{
		Name:      state.GetUserSecretName(),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			types.LabelSecretType: types.SecretTypeUserCredentials,
			types.LabelManaged:    types.LabelManagedValue,
		},
	}
	secretValue := map[string]string{
		types.UserCredentialSecretKeyName: string(userCreds),
	}
	err = u.secretStorer.ApplySecret(ctx, secretOwner, secretMeta, secretValue)
	if err != nil {
		return err
	}

	state.Status.Claims = v1alpha1.UserClaims{
		AccountName: state.Spec.AccountName,
		NatsLimits:  state.Spec.NatsLimits,
		Permissions: state.Spec.Permissions,
		UserLimits:  state.Spec.UserLimits,
	}

	if state.Labels == nil {
		state.Labels = make(map[string]string, 3)
	}

	state.GetLabels()[types.LabelUserID] = userPublicKey
	state.GetLabels()[types.LabelUserAccountID] = account.GetLabels()[types.LabelAccountID]
	state.GetLabels()[types.LabelUserSignedBy] = accountSigningKeyPublicKey

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

func (u *UserManager) DeleteUser(ctx context.Context, state *v1alpha1.User) error {
	log := logf.FromContext(ctx)
	log.Info("Delete user", "userName", state.GetName())

	err := u.secretStorer.DeleteSecret(ctx, state.Namespace, state.GetUserSecretName())
	if err != nil {
		return fmt.Errorf("failed to delete user secret %s/%s: %w", state.Namespace, state.GetUserSecretName(), err)
	}

	return nil
}

func (u *UserManager) getAccountSigningKeyPair(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
	if keyPair, err := u.getAccountSigningKeyPairByAccountID(ctx, namespace, accountName, accountID); err == nil {
		return keyPair, nil
	}

	keyPair, err := u.getDeprecatedAccountSigningKeyPair(ctx, namespace, accountName, accountID)
	if err != nil {
		return nil, err
	}

	return keyPair, nil
}

func (u *UserManager) getAccountSigningKeyPairByAccountID(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
	labels := map[string]string{
		types.LabelAccountID:  accountID,
		types.LabelSecretType: types.SecretTypeAccountSign,
		types.LabelManaged:    types.LabelManagedValue,
	}
	secrets, err := u.secretStorer.GetSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing secret for account: %s-%s due to %w", namespace, accountName, err)
	}

	if len(secrets.Items) < 1 {
		return nil, fmt.Errorf("no signing secret found for account: %s-%s", namespace, accountName)
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("more than 1 signing secret found for account: %s-%s", namespace, accountName)
	}

	seed, ok := secrets.Items[0].Data[types.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for user credentials seed was malformed")
	}
	return nkeys.FromSeed(seed)
}

// Todo: Almost identical to the one in account/account.go - refactor ?
func (u *UserManager) getDeprecatedAccountSigningKeyPair(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
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

			accountSecret, err := u.secretStorer.GetSecret(ctx, namespace, secretName)
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
			if err := u.secretStorer.LabelSecret(ctx, namespace, secretName, labels); err != nil {
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

	accountSignSecret, ok := secrets[types.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("no signing key found for account %s-%s", namespace, accountName)
	}

	accountSignSecretSeed, ok := accountSignSecret[types.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("no signing key seed found for account %s-%s", namespace, accountName)
	}
	return nkeys.FromSeed([]byte(accountSignSecretSeed))
}
