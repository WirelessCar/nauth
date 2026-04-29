package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const GroupNameInline = "inline"

type AccountManager struct {
	natsSysClient         outbound.NatsSysClient
	natsAccClient         outbound.NatsAccountClient
	accountReader         outbound.AccountReader
	clusterTargetResolver clusterTargetResolver
	secretManager         secretManager
}

func NewAccountManager(
	natsSysClient outbound.NatsSysClient,
	natsAccClient outbound.NatsAccountClient,
	accountReader outbound.AccountReader,
	secretClient outbound.SecretClient,
	clusterManager *ClusterManager,
) (*AccountManager, error) {
	sm, err := newSecretManagerImpl(secretClient)
	if err != nil {
		return nil, err
	}
	return newAccountManager(natsSysClient, natsAccClient, accountReader, clusterManager, sm)
}

func newAccountManager(
	natsSysClient outbound.NatsSysClient,
	natsAccClient outbound.NatsAccountClient,
	accountReader outbound.AccountReader,
	clusterTargetResolver clusterTargetResolver,
	secretManager secretManager,
) (*AccountManager, error) {
	m := &AccountManager{
		natsSysClient:         natsSysClient,
		natsAccClient:         natsAccClient,
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
	if a.natsSysClient == nil {
		return errors.New("natsSysClient is required")
	}
	if a.natsAccClient == nil {
		return errors.New("natsAccClient is required")
	}

	return nil
}

func (a *AccountManager) CreateOrUpdate(ctx context.Context, resources domain.AccountResources) (*domain.AccountResult, error) {
	account := &resources.Account
	accountRef := domain.NewNamespacedName(account.GetNamespace(), account.GetName())
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.resolveClusterTarget(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	fixedAccountID := account.GetLabel(v1alpha1.AccountLabelAccountID)
	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, accountRef, fixedAccountID)
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

		err = a.secretManager.ApplyRootSecret(ctx, accountRef, accountKeyPair)
		if err != nil {
			return nil, fmt.Errorf("failed to apply account root secret: %w", err)
		}

		err = a.secretManager.ApplySignSecret(ctx, accountRef, accountPublicKey, accountSigningKeyPair)
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

	accountIDReader := cachedAccountIDReader(ctx, a.accountReader)
	claimsBuilder := newAccountClaimsBuilder(accountPublicKey, account.Spec.JetStreamEnabled).
		displayName(getDisplayName(account)).
		signingKey(accountSigningPublicKey)
	if err = applySpec(accountIDReader, claimsBuilder, account.Spec); err != nil {
		return nil, fmt.Errorf("failed to prepare account claims: %w", err)
	}
	adoptions := nauth.NewAccountAdoptions()
	for _, exp := range resources.ExportGroups {
		adoptionResult := nauth.AdoptionResult{Ref: exp.Ref}
		err = claimsBuilder.addExportGroup(*exp)
		if err != nil {
			adoptionResult.Failure = nauth.AdoptionFailureConflict
			adoptionResult.Message = err.Error()
		}
		if err = adoptions.Exports.Add(adoptionResult); err != nil {
			return nil, fmt.Errorf("failed to add adoption result for export group %q: %w", exp.Ref, err)
		}
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
	prevClaimsHash := account.Status.ClaimsHash
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

	nauthClaims := convertNatsAccountClaims(natsClaims)
	return &domain.AccountResult{
		AccountID:       accountPublicKey,
		AccountSignedBy: operatorSigningPublicKey,
		Claims:          &nauthClaims,
		ClaimsHash:      claimsHash,
		Adoptions:       adoptions,
	}, nil
}

func applySpec(accountIDReader resolveAccountIDFn, builder *accountClaimsBuilder, spec v1alpha1.AccountSpec) error {
	builder.
		accountLimits(toNAuthAccountLimits(spec.AccountLimits)).
		jetStreamLimits(toNAuthJetStreamLimits(spec.JetStreamLimits)).
		natsLimits(toNAuthNatsLimits(spec.NatsLimits))

	inlineExportGroup := toInlineExportGroup(spec.Exports)
	if err := builder.addExportGroup(inlineExportGroup); err != nil {
		return fmt.Errorf("failed to add inline exports: %w", err)
	}

	inlineImportGroup, inlineImportsErr := toInlineImportGroup(accountIDReader, spec.Imports)
	if inlineImportsErr != nil {
		return fmt.Errorf("failed to convert inline imports to domain model: %w", inlineImportsErr)
	}
	if err := builder.addImportGroup(inlineImportGroup); err != nil {
		return fmt.Errorf("failed to add inline imports: %w", err)
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

func (a *AccountManager) Import(ctx context.Context, state *v1alpha1.Account) (*domain.AccountResult, error) {
	accountRef := domain.NewNamespacedName(state.GetNamespace(), state.GetName())
	if err := accountRef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account reference %q: %w", accountRef, err)
	}

	cluster, err := a.resolveClusterTarget(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cluster target: %w", err)
	}

	accountID := state.GetLabel(v1alpha1.AccountLabelAccountID)
	if accountID == "" {
		return nil, fmt.Errorf("account ID is missing for account %s during import", state.GetName())
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

	nauthClaims := convertNatsAccountClaims(natsClaims)
	claimsHash, err := hashSignedAccountJWTClaims(accountJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to hash account claims during import: %w", err)
	}
	return &domain.AccountResult{
		AccountID:       accountID,
		AccountSignedBy: natsClaims.Issuer,
		Claims:          &nauthClaims,
		ClaimsHash:      claimsHash,
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

	operatorPublicKey, err := cluster.OperatorSigningKey.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to get operator signing public key: %w", err)
	}

	accountID := state.GetLabel(v1alpha1.AccountLabelAccountID)
	if accountID == "" {
		return fmt.Errorf("account ID is missing for account %s", accountRef)
	}

	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
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

	err = a.secretManager.DeleteAll(ctx, accountRef, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account secrets: %w", err)
	}

	return nil
}

func (a *AccountManager) listAccountStreams(cluster *clusterTarget, accountSecrets *Secrets, accountID string) ([]string, error) {
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
	account, err := a.accountReader.Get(ctx, accountRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get account for user JWT signing: %w", err)
	}
	accountID := account.GetLabel(v1alpha1.AccountLabelAccountID)
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
	accountSecrets, found, err := a.secretManager.GetSecrets(ctx, accountRef, accountID)
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

type resolveAccountIDFn func(accountRef domain.NamespacedName) (accountID string, err error)

func cachedAccountIDReader(ctx context.Context, accountReader outbound.AccountReader) resolveAccountIDFn {
	cache := make(map[domain.NamespacedName]string)
	return func(accountRef domain.NamespacedName) (string, error) {
		accountID := ""
		var cached bool
		if accountID, cached = cache[accountRef]; !cached {
			account, err := accountReader.Get(ctx, accountRef)
			if err != nil {
				return "", fmt.Errorf("failed to resolve account ID: %w", err)
			}
			accountID = account.GetLabel(v1alpha1.AccountLabelAccountID)
			cache[accountRef] = accountID
		}
		if accountID == "" {
			return "", fmt.Errorf("account ID label %s is missing for account %q", v1alpha1.AccountLabelAccountID, accountRef)
		}
		return accountID, nil
	}
}

func toNAuthAccountLimits(source *v1alpha1.AccountLimits) *nauth.AccountLimits {
	if source == nil {
		return nil
	}
	return &nauth.AccountLimits{
		Imports:         source.Imports,
		Exports:         source.Exports,
		WildcardExports: source.WildcardExports,
		Conn:            source.Conn,
		LeafNodeConn:    source.LeafNodeConn,
	}
}

func toNAuthJetStreamLimits(source *v1alpha1.JetStreamLimits) *nauth.JetStreamLimits {
	if source == nil {
		return nil
	}
	return &nauth.JetStreamLimits{
		MemoryStorage:        source.MemoryStorage,
		DiskStorage:          source.DiskStorage,
		Streams:              source.Streams,
		Consumer:             source.Consumer,
		MaxAckPending:        source.MaxAckPending,
		MemoryMaxStreamBytes: source.MemoryMaxStreamBytes,
		DiskMaxStreamBytes:   source.DiskMaxStreamBytes,
		MaxBytesRequired:     source.MaxBytesRequired,
	}
}

func toNAuthNatsLimits(source *v1alpha1.NatsLimits) *nauth.NatsLimits {
	if source == nil {
		return nil
	}
	return &nauth.NatsLimits{
		Subs:    source.Subs,
		Data:    source.Data,
		Payload: source.Payload,
	}
}

func toInlineImportGroup(reader resolveAccountIDFn, sources v1alpha1.Imports) (nauth.ImportGroup, error) {
	result := nauth.ImportGroup{
		Name: GroupNameInline,
	}
	for _, source := range sources {
		accountRef := domain.NewNamespacedName(source.AccountRef.Namespace, source.AccountRef.Name)
		var err error
		accountID, err := reader(accountRef)
		if err != nil {
			return result, fmt.Errorf("failed to resolve account ID for inline import %s: %w", accountRef, err)
		}
		if accountID == "" {
			return result, fmt.Errorf("account ID is missing for inline import %s", accountRef)
		}
		result.Imports = append(result.Imports, &nauth.Import{
			AccountID:    nauth.AccountID(accountID),
			Name:         source.Name,
			Subject:      nauth.Subject(source.Subject),
			LocalSubject: nauth.Subject(source.LocalSubject),
			Type:         toNAuthExportTypeFromAPI(source.Type),
			Share:        source.Share,
			AllowTrace:   source.AllowTrace,
		})
	}
	return result, nil
}

func toInlineExportGroup(sources v1alpha1.Exports) nauth.ExportGroup {
	result := nauth.ExportGroup{
		Name: GroupNameInline,
	}
	for _, source := range sources {
		result.Exports = append(result.Exports, &nauth.Export{
			Name:                 source.Name,
			Subject:              nauth.Subject(source.Subject),
			Type:                 toNAuthExportTypeFromAPI(source.Type),
			TokenReq:             source.TokenReq,
			Revocations:          nauth.RevocationList(source.Revocations),
			ResponseType:         toNAuthResponseTypeFromAPI(source.ResponseType),
			ResponseThreshold:    source.ResponseThreshold,
			Latency:              toNAuthServiceLatencyFromAPI(source.Latency),
			AccountTokenPosition: source.AccountTokenPosition,
			Advertise:            source.Advertise,
			AllowTrace:           source.AllowTrace,
		})
	}
	return result
}

func toNAuthResponseTypeFromAPI(source v1alpha1.ResponseType) nauth.ResponseType {
	switch source {
	case v1alpha1.ResponseTypeSingleton:
		return nauth.ResponseTypeSingleton
	case v1alpha1.ResponseTypeStream:
		return nauth.ResponseTypeStream
	case v1alpha1.ResponseTypeChunked:
		return nauth.ResponseTypeChunked
	default:
		return ""
	}
}

func toNAuthServiceLatencyFromAPI(source *v1alpha1.ServiceLatency) *nauth.ServiceLatency {
	if source == nil {
		return nil
	}

	return &nauth.ServiceLatency{
		Sampling: nauth.SamplingRate(source.Sampling),
		Results:  nauth.Subject(source.Results),
	}
}

func toNAuthExportTypeFromAPI(exportType v1alpha1.ExportType) nauth.ExportType {
	switch exportType {
	case v1alpha1.Stream:
		return nauth.ExportTypeStream
	case v1alpha1.Service:
		return nauth.ExportTypeService
	default:
		return nauth.ExportTypeUnknown
	}
}

var _ inbound.AccountManager = (*AccountManager)(nil)
var _ UserJWTSigner = (*AccountManager)(nil)
