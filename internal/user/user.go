package user

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/WirelessCar/nauth/internal/nats"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecretClient interface {
	Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string, update bool) error
	Get(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
	Delete(ctx context.Context, namespace string, name string) error
	Label(ctx context.Context, namespace, name string, labels map[string]string) error
}

type AccountGetter interface {
	Get(ctx context.Context, accountRefName string, namespace string) (account *v1alpha1.Account, err error)
}

type Manager struct {
	accounts       AccountGetter
	natsClient     nats.NatsClient
	secretClient   SecretClient
	nauthNamespace string
}

func NewManager(accounts AccountGetter, natsClient nats.NatsClient, secretClient SecretClient, opts ...func(*Manager)) *Manager {
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
		manager.nauthNamespace = strings.TrimSpace(string(controllerNamespace))
	}
	return manager
}

func WithNamespace(namespace string) func(*Manager) {
	return func(manager *Manager) {
		manager.nauthNamespace = namespace
	}
}

func (u *Manager) getOperatorSigningKeyPair(ctx context.Context) (nkeys.KeyPair, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeOperatorSign,
	}
	operatorSecret, err := u.secretClient.GetByLabels(ctx, u.nauthNamespace, labels)
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

// ensureSigningKeySecret returns a KeyPair from a Kubernetes secret, which is created if needed
func (u *Manager) ensureSigningKeySecret(ctx context.Context, namespace string, state *v1alpha1.User) (nkeys.KeyPair, error) {
	signingKey, err := u.getUserSigningKeyPair(ctx, namespace, state.Name)
	if err == nil {
		return signingKey, nil
	}
	// Create or update the signing key secret
	signingKey, _ = nkeys.CreateAccount()
	accountSeed, _ := signingKey.Seed()
	signingKeyPublicKey, _ := signingKey.PublicKey()
	// signingPrivateKey, _ := sk.PrivateKey()
	secretOwner := &secret.Owner{
		Owner: state,
	}
	secretMeta := metav1.ObjectMeta{
		Name:      state.GetUserSigningKeySecretName(),
		Namespace: state.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelUserSigningKeyID: signingKeyPublicKey,
			k8s.LabelSecretType:       k8s.SecretTypeUserSign,
			k8s.LabelManaged:          k8s.LabelManagedValue,
			k8s.LabelUserName:         state.Name,
		},
	}
	secretValue := map[string]string{
		k8s.DefaultSecretKeyName: string(accountSeed),
	}
	err = u.secretClient.Apply(ctx, secretOwner, secretMeta, secretValue, true)
	if err != nil {
		return nil, err
	}
	return signingKey, nil
}

// deleteScoppingSigningKey removes a signing key from the account JWT
func (u *Manager) deleteScoppingSigningKey(ctx context.Context, namespace string, accountID string, state *v1alpha1.User) error {
	signingKeyPair, err := u.ensureSigningKeySecret(ctx, namespace, state)
	if err != nil {
		return fmt.Errorf("cannot get signing key secret: %w", err)
	}
	signingKeyPublicKey, _ := signingKeyPair.PublicKey()

	// The secret is created, update the account JWT
	err = u.natsClient.EnsureConnected(u.nauthNamespace)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer u.natsClient.Disconnect()
	accountJWT, err := u.natsClient.LookupAccountJWT(accountID)
	if err != nil {
		return fmt.Errorf("failed to lookup account jwt for account %s: %w", accountID, err)
	}
	if len(accountJWT) == 0 {
		return fmt.Errorf("account jwt for account %s not found", accountID)
	}
	accountNatsClaims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return fmt.Errorf("failed to decode account jwt for account %s: %w", accountID, err)
	}

	// Add the new signing key to the list
	if accountNatsClaims.SigningKeys == nil {
		accountNatsClaims.SigningKeys = jwt.SigningKeys{}
	}
	accountNatsClaims.SigningKeys.Add(signingKeyPublicKey)

	delete(accountNatsClaims.SigningKeys, signingKeyPublicKey)

	// Get operator signing key
	operatorSigningKeyPair, err := u.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	// Sign account JWT
	newAccountJWT, err := accountNatsClaims.Encode(operatorSigningKeyPair)
	if err != nil {
		userResource := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return fmt.Errorf("failed to sign account jwt for %s: %w", userResource, err)
	}

	// Upload account JWT
	if err := u.natsClient.UploadAccountJWT(newAccountJWT); err != nil {
		return fmt.Errorf("failed to upload account jwt: %w", err)
	}
	return nil
}

// ensureScoppingSigningKey creates a scopping signing key, making sure the Kubernetes secret is created, and the account JWT updated
func (u *Manager) ensureScoppingSigningKey(ctx context.Context, namespace string, accountID string, userPublicKey string, state *v1alpha1.User) (nkeys.KeyPair, error) {
	signingKeyPair, err := u.ensureSigningKeySecret(ctx, namespace, state)
	if err != nil {
		return nil, fmt.Errorf("cannot get signing key secret: %w", err)
	}
	signingKeyPublicKey, _ := signingKeyPair.PublicKey()

	// The secret is created, update the account JWT
	err = u.natsClient.EnsureConnected(u.nauthNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}
	defer u.natsClient.Disconnect()
	accountJWT, err := u.natsClient.LookupAccountJWT(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup account jwt for account %s: %w", accountID, err)
	}
	if len(accountJWT) == 0 {
		return nil, fmt.Errorf("account jwt for account %s not found", accountID)
	}
	accountNatsClaims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to decode account jwt for account %s: %w", accountID, err)
	}

	// Add the new signing key to the list
	if accountNatsClaims.SigningKeys == nil {
		accountNatsClaims.SigningKeys = jwt.SigningKeys{}
	}
	accountNatsClaims.SigningKeys.Add(signingKeyPublicKey)

	delete(accountNatsClaims.SigningKeys, signingKeyPublicKey)

	natsClaims := newClaimsBuilder(getDisplayName(state), state.Spec, userPublicKey, accountID).build()
	scope := jwt.NewUserScope()
	scope.Key = signingKeyPublicKey
	scope.Role = state.Name
	scope.Template.Pub = natsClaims.Pub
	scope.Template.Sub = natsClaims.Sub
	scope.Template.Permissions = natsClaims.Permissions
	scope.Template.Limits = natsClaims.Limits
	accountNatsClaims.SigningKeys.AddScopedSigner(scope)

	// Get operator signing key
	operatorSigningKeyPair, err := u.getOperatorSigningKeyPair(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing key pair from seed: %w", err)
	}

	// Sign account JWT
	newAccountJWT, err := accountNatsClaims.Encode(operatorSigningKeyPair)
	if err != nil {
		userResource := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return nil, fmt.Errorf("failed to sign account jwt for %s: %w", userResource, err)
	}

	// Upload account JWT
	if err := u.natsClient.UploadAccountJWT(newAccountJWT); err != nil {
		return nil, fmt.Errorf("failed to upload account jwt: %w", err)
	}
	return signingKeyPair, nil
}

/*
if useSigningKey = false

	find user secret, create if needed, update if needed

if useSigningKey = true

	find user scoped key, create if needed, update if needed
		if created of updated, update the account JWT
	find user creds, create if needed (update not needed)
*/
func (u *Manager) CreateOrUpdate(ctx context.Context, state *v1alpha1.User) error {
	acc, err := u.accounts.Get(ctx, state.Spec.AccountName, state.Namespace)
	if err != nil {
		return errors.Join(k8s.NoErrRetryLater, err)
	}

	var signingKeyPair nkeys.KeyPair
	var natsClaims *jwt.UserClaims
	var signingKeyPublicKey string

	accountID := acc.GetLabels()[k8s.LabelAccountID]
	accountSigningKeyPair, err := u.getAccountSigningKeyPair(ctx, acc.GetNamespace(), acc.GetName(), accountID)
	if err != nil {
		return fmt.Errorf("failed to get signing key secret %s/%s: %w", acc.GetNamespace(), acc.GetName(), err)
	}

	userKeyPair, _ := nkeys.CreateUser()
	userPublicKey, _ := userKeyPair.PublicKey()
	userSeed, _ := userKeyPair.Seed()

	// Get the user's signing key, create if needed
	if state.Spec.UseSigningKey {
		signingKeyPair, err = u.ensureScoppingSigningKey(ctx, acc.GetNamespace(), accountID, userPublicKey, state)
		if err != nil {
			return fmt.Errorf("failed to update the account JWT: %w", err)
		}
		signingKeyPublicKey, _ = signingKeyPair.PublicKey()
		// Scopped users claims must be empty
		natsClaims = newClaimsBuilder(getDisplayName(state), v1alpha1.UserSpec{
			AccountName: state.Spec.AccountName,
			DisplayName: state.Spec.DisplayName,
			Permissions: nil,
			UserLimits:  nil,
			NatsLimits:  nil,
		}, userPublicKey, accountID).build()
	} else {
		// Sign using the account key
		signingKeyPublicKey, _ = accountSigningKeyPair.PublicKey()
		signingKeyPair = accountSigningKeyPair
		natsClaims = newClaimsBuilder(getDisplayName(state), state.Spec, userPublicKey, accountID).
			build()
	}

	userJwt, err := natsClaims.Encode(signingKeyPair)
	if err != nil {
		userResource := fmt.Sprintf("%s-%s", state.GetNamespace(), state.GetName())
		return fmt.Errorf("failed to sign user jwt for %s: %w", userResource, err)
	}

	userCreds, _ := jwt.FormatUserConfig(userJwt, userSeed)

	secretOwner := &secret.Owner{
		Owner: state,
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
	err = u.secretClient.Apply(ctx, secretOwner, secretMeta, secretValue, !state.Spec.UseSigningKey)
	if err != nil {
		return err
	}

	toNAuthUserClaims(natsClaims)
	state.Status.Claims = toNAuthUserClaims(natsClaims)

	if state.Labels == nil {
		state.Labels = make(map[string]string, 3)
	}

	state.GetLabels()[k8s.LabelUserID] = userPublicKey
	state.GetLabels()[k8s.LabelUserAccountID] = acc.GetLabels()[k8s.LabelAccountID]
	state.GetLabels()[k8s.LabelUserSignedBy] = signingKeyPublicKey

	state.Status.ObservedGeneration = state.Generation
	state.Status.ReconcileTimestamp = metav1.Now()

	return nil
}

/*
if useSigningKey = false

	Delete user secret

if useSigningKey = true

	Update Account JWT
	Delete signing key secret
	Delete secret
*/
func (u *Manager) Delete(ctx context.Context, state *v1alpha1.User) error {
	log := logf.FromContext(ctx)
	log.Info("Delete user", "userName", state.GetName())

	if state.Spec.UseSigningKey {
		acc, err := u.accounts.Get(ctx, state.Spec.AccountName, state.Namespace)
		if err != nil {
			return errors.Join(k8s.NoErrRetryLater, err)
		}

		accountID := acc.GetLabels()[k8s.LabelAccountID]
		if err := u.deleteScoppingSigningKey(ctx, state.Namespace, accountID, state); err != nil {
			return fmt.Errorf("failed to update the account JWT: %w", err)
		}

		if err := u.secretClient.Delete(ctx, state.Namespace, state.GetUserSigningKeySecretName()); err != nil {
			return fmt.Errorf("failed to delete user signing key secret %s/%s: %w", state.Namespace, state.GetUserSigningKeySecretName(), err)
		}
	}
	if err := u.secretClient.Delete(ctx, state.Namespace, state.GetUserSecretName()); err != nil {
		return fmt.Errorf("failed to delete user secret %s/%s: %w", state.Namespace, state.GetUserSecretName(), err)
	}

	return nil
}

func (u *Manager) getUserSigningKeyPair(ctx context.Context, namespace, userName string) (nkeys.KeyPair, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeUserSign,
		k8s.LabelManaged:    k8s.LabelManagedValue,
		k8s.LabelUserName:   userName,
	}
	secrets, err := u.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing secret for user: %s-%s due to %w", namespace, userName, err)
	}

	if len(secrets.Items) < 1 {
		return nil, fmt.Errorf("no signing secret found for user: %s-%s", namespace, userName)
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("more than 1 signing secret found for user: %s-%s", namespace, userName)
	}

	seed, ok := secrets.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("secret for user credentials seed was malformed")
	}
	return nkeys.FromSeed(seed)
}

func (u *Manager) getAccountSigningKeyPair(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
	if keyPair, err := u.getAccountSigningKeyPairByAccountID(ctx, namespace, accountName, accountID); err == nil {
		return keyPair, nil
	}

	keyPair, err := u.getDeprecatedAccountSigningKeyPair(ctx, namespace, accountName, accountID)
	if err != nil {
		return nil, err
	}

	return keyPair, nil
}

func (u *Manager) getAccountSigningKeyPairByAccountID(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
	labels := map[string]string{
		k8s.LabelAccountID:  accountID,
		k8s.LabelSecretType: k8s.SecretTypeAccountSign,
		k8s.LabelManaged:    k8s.LabelManagedValue,
	}

	secrets, err := u.secretClient.GetByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing secret for account: %s-%s due to %w", namespace, accountName, err)
	}

	if len(secrets.Items) < 1 {
		return nil, fmt.Errorf("no signing secret found for account: %s-%s", namespace, accountName)
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("more than 1 signing secret found for account: %s-%s", namespace, accountName)
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
func (u *Manager) getDeprecatedAccountSigningKeyPair(ctx context.Context, namespace, accountName, accountID string) (nkeys.KeyPair, error) {
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

			accountSecret, err := u.secretClient.Get(ctx, namespace, secretName)
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
			if err := u.secretClient.Label(ctx, namespace, secretName, labels); err != nil {
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

	accountSignSecret, ok := secrets[k8s.SecretTypeAccountSign]
	if !ok {
		return nil, fmt.Errorf("no signing key found for account %s-%s", namespace, accountName)
	}

	accountSignSecretSeed, ok := accountSignSecret[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("no signing key seed found for account %s-%s", namespace, accountName)
	}
	return nkeys.FromSeed([]byte(accountSignSecretSeed))
}
