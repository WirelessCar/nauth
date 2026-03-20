package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: Core must not depend on adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type AccountManager struct {
	natsClient            outbound.NatsClient
	accountReader         outbound.AccountReader
	clusterTargetResolver clusterTargetResolver
	secretManager         secretManager
}

func NewAccountManager(
	natsClient outbound.NatsClient,
	accountReader outbound.AccountReader,
	natsClusterReader outbound.NatsClusterReader,
	secretClient outbound.SecretClient,
	configMapReader outbound.ConfigMapReader,
	config *Config,
) (*AccountManager, error) {
	ccr, err := newClusterTargetResolverImpl(natsClusterReader, secretClient, configMapReader, config)
	if err != nil {
		return nil, err
	}
	sm, err := newSecretManagerImpl(secretClient)
	if err != nil {
		return nil, err
	}
	return newAccountManager(natsClient, accountReader, ccr, sm)
}

func newAccountManager(
	natsClient outbound.NatsClient,
	accountReader outbound.AccountReader,
	clusterTargetResolver clusterTargetResolver,
	secretManager secretManager,
) (*AccountManager, error) {
	m := &AccountManager{
		natsClient:            natsClient,
		accountReader:         accountReader,
		clusterTargetResolver: clusterTargetResolver,
		secretManager:         secretManager,
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func (a *AccountManager) validate() error {
	if a.clusterTargetResolver == nil {
		return errors.New("clusterTargetResolver is required")
	}
	if a.accountReader == nil {
		return errors.New("accountReader is required")
	}
	if a.secretManager == nil {
		return errors.New("secretManager is required")
	}
	if a.natsClient == nil {
		return errors.New("natsClient is required")
	}

	return nil
}

func (a *AccountManager) Create(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	accountRef := domain.NewNamespacedName(state.GetNamespace(), state.GetName())
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	var accountPublicKey string
	var accountSigningPublicKey string
	var accountKeyPair nkeys.KeyPair
	var accountSigningKeyPair nkeys.KeyPair

	cluster, err := a.resolveClusterTarget(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}
	accountSecrets, err := a.secretManager.GetSecrets(ctx, accountRef, "")
	if err == nil {
		accountKeyPair = accountSecrets.Root
		accountPublicKey, err = accountKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to get account public key from existing secret during creation: %w", err)
		}

		accountSigningKeyPair = accountSecrets.Sign
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

	err = a.secretManager.ApplyRootSecret(ctx, accountRef, accountKeyPair)
	if err != nil {
		return nil, fmt.Errorf("failed to apply account root secret during creation: %w", err)
	}

	err = a.secretManager.ApplySignSecret(ctx, accountRef, accountPublicKey, accountSigningKeyPair)
	if err != nil {
		return nil, fmt.Errorf("failed to apply account signing secret during creation: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during creation: %w", err)
	}

	natsClaims, err := newAccountClaimsBuilder(ctx, getDisplayName(state), state.Spec, accountPublicKey, a.accountReader).
		signingKey(accountSigningPublicKey).
		build()
	if err != nil {
		return nil, fmt.Errorf("failed to build NATS account claims during creation: %w", err)
	}
	signedJwt, err := signAccountJWT(natsClaims, cluster.OperatorSigningKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign account jwt during creation: %w", err)
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
	return &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func signAccountJWT(claims *jwt.AccountClaims, operatorSigningKey nkeys.KeyPair) (string, error) {
	claimsVal := &jwt.ValidationResults{}
	claims.Validate(claimsVal)
	if errs := claimsVal.Errors(); len(errs) > 0 {
		return "", fmt.Errorf("account claims validation failed: %v", errs)
	}
	return claims.Encode(operatorSigningKey)
}

func (a *AccountManager) Update(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	accountRef := domain.NewNamespacedName(state.GetNamespace(), state.GetName())
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.resolveClusterTarget(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]
	secrets, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
	if err != nil {
		return nil, err
	}

	sysAccountID := cluster.SystemAdminCreds.AccountID
	if sysAccountID == accountID {
		return nil, fmt.Errorf("updating system account is not supported, consider '%s: %s'", k8s.LabelManagementPolicy, k8s.LabelManagementPolicyObserveValue)
	}

	accountKeyPair := secrets.Root
	accountPublicKey, err := accountKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account public key from existing seed during update: %w", err)
	}

	accountSigningKeyPair := secrets.Sign
	accountSigningPublicKey, err := accountSigningKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing public key from existing seed during update: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during update: %w", err)
	}

	natsClaims, err := newAccountClaimsBuilder(ctx, getDisplayName(state), state.Spec, accountPublicKey, a.accountReader).
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
	return &domain.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func (a *AccountManager) Import(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	accountRef := domain.NewNamespacedName(state.GetNamespace(), state.GetName())
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.resolveClusterTarget(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key during import: %w", err)
	}

	accountID := state.GetLabels()[k8s.LabelAccountID]
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s during import", state.GetName())
	}

	secrets, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for account %s during import: %w", accountID, err)
	}

	accountRootKeyPair := secrets.Root
	accountRootPublicKey, err := accountRootKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account public key for account %s from existing seed during import: %w", accountID, err)
	}
	if accountRootPublicKey != accountID {
		return nil, fmt.Errorf("account root seed does not match account ID during import: expected %s, got %s", accountID, accountRootPublicKey)
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
	return &domain.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
	}, nil
}

func (a *AccountManager) Delete(ctx context.Context, state *v1alpha1.Account) error {
	accountRef := domain.NewNamespacedName(state.GetNamespace(), state.GetName())
	if err := accountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.resolveClusterTarget(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to resolve cluster target: %w", err)
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

	err = a.secretManager.DeleteAll(ctx, accountRef, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account secrets: %w", err)
	}

	return nil
}

func (a *AccountManager) SignUserJWT(ctx context.Context, accountRef domain.NamespacedName, claims *jwt.UserClaims) (*SignedUserJWT, error) {
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}
	account, err := a.accountReader.Get(ctx, accountRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get account for user JWT signing: %w", err)
	}
	accountID := account.GetLabels()[k8s.LabelAccountID]
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s during user JWT signing", accountRef)
	}
	if claims.IssuerAccount != "" && claims.IssuerAccount != accountID {
		return nil, fmt.Errorf("claims issuer account ID %s does not match %s bound to account %q during user JWT signing", claims.IssuerAccount, accountID, accountRef)
	}
	if claims.IssuerAccount == "" {
		claims.IssuerAccount = accountID
	}
	claimsVal := &jwt.ValidationResults{}
	claims.Validate(claimsVal)
	if errs := claimsVal.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("claims validation failed during user JWT signing: %v", claimsVal.Errors())
	}
	accountSecrets, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account secrets for user JWT signing: %w", err)
	}
	signPubKey, err := accountSecrets.Sign.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account signing public key for user JWT signing: %w", err)
	}
	userJWT, err := claims.Encode(accountSecrets.Sign)
	if err != nil {
		return nil, fmt.Errorf("failed to sign user JWT using %s for account %s (%q): %w", signPubKey, accountID, accountRef, err)
	}
	return &SignedUserJWT{
		UserJWT:   userJWT,
		AccountID: accountID,
		SignedBy:  signPubKey,
	}, nil
}

func (a *AccountManager) resolveClusterTarget(ctx context.Context, account *v1alpha1.Account) (*clusterTarget, error) {
	natsClusterRef := account.Spec.NatsClusterRef
	if natsClusterRef != nil && natsClusterRef.Namespace == "" {
		natsClusterRef = natsClusterRef.DeepCopy()
		natsClusterRef.Namespace = account.GetNamespace()
	}

	return a.clusterTargetResolver.GetClusterTarget(ctx, natsClusterRef)
}

func getDisplayName(account *v1alpha1.Account) string {
	if account.Spec.DisplayName != "" {
		return account.Spec.DisplayName
	}
	return fmt.Sprintf("%s/%s", account.GetNamespace(), account.GetName())
}

var _ inbound.AccountManager = (*AccountManager)(nil)
var _ UserJWTSigner = (*AccountManager)(nil)
