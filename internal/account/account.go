package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/controller"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type Manager struct {
	natsClient            ports.NatsClient
	accountResolver       ports.NauthAccountResolver
	clusterConfigResolver clusterConfigResolver
	secretManager         secretManager
}

func NewManager(
	natsClient ports.NatsClient,
	accountResolver ports.NauthAccountResolver,
	natsClusterReader ports.NauthNatsClusterResolver,
	secretClient ports.SecretClient,
	configMapResolver ports.ConfigMapResolver,
	operatorClusterRef *v1alpha1.NatsClusterRef,
	operatorClusterOptional bool,
	operatorNamespace string,
	defaultNatsURL string,
) (*Manager, error) {
	ccr, err := newClusterConfigReaderImpl(natsClusterReader, secretClient, configMapResolver, operatorClusterRef, operatorClusterOptional, operatorNamespace, defaultNatsURL)
	if err != nil {
		return nil, err
	}
	sm, err := newSecretManagerImpl(secretClient)
	if err != nil {
		return nil, err
	}
	return newManager(natsClient, accountResolver, ccr, sm)
}

func newManager(
	natsClient ports.NatsClient,
	accountResolver ports.NauthAccountResolver,
	clusterConfigReader clusterConfigResolver,
	secretManager secretManager,
) (*Manager, error) {
	m := &Manager{
		natsClient:            natsClient,
		accountResolver:       accountResolver,
		clusterConfigResolver: clusterConfigReader,
		secretManager:         secretManager,
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
	if a.secretManager == nil {
		return errors.New("secret manager is required")
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
	accountSecrets, err := a.secretManager.GetSecrets(ctx, state.GetNamespace(), state.GetName(), "")
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

	err = a.secretManager.ApplyRootSecret(ctx, state.GetNamespace(), state.GetName(), accountKeyPair)
	if err != nil {
		return nil, fmt.Errorf("failed to apply account root secret during creation: %w", err)
	}

	err = a.secretManager.ApplySignSecret(ctx, state.GetNamespace(), state.GetName(), accountPublicKey, accountSigningKeyPair)
	if err != nil {
		return nil, fmt.Errorf("failed to apply account signing secret during creation: %w", err)
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
	secrets, err := a.secretManager.GetSecrets(ctx, state.GetNamespace(), state.GetName(), accountID)
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

	secrets, err := a.secretManager.GetSecrets(ctx, state.GetNamespace(), state.GetName(), accountID)
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

	err = a.secretManager.DeleteAll(ctx, state.GetNamespace(), state.GetName(), accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account secrets: %w", err)
	}

	return nil
}

func (a *Manager) resolveClusterConfig(ctx context.Context, account *v1alpha1.Account) (*clusterConfig, error) {
	natsClusterRef := account.Spec.NatsClusterRef
	if natsClusterRef != nil && natsClusterRef.Namespace == "" {
		natsClusterRef = natsClusterRef.DeepCopy()
		natsClusterRef.Namespace = account.GetNamespace()
	}

	return a.clusterConfigResolver.GetClusterConfig(ctx, natsClusterRef)
}

func getDisplayName(account *v1alpha1.Account) string {
	if account.Spec.DisplayName != "" {
		return account.Spec.DisplayName
	}
	return fmt.Sprintf("%s/%s", account.GetNamespace(), account.GetName())
}
