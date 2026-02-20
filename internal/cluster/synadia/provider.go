/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS FOR A PARTICULAR PURPOSE.
See the License for the specific language governing permissions and
limitations under the License.
*/

package synadia

import (
	"context"
	"fmt"
	"sort"
	"time"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const defaultReconcileInterval = 5 * time.Minute

// Provider implements cluster.Provider for Synadia Cloud via the REST API.
type Provider struct {
	apiClient     *Client
	system        *synadiav1alpha1.System
	k8sClient     client.Client
	secretApplier SecretApplier
}

// SecretApplier writes user credentials to a Secret (same shape as nauth).
type SecretApplier interface {
	Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error
	Delete(ctx context.Context, namespace string, name string) error
}

// NewProvider creates a Synadia provider.
func NewProvider(
	apiClient *Client,
	system *synadiav1alpha1.System,
	k8sClient client.Client,
	secretApplier SecretApplier,
) *Provider {
	return &Provider{
		apiClient:     apiClient,
		system:        system,
		k8sClient:     k8sClient,
		secretApplier: secretApplier,
	}
}

// requeueAfter returns the duration to requeue from System.spec.reconcileInterval (default 5m).
func (p *Provider) requeueAfter() time.Duration {
	if p.system.Spec.ReconcileInterval != nil && p.system.Spec.ReconcileInterval.Duration > 0 {
		return p.system.Spec.ReconcileInterval.Duration
	}
	return defaultReconcileInterval
}

// CreateAccount creates a new account. Synadia uses idempotent createOrUpdate internally.
func (p *Provider) CreateAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	return p.createOrUpdateAccount(ctx, account)
}

// UpdateAccount updates an existing account. Synadia uses idempotent createOrUpdate internally.
func (p *Provider) UpdateAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	return p.createOrUpdateAccount(ctx, account)
}

// ImportAccount syncs state for observe policy. Synadia uses idempotent createOrUpdate internally.
func (p *Provider) ImportAccount(ctx context.Context, account *nauthv1alpha1.Account) (*cluster.AccountResult, error) {
	return p.createOrUpdateAccount(ctx, account)
}

// createOrUpdateAccount is the idempotent implementation used by CreateAccount, UpdateAccount, ImportAccount.
func (p *Provider) createOrUpdateAccount(
	ctx context.Context, account *nauthv1alpha1.Account,
) (*cluster.AccountResult, error) {
	systemID := p.system.Status.SystemID
	if systemID == "" {
		return nil, fmt.Errorf("system %s/%s has no systemId in status", p.system.GetNamespace(), p.system.GetName())
	}

	accountID := ""
	managementPolicy := ""
	if account.Labels != nil {
		accountID = account.Labels[k8s.LabelAccountID]
		managementPolicy = account.Labels[k8s.LabelManagementPolicy]
	}

	interval := p.requeueAfter()
	result := &cluster.AccountResult{
		RequeueAfter: &interval,
	}

	// Observe: only GET and return state
	if managementPolicy == k8s.LabelManagementPolicyObserveValue {
		if accountID == "" {
			return nil, fmt.Errorf("observe policy requires account to already have an account ID")
		}
		got, err := p.apiClient.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		result.AccountID = got.ID
		result.AccountSignedBy = got.AccountPublicKey
		result.Claims = p.claimsFromAccountDTO(got)
		return result, nil
	}

	if accountID == "" {
		// Create account – limits go under jwt_settings.limits per OpenAPI AccountCreateRequest.
		tieredLimits := p.tieredLimitsForAccount(ctx, account)
		desiredNats := NatsLimitsFromAccount(&account.Spec)
		createReq := &CreateAccountRequest{
			Name: account.Spec.DisplayName,
			JwtSettings: &JwtSettingsDTO{
				Limits: &JwtSettingsLimitsDTO{
					TieredLimits: tieredLimits,
					Subs:         desiredNats.Subs,
					Payload:      desiredNats.Payload,
					Data:         desiredNats.Data,
					Conn:         desiredNats.Conn,
					Leaf:         desiredNats.Leaf,
					Imports:      desiredNats.Import,
					Exports:      desiredNats.Export,
					Wildcards:    desiredNats.Wildcard,
				},
			},
		}
		if createReq.Name == "" {
			createReq.Name = account.GetNamespace() + "/" + account.GetName()
		}
		created, err := p.apiClient.CreateAccount(ctx, systemID, createReq)
		if err != nil {
			return nil, fmt.Errorf("create account: %w", err)
		}
		result.AccountID = created.ID
		result.AccountSignedBy = created.AccountPublicKey
		result.Claims = p.claimsFromAccountDTO(created)
		return result, nil
	}

	// Update: GET and PATCH if needed
	got, err := p.apiClient.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	result.AccountID = got.ID
	result.AccountSignedBy = got.AccountPublicKey
	result.Claims = p.claimsFromAccountDTO(got)

	// Detect account nkey rotation (public key changed)
	storedKey := ""
	if account.Labels != nil {
		storedKey = account.Labels[k8s.LabelAccountSignedBy]
	}
	if got.AccountPublicKey != "" && storedKey != "" && got.AccountPublicKey != storedKey {
		result.AccountNkeyRotated = true
	}

	desiredTiered := p.tieredLimitsForAccount(ctx, account)
	desiredNats := NatsLimitsFromAccount(&account.Spec)
	gotTiered := got.EffectiveTieredLimits()
	gotNats := got.EffectiveNatsLimits()
	if !tieredLimitsEqual(gotTiered, desiredTiered) || !natsLimitsEqual(gotNats, desiredNats) {
		patched, err := p.apiClient.PatchAccount(ctx, accountID, &PatchAccountRequest{
			JwtSettings: &JwtSettingsDTO{
				Limits: &JwtSettingsLimitsDTO{
					TieredLimits: desiredTiered,
					Subs:         desiredNats.Subs,
					Payload:      desiredNats.Payload,
					Data:         desiredNats.Data,
					Conn:         desiredNats.Conn,
					Leaf:         desiredNats.Leaf,
					Imports:      desiredNats.Import,
					Exports:      desiredNats.Export,
					Wildcards:    desiredNats.Wildcard,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("patch account: %w", err)
		}
		result.Claims = p.claimsFromAccountDTO(patched)
	}
	return result, nil
}

func (p *Provider) claimsFromAccountDTO(dto *AccountDTO) *nauthv1alpha1.AccountClaims {
	if dto == nil {
		return nil
	}
	nats := dto.EffectiveNatsLimits()
	if nats == nil {
		return &nauthv1alpha1.AccountClaims{}
	}
	return &nauthv1alpha1.AccountClaims{
		NatsLimits: &nauthv1alpha1.NatsLimits{
			Subs:    nats.Subs,
			Data:    nats.Data,
			Payload: nats.Payload,
		},
	}
}

func tieredLimitsEqual(a, b *TieredLimitsDTO) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return tierEqual(a.R1, b.R1) && tierEqual(a.R3, b.R3)
}

func tierEqual(a, b *TieredTierDTO) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return ptrEqual(a.DiskStorage, b.DiskStorage) && ptrEqual(a.DiskMaxStreamBytes, b.DiskMaxStreamBytes) &&
		ptrEqual(a.Streams, b.Streams) && ptrEqual(a.Consumer, b.Consumer) &&
		ptrEqual(a.MaxAckPending, b.MaxAckPending) && ptrBoolEqual(a.MaxBytesRequired, b.MaxBytesRequired)
}

func natsLimitsEqual(a, b *NatsLimitsDTO) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return ptrEqual(a.Subs, b.Subs) && ptrEqual(a.Payload, b.Payload) && ptrEqual(a.Data, b.Data) &&
		ptrEqual(a.Conn, b.Conn) && ptrEqual(a.Leaf, b.Leaf) &&
		ptrEqual(a.Import, b.Import) && ptrEqual(a.Export, b.Export) && ptrBoolEqual(a.Wildcard, b.Wildcard)
}

func ptrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrBoolEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// tieredLimitsForAccount looks up the single TieredLimit for the account and builds the API DTO.
func (p *Provider) tieredLimitsForAccount(ctx context.Context, account *nauthv1alpha1.Account) *TieredLimitsDTO {
	tl := p.getTieredLimitForAccount(ctx, account)
	return TieredLimitsFromTieredLimit(tl)
}

// getTieredLimitForAccount returns the single TieredLimit in use for this account.
// Exactly one TieredLimit per account is used; the controller marks it via status.selectedForAccount.
// If multiple TieredLimits reference the same account, the one with status.selectedForAccount set is used;
// otherwise the controller picks one deterministically (oldest by creation, then namespace/name), sets the mark, and clears it on others.
func (p *Provider) getTieredLimitForAccount(
	ctx context.Context, account *nauthv1alpha1.Account,
) *synadiav1alpha1.TieredLimit {
	list := &synadiav1alpha1.TieredLimitList{}
	if err := p.k8sClient.List(ctx, list); err != nil {
		return nil
	}
	accountNs := account.GetNamespace()
	accountName := account.GetName()

	// Collect candidates: TieredLimits that reference this account.
	var candidates []*synadiav1alpha1.TieredLimit
	for i := range list.Items {
		tl := &list.Items[i]
		refNs := tl.Spec.AccountRef.Namespace
		if refNs == "" {
			refNs = tl.GetNamespace()
		}
		if tl.Spec.AccountRef.Name == accountName && refNs == accountNs {
			candidates = append(candidates, tl)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		// Only one candidate; ensure it is marked so we only use it going forward.
		chosen := candidates[0]
		if !selectedForAccountEquals(chosen.Status.SelectedForAccount, accountNs, accountName) {
			p.patchTieredLimitSelectedForAccount(ctx, chosen, accountNs, accountName)
		}
		return chosen
	}

	// Multiple candidates: use the one already marked, or pick one deterministically and mark it.
	var alreadySelected *synadiav1alpha1.TieredLimit
	for _, tl := range candidates {
		if selectedForAccountEquals(tl.Status.SelectedForAccount, accountNs, accountName) {
			alreadySelected = tl
			break
		}
	}
	if alreadySelected != nil {
		return alreadySelected
	}

	// Pick one deterministically: oldest by CreationTimestamp, then namespace/name.
	sort.Slice(candidates, func(i, j int) bool {
		ti, tj := candidates[i].CreationTimestamp, candidates[j].CreationTimestamp
		if !ti.Time.Equal(tj.Time) {
			return ti.Before(&tj)
		}
		ni, nj := candidates[i].GetNamespace()+"/"+candidates[i].GetName(), candidates[j].GetNamespace()+"/"+candidates[j].GetName()
		return ni < nj
	})
	chosen := candidates[0]

	// Clear selectedForAccount from any other candidate that had it set for this account.
	for _, tl := range candidates[1:] {
		if selectedForAccountEquals(tl.Status.SelectedForAccount, accountNs, accountName) {
			p.patchTieredLimitSelectedForAccount(ctx, tl, "", "")
		}
	}
	// Mark the chosen one.
	p.patchTieredLimitSelectedForAccount(ctx, chosen, accountNs, accountName)
	return chosen
}

// selectedForAccountEquals returns true when ref designates the given account (namespace/name).
// Empty namespace in ref is treated as "same namespace as the TieredLimit" at selection time; when comparing we require ref.Namespace == accountNs when set.
func selectedForAccountEquals(ref *synadiav1alpha1.AccountRef, accountNs, accountName string) bool {
	if ref == nil || ref.Name != accountName {
		return false
	}
	if ref.Namespace == "" {
		return true
	}
	return ref.Namespace == accountNs
}

// patchTieredLimitSelectedForAccount sets status.selectedForAccount on tl. Pass accountNs/accountName empty to clear.
func (p *Provider) patchTieredLimitSelectedForAccount(ctx context.Context, tl *synadiav1alpha1.TieredLimit, accountNs, accountName string) {
	logger := log.FromContext(ctx)
	base := tl.DeepCopy()
	if accountName == "" {
		tl.Status.SelectedForAccount = nil
	} else {
		tl.Status.SelectedForAccount = &synadiav1alpha1.AccountRef{Name: accountName, Namespace: accountNs}
	}
	if err := p.k8sClient.Status().Patch(ctx, tl, client.MergeFrom(base)); err != nil {
		logger.Error(err, "failed to patch TieredLimit selectedForAccount", "tieredLimit", tl.GetNamespace()+"/"+tl.GetName())
	}
}

// defaultSKGroupID returns the ID of the "Default" signing key group for the account.
func (p *Provider) defaultSKGroupID(ctx context.Context, accountID string) (string, error) {
	resp, err := p.apiClient.ListSigningKeyGroups(ctx, accountID)
	if err != nil {
		return "", fmt.Errorf("list signing key groups for account %s: %w", accountID, err)
	}
	for _, g := range resp.Items {
		if g.Name == "Default" {
			return g.ID, nil
		}
	}
	// If no group named "Default", use the first available group.
	if len(resp.Items) > 0 {
		return resp.Items[0].ID, nil
	}
	return "", fmt.Errorf("no signing key groups found for account %s", accountID)
}

// userJwtSettingsFromSpec builds the full NatsUserJwtSettingsDTO from the User spec,
// including limits and permissions.
func userJwtSettingsFromSpec(spec *nauthv1alpha1.UserSpec) *NatsUserJwtSettingsDTO {
	nats := NatsLimitsFromUser(spec)
	jwt := &NatsUserJwtSettingsDTO{
		Subs:    nats.Subs,
		Data:    nats.Data,
		Payload: nats.Payload,
	}
	if spec.Permissions != nil {
		if !spec.Permissions.Pub.Empty() {
			jwt.Pub = &PermissionDTO{
				Allow: spec.Permissions.Pub.Allow,
				Deny:  spec.Permissions.Pub.Deny,
			}
		}
		if !spec.Permissions.Sub.Empty() {
			jwt.Sub = &PermissionDTO{
				Allow: spec.Permissions.Sub.Allow,
				Deny:  spec.Permissions.Sub.Deny,
			}
		}
		if spec.Permissions.Resp != nil {
			jwt.Resp = &ResponsePermissionDTO{
				MaxMsgs: spec.Permissions.Resp.MaxMsgs,
				Expires: int64(spec.Permissions.Resp.Expires),
			}
		}
	}
	return jwt
}

// DeleteAccount removes the account from Synadia via DELETE /api/core/beta/accounts/{accountId}.
// It refuses to delete the account when JetStream resources (streams, KV buckets, or
// object-store buckets) still exist, requiring the user to remove them first.
func (p *Provider) DeleteAccount(ctx context.Context, account *nauthv1alpha1.Account) error {
	accountID := ""
	if account.Labels != nil {
		accountID = account.Labels[k8s.LabelAccountID]
	}
	if accountID == "" {
		return nil // nothing to delete on the API side
	}

	// Block deletion when JetStream resources are still present.
	names, err := p.jetStreamResourceNames(ctx, accountID)
	if err != nil {
		return fmt.Errorf("cannot verify JetStream resources before deleting account: %w", err)
	}
	if len(names) > 0 {
		return fmt.Errorf("cannot delete account: JetStream resources still exist (%v). Delete them manually before removing the account", names)
	}

	return p.apiClient.DeleteAccount(ctx, accountID)
}

// jetStreamResourceNames returns the names of all JetStream resources (streams,
// KV buckets, object-store buckets) that belong to the account. An empty slice
// means the account has no JetStream resources.
func (p *Provider) jetStreamResourceNames(ctx context.Context, accountID string) ([]string, error) {
	streams, err := p.apiClient.ListStreams(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}

	kvBuckets, err := p.apiClient.ListKVBuckets(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list kv buckets: %w", err)
	}

	objectBuckets, err := p.apiClient.ListObjectBuckets(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list object buckets: %w", err)
	}

	names := make([]string, 0, len(streams.Items)+len(kvBuckets.Items)+len(objectBuckets.Items))
	for _, s := range streams.Items {
		names = append(names, "stream/"+s.Name)
	}
	for _, b := range kvBuckets.Items {
		names = append(names, "kv/"+b.Name)
	}
	for _, b := range objectBuckets.Items {
		names = append(names, "object/"+b.Name)
	}

	return names, nil
}

// CreateOrUpdateUser creates or updates a user and stores credentials in a Secret.
func (p *Provider) CreateOrUpdateUser(ctx context.Context, user *nauthv1alpha1.User) (*cluster.UserResult, error) {
	account, err := p.getAccountForUser(ctx, user)
	if err != nil {
		return nil, err
	}
	accountID := ""
	if account.Labels != nil {
		accountID = account.Labels[k8s.LabelAccountID]
	}
	if accountID == "" {
		return nil, fmt.Errorf("account %s/%s has no account ID", account.GetNamespace(), account.GetName())
	}

	userID := ""
	if user.Labels != nil {
		userID = user.Labels[k8s.LabelUserID]
	}

	interval := p.requeueAfter()
	result := &cluster.UserResult{RequeueAfter: &interval}

	desiredJwt := userJwtSettingsFromSpec(&user.Spec)

	if userID == "" {
		// Create user – per OpenAPI NatsUserCreateRequest, limits go under jwt_settings.
		// sk_group_id is required; look up the "Default" signing key group for the account.
		skGroupID, err := p.defaultSKGroupID(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("resolve signing key group: %w", err)
		}
		createReq := &CreateNatsUserRequest{
			Name:        user.Spec.DisplayName,
			SKGroupID:   skGroupID,
			JwtSettings: desiredJwt,
		}
		if createReq.Name == "" {
			createReq.Name = user.GetNamespace() + "/" + user.GetName()
		}
		created, err := p.apiClient.CreateNatsUser(ctx, accountID, createReq)
		if err != nil {
			return nil, fmt.Errorf("create user: %w", err)
		}
		userID = created.ID
		result.UserID = created.ID
		result.UserSignedBy = created.UserPublicKey
		result.Claims = p.claimsFromUserDTO(created)
	} else {
		got, err := p.apiClient.GetNatsUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		result.UserID = got.ID
		result.UserSignedBy = got.UserPublicKey
		// Always patch to keep limits and permissions in sync.
		patched, err := p.apiClient.PatchNatsUser(ctx, userID, &PatchNatsUserRequest{
			JwtSettings: desiredJwt,
		})
		if err != nil {
			return nil, fmt.Errorf("patch user: %w", err)
		}
		result.Claims = p.claimsFromUserDTO(patched)
	}

	// Fetch credentials and store in Secret (same shape as nauth)
	creds, err := p.apiClient.GetNatsUserCreds(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user creds: %w", err)
	}
	owner := &secret.Owner{Owner: user}
	secretMeta := metav1.ObjectMeta{
		Name:      user.GetUserSecretName(),
		Namespace: user.GetNamespace(),
		Labels: map[string]string{
			k8s.LabelSecretType: k8s.SecretTypeUserCredentials,
			k8s.LabelManaged:    k8s.LabelManagedValue,
		},
	}
	secretData := map[string]string{k8s.UserCredentialSecretKeyName: string(creds)}
	if err := p.secretApplier.Apply(ctx, owner, secretMeta, secretData); err != nil {
		return nil, fmt.Errorf("write user secret: %w", err)
	}

	if user.Labels == nil {
		user.Labels = make(map[string]string)
	}
	user.Labels[k8s.LabelUserID] = result.UserID
	user.Labels[k8s.LabelUserAccountID] = accountID
	user.Labels[k8s.LabelUserSignedBy] = result.UserSignedBy
	if result.Claims != nil {
		user.Status.Claims = *result.Claims
	}
	return result, nil
}

func (p *Provider) getAccountForUser(ctx context.Context, user *nauthv1alpha1.User) (*nauthv1alpha1.Account, error) {
	acc := &nauthv1alpha1.Account{}
	key := client.ObjectKey{Namespace: user.GetNamespace(), Name: user.Spec.AccountName}
	if err := p.k8sClient.Get(ctx, key, acc); err != nil {
		return nil, fmt.Errorf("get account %s: %w", user.Spec.AccountName, err)
	}
	return acc, nil
}

func (p *Provider) claimsFromUserDTO(dto *NatsUserDTO) *nauthv1alpha1.UserClaims {
	if dto == nil {
		return nil
	}
	c := &nauthv1alpha1.UserClaims{}
	if dto.JwtSettings != nil {
		c.NatsLimits = &nauthv1alpha1.NatsLimits{
			Subs:    dto.JwtSettings.Subs,
			Data:    dto.JwtSettings.Data,
			Payload: dto.JwtSettings.Payload,
		}
	}
	return c
}

// DeleteUser deletes the user from Synadia (DELETE /api/core/beta/nats-users/{userId}) and removes the credentials Secret.
func (p *Provider) DeleteUser(ctx context.Context, user *nauthv1alpha1.User) error {
	userID := ""
	if user.Labels != nil {
		userID = user.Labels[k8s.LabelUserID]
	}
	if userID != "" {
		if err := p.apiClient.DeleteNatsUser(ctx, userID); err != nil {
			return fmt.Errorf("delete user from Synadia: %w", err)
		}
	}
	if err := p.secretApplier.Delete(ctx, user.GetNamespace(), user.GetUserSecretName()); err != nil {
		return fmt.Errorf("delete user secret: %w", err)
	}
	return nil
}

// Ensure Provider implements cluster.Provider.
var _ cluster.Provider = (*Provider)(nil)
