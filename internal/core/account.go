package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type AccountManager struct {
	natsSysClient         outbound.NatsSysClient
	natsAccClient         outbound.NatsAccountClient
	accountIDReader       outbound.AccountIDReader
	clusterTargetResolver clusterTargetResolver
	secretManager         secretManager
}

func NewAccountManager(
	natsSysClient outbound.NatsSysClient,
	natsAccClient outbound.NatsAccountClient,
	accountIDReader outbound.AccountIDReader,
	secretClient outbound.SecretClient,
	clusterManager *ClusterManager,
) (*AccountManager, error) {
	sm, err := newSecretManagerImpl(secretClient)
	if err != nil {
		return nil, err
	}
	return newAccountManager(natsSysClient, natsAccClient, accountIDReader, clusterManager, sm)
}

func newAccountManager(
	natsSysClient outbound.NatsSysClient,
	natsAccClient outbound.NatsAccountClient,
	accountIDReader outbound.AccountIDReader,
	clusterTargetResolver clusterTargetResolver,
	secretManager secretManager,
) (*AccountManager, error) {
	m := &AccountManager{
		natsSysClient:         natsSysClient,
		natsAccClient:         natsAccClient,
		accountIDReader:       accountIDReader,
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
	if a.accountIDReader == nil {
		return errors.New("accountIDReader is required")
	}
	if a.secretManager == nil {
		return errors.New("secretManager is required")
	}
	if a.natsSysClient == nil {
		return errors.New("natsSysClient is required")
	}
	if a.natsAccClient == nil {
		return errors.New("natsAccClient is required")
	}

	return nil
}

func (a *AccountManager) CreateOrUpdate(ctx context.Context, request nauth.AccountRequest) (*nauth.AccountResult, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account request: %w", err)
	}

	cluster, err := a.clusterTargetResolver.GetClusterTarget(ctx, request.ClusterRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	fixedAccountID := string(request.AccountID)
	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, request.AccountRef, fixedAccountID)
	if fixedAccountID != "" {
		// Update
		if !found {
			return nil, fmt.Errorf("account secrets not found for account %s", fixedAccountID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get account secrets for account %s: %w", fixedAccountID, err)
		}
		if fixedAccountID == cluster.SystemAdminCreds.AccountID {
			return nil, fmt.Errorf("reconciling system account is not supported")
		}
	} else if found && err != nil {
		// Create
		return nil, fmt.Errorf("existing account secrets are invalid; account creation requires manual intervention: %w", err)
	}

	var accountKeyPair nkeys.KeyPair
	var accountPublicKey string
	var accountSigningKeyPair nkeys.KeyPair
	if found {
		accountKeyPair = accountSecrets.Root
		accountPublicKey, err = accountKeyPair.PublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to extract account root public key from existing secret: %w", err)
		}
		accountSigningKeyPair = accountSecrets.Sign
	} else {
		accountKeyPair, err = nkeys.CreateAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to create account root key pair: %w", err)
		}
		accountPublicKey, _ = accountKeyPair.PublicKey() // Safe due to new nkey

		accountSigningKeyPair, err = nkeys.CreateAccount()
		if err != nil {
			return nil, fmt.Errorf("failed to create account signing key pair: %w", err)
		}

		err = a.secretManager.ApplyRootSecret(ctx, request.AccountRef, accountKeyPair)
		if err != nil {
			return nil, fmt.Errorf("failed to apply account root secret: %w", err)
		}

		err = a.secretManager.ApplySignSecret(ctx, request.AccountRef, accountPublicKey, accountSigningKeyPair)
		if err != nil {
			return nil, fmt.Errorf("failed to apply account signing secret: %w", err)
		}
	}

	accountSigningPublicKey, err := accountSigningKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to extract account signing public key: %w", err)
	}

	operatorSigningPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator signing public key: %w", err)
	}

	claimsBuilder := newAccountClaimsBuilder(accountPublicKey, request.JetStreamEnabled).
		displayName(getDisplayName(request)).
		signingKey(accountSigningPublicKey).
		accountLimits(request.AccountLimits).
		jetStreamLimits(request.JetStreamLimits).
		natsLimits(request.NatsLimits)

	adoptions := nauth.NewAccountAdoptions()
	if err = adoptExportGroups(request.ExportGroups, claimsBuilder, adoptions); err != nil {
		return nil, fmt.Errorf("failed to adopt export groups: %w", err)
	}
	if err = adoptImportGroups(request.ImportGroups, claimsBuilder, adoptions); err != nil {
		return nil, fmt.Errorf("failed to adopt import groups: %w", err)
	}

	natsClaims, err := claimsBuilder.build()
	if err != nil {
		return nil, fmt.Errorf("failed to build NATS account claims: %w", err)
	}

	signedJwt, err := signAccountJWT(natsClaims, cluster.OperatorSigningKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign account jwt: %w", err)
	}

	claimsHash, err := hashSignedAccountJWTClaims(signedJwt)
	if err != nil {
		return nil, fmt.Errorf("failed to hash account claims: %w", err)
	}

	log := logf.FromContext(ctx)
	prevClaimsHash := request.ClaimsHash
	if prevClaimsHash == "" || prevClaimsHash != claimsHash {
		sysConn, err := a.natsSysClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
		}
		defer sysConn.Disconnect()

		err = sysConn.UploadAccountJWT(signedJwt)
		if err != nil {
			return nil, fmt.Errorf("failed to upload account jwt: %w", err)
		}
		log.Info("Uploaded Account JWT to NATS",
			"accountID", accountPublicKey, "prevClaimsHash", prevClaimsHash, "claimsHash", claimsHash)
	}

	nauthClaims, err := convertNatsAccountClaims(natsClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to convert NATS account claims: %w", err)
	}
	return &nauth.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
		ClaimsHash:      claimsHash,
		Adoptions:       adoptions,
	}, nil
}

func adoptExportGroups(groups nauth.ExportGroups, claimsBuilder *accountClaimsBuilder, adoptions *nauth.AccountAdoptions) error {
	for _, exp := range groups {
		adoptionResult := nauth.AdoptionResult{Ref: exp.Ref}
		err := claimsBuilder.addExportGroup(*exp)
		if err != nil {
			if exp.Required {
				return fmt.Errorf("failed to include required export group %q: %w", exp.Ref, err)
			}
			adoptionResult.Failure = nauth.AdoptionFailureConflict
			adoptionResult.Message = err.Error()
		}
		if err = adoptions.Exports.Add(adoptionResult); err != nil {
			return fmt.Errorf("failed to add adoption result for export group %q: %w", exp.Ref, err)
		}
	}
	return nil
}

func adoptImportGroups(groups nauth.ImportGroups, claimsBuilder *accountClaimsBuilder, adoptions *nauth.AccountAdoptions) error {
	for _, imp := range groups {
		adoptionResult := nauth.AdoptionResult{Ref: imp.Ref}
		err := claimsBuilder.addImportGroup(*imp)
		if err != nil {
			if imp.Required {
				return fmt.Errorf("failed to include required import group %q: %w", imp.Ref, err)
			}
			adoptionResult.Failure = nauth.AdoptionFailureConflict
			adoptionResult.Message = err.Error()
		}
		if err = adoptions.Imports.Add(adoptionResult); err != nil {
			return fmt.Errorf("failed to add adoption result for import group %q: %w", imp.Ref, err)
		}
	}
	return nil
}

func signAccountJWT(claims *jwt.AccountClaims, operatorSigningKey nkeys.KeyPair) (string, error) {
	claimsVal := &jwt.ValidationResults{}
	claims.Validate(claimsVal)
	if errs := claimsVal.Errors(); len(errs) > 0 {
		return "", fmt.Errorf("account claims validation failed: %v", errs)
	}
	return claims.Encode(operatorSigningKey)
}

func (a *AccountManager) Import(ctx context.Context, reference nauth.AccountReference) (*nauth.AccountResult, error) {
	accountRef := reference.AccountRef
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.clusterTargetResolver.GetClusterTarget(ctx, reference.NatsClusterRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	accountID := string(reference.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s during import", accountRef)
	}

	secrets, found, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets for account %s during import: %w", accountID, err)
	}
	if !found {
		return nil, fmt.Errorf("account secrets not found for account %s during import", accountID)
	}

	accountRootKeyPair := secrets.Root
	accountRootPublicKey, err := accountRootKeyPair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get account public key for account %s from existing seed during import: %w", accountID, err)
	}
	if accountRootPublicKey != accountID {
		return nil, fmt.Errorf("account root seed does not match account ID during import: expected %s, got %s", accountID, accountRootPublicKey)
	}

	sysConn, err := a.natsSysClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster during import: %w", err)
	}
	defer sysConn.Disconnect()
	accountJWT, err := sysConn.LookupAccountJWT(accountID)
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

	nauthClaims, err := convertNatsAccountClaims(natsClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to convert NATS account claims for account %s during import: %w", accountID, err)
	}
	claimsHash, err := hashSignedAccountJWTClaims(accountJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to hash account claims during import: %w", err)
	}
	return &nauth.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: natsClaims.Issuer,
		Claims:          &nauthClaims,
		ClaimsHash:      claimsHash,
	}, nil
}

func (a *AccountManager) Delete(ctx context.Context, reference nauth.AccountReference) error {
	if err := reference.AccountRef.Validate(); err != nil {
		return fmt.Errorf("invalid account reference: %w", err)
	}

	cluster, err := a.clusterTargetResolver.GetClusterTarget(ctx, reference.NatsClusterRef)
	if err != nil {
		return fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	operatorPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get operator signing public key: %w", err)
	}

	accountID := string(reference.AccountID)
	if accountID == "" {
		return fmt.Errorf("account ID is missing for account %s", reference.AccountRef)
	}

	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, reference.AccountRef, accountID)
	if err != nil {
		return fmt.Errorf("failed to get secrets for account: %w", err)
	}
	if found {
		// Account secrets may already be gone if secretManager.DeleteAll partially failed during previous attempt.
		// Then we won't be able to sign a JWT to lookup account streams, but we can skip the check since the account
		// is effectively already deleted in NATS.
		streams, err := a.listAccountStreams(cluster, accountSecrets, accountID)
		if err != nil {
			return fmt.Errorf("failed to list account streams: %w", err)
		}
		if len(streams) > 0 {
			return fmt.Errorf("account deletion aborted due to %d JetStream Stream(s) still exist for account: %s", len(streams), streams)
		}
	}

	// Delete is done by signing a jwt with a list of accounts to be deleted
	deleteClaim := jwt.NewGenericClaims(operatorPublicKey)
	deleteClaim.Data["accounts"] = []string{accountID}

	deleteJwt, err := deleteClaim.Encode(cluster.OperatorSigningKey)
	if err != nil {
		return fmt.Errorf("failed to sign account JWT: %w", err)
	}

	sysConn, err := a.natsSysClient.Connect(cluster.NatsURL, cluster.SystemAdminCreds)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer sysConn.Disconnect()

	err = sysConn.DeleteAccountJWT(deleteJwt)
	if err != nil {
		return fmt.Errorf("failed to delete account JWT in NATS: %w", err)
	}

	err = a.secretManager.DeleteAll(ctx, reference.AccountRef, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account secrets: %w", err)
	}

	return nil
}

func (a *AccountManager) listAccountStreams(cluster *nauth.ClusterTarget, accountSecrets *Secrets, accountID string) ([]string, error) {
	tempUserCreds, err := createTempJetStreamCreds(accountID, accountSecrets.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary account JetStream credentials: %w", err)
	}

	accConn, err := a.natsAccClient.Connect(cluster.NatsURL, *tempUserCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster for JetStream streams lookup: %w", err)
	}
	defer accConn.Disconnect()

	streamNames, err := accConn.ListAccountStreams()
	if err != nil {
		return nil, fmt.Errorf("failed to lookup account JetStream streams: %w", err)
	}
	return streamNames, nil
}

func createTempJetStreamCreds(accountID string, accountKeyPair nkeys.KeyPair) (*domain.NatsUserCreds, error) {
	userKeyPair, _ := nkeys.CreateUser()
	userPublicKey, _ := userKeyPair.PublicKey()
	userSeed, _ := userKeyPair.Seed()

	claims := jwt.NewUserClaims(userPublicKey)
	claims.IssuerAccount = accountID
	claims.Expires = time.Now().Add(30 * time.Second).Unix()
	claims.Permissions = jwt.Permissions{
		Pub: jwt.Permission{
			Allow: jwt.StringList{"$JS.API.>"},
		},
		Sub: jwt.Permission{
			Allow: jwt.StringList{"_INBOX.>"},
		},
	}

	userJWT, err := claims.Encode(accountKeyPair)
	if err != nil {
		return nil, fmt.Errorf("sign temporary user JWT: %w", err)
	}
	userCreds, err := jwt.FormatUserConfig(userJWT, userSeed)
	if err != nil {
		return nil, fmt.Errorf("format temporary user credentials: %w", err)
	}

	natsUserCreds, err := domain.NewNatsUserCreds(userCreds)
	if err != nil {
		return nil, fmt.Errorf("build user credentials: %w", err)
	}
	return natsUserCreds, nil
}

func (a *AccountManager) SignUserJWT(ctx context.Context, accountRef domain.NamespacedName, claims *jwt.UserClaims) (*SignedUserJWT, error) {
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}
	accountID, err := a.accountIDReader.GetAccountID(ctx, accountRef)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup Account ID for %q during user JWT signing: %w", accountRef, err)
	}
	if claims.IssuerAccount != "" && claims.IssuerAccount != string(accountID) {
		return nil, fmt.Errorf("claims issuer account ID %s does not match %s bound to account %q during user JWT signing", claims.IssuerAccount, accountID, accountRef)
	}
	if claims.IssuerAccount == "" {
		claims.IssuerAccount = string(accountID)
	}
	claimsVal := &jwt.ValidationResults{}
	claims.Validate(claimsVal)
	if errs := claimsVal.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("claims validation failed during user JWT signing: %v", claimsVal.Errors())
	}
	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, accountRef, string(accountID))
	if err != nil {
		return nil, fmt.Errorf("failed to get account secrets for user JWT signing: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("account secrets not found for user JWT signing")
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
		AccountID: string(accountID),
		SignedBy:  signPubKey,
	}, nil
}

func getDisplayName(request nauth.AccountRequest) string {
	if request.DisplayName != "" {
		return request.DisplayName
	}
	return request.AccountRef.String()
}

var _ inbound.AccountManager = (*AccountManager)(nil)
var _ UserJWTSigner = (*AccountManager)(nil)
