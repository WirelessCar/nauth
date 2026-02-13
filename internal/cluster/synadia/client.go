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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiPathPrefix = "api/core/beta"
)

// Client is an HTTP client for the Synadia Cloud REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	getToken   func(ctx context.Context) (string, error)
}

// NewClient creates a Synadia API client. getToken is called per request (or cached by the caller).
func NewClient(baseURL string, getToken func(context.Context) (string, error)) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		getToken: getToken,
	}
}

// do executes a request with Bearer auth and optional JSON body; decodes response into out.
func (c *Client) do(ctx context.Context, method, urlPath string, body any, out any) (retErr error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}
	base := strings.TrimSuffix(c.baseURL, "/")
	u := base + "/" + apiPathPrefix + "/" + urlPath
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close response body: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %s (status %d)", method, u, string(slurp), resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// DTOs for Synadia API (snake_case for API compatibility).

// SystemItem is a system in list/get responses.
type SystemItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListSystemsResponse is the response for GET /teams/{teamId}/systems.
type ListSystemsResponse struct {
	Systems []SystemItem `json:"items,omitempty"`
}

// ListSystems returns systems for the given team.
func (c *Client) ListSystems(ctx context.Context, teamID string) (*ListSystemsResponse, error) {
	var out ListSystemsResponse
	if err := c.do(ctx, http.MethodGet, "teams/"+teamID+"/systems", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccountItem is an account in list responses.
type AccountItem struct {
	ID string `json:"id"`
}

// ListAccountsResponse is the response for GET /systems/{systemId}/accounts.
type ListAccountsResponse struct {
	Accounts []AccountItem `json:"items,omitempty"`
}

// ListAccounts returns accounts for the given system.
func (c *Client) ListAccounts(ctx context.Context, systemID string) (*ListAccountsResponse, error) {
	var out ListAccountsResponse
	if err := c.do(ctx, http.MethodGet, "systems/"+systemID+"/accounts", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TieredLimitsDTO is the tiered limits payload for Synadia API.
// GET account returns jwt_settings.limits.tiered_limits with keys "R1"/"R3"; PATCH uses the same.
// Fields intentionally omit "omitempty" so that removing a tier sends explicit null in PATCH requests.
type TieredLimitsDTO struct {
	R1 *TieredTierDTO `json:"R1"`
	R3 *TieredTierDTO `json:"R3"`
}

// TieredTierDTO is one tier (R1 or R3) in the API.
type TieredTierDTO struct {
	DiskStorage        *int64 `json:"disk_storage,omitempty"`
	DiskMaxStreamBytes *int64 `json:"disk_max_stream_bytes,omitempty"`
	Streams            *int64 `json:"streams,omitempty"`
	Consumer           *int64 `json:"consumer,omitempty"`
	MaxAckPending      *int64 `json:"max_ack_pending,omitempty"` // -1 = unlimited
	MaxBytesRequired   *bool  `json:"max_bytes_required,omitempty"`
}

// JwtSettingsLimitsDTO is limits inside jwt_settings.limits (GET account response and PATCH body).
// Fields map to OperatorLimits = NatsLimits + AccountLimits + JetStreamLimits + tiered_limits.
type JwtSettingsLimitsDTO struct {
	Subs         *int64           `json:"subs,omitempty"`
	Payload      *int64           `json:"payload,omitempty"`
	Data         *int64           `json:"data,omitempty"`
	Conn         *int64           `json:"conn,omitempty"`
	Leaf         *int64           `json:"leaf,omitempty"`
	Imports      *int64           `json:"imports,omitempty"`
	Exports      *int64           `json:"exports,omitempty"`
	Wildcards    *bool            `json:"wildcards,omitempty"`
	TieredLimits *TieredLimitsDTO `json:"tiered_limits,omitempty"`
}

// JwtSettingsDTO is jwt_settings in account GET response; used for PATCH body to update limits.
type JwtSettingsDTO struct {
	Limits *JwtSettingsLimitsDTO `json:"limits,omitempty"`
}

// AccountDTO is the full account in get/patch responses.
// Limits are only under jwt_settings.limits (core nats limits and tiered_limits with R1/R3).
type AccountDTO struct {
	ID               string          `json:"id"`
	AccountPublicKey string          `json:"account_public_key,omitempty"`
	JwtSettings      *JwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// NatsLimitsDTO is core NATS limits in the API.
type NatsLimitsDTO struct {
	Subs     *int64 `json:"subs,omitempty"`
	Payload  *int64 `json:"payload,omitempty"`
	Data     *int64 `json:"data,omitempty"`
	Conn     *int64 `json:"conn,omitempty"`
	Leaf     *int64 `json:"leaf,omitempty"`
	Import   *int64 `json:"import,omitempty"`
	Export   *int64 `json:"export,omitempty"`
	Wildcard *bool  `json:"wildcard,omitempty"`
}

// EffectiveTieredLimits returns tiered_limits from jwt_settings.limits.
func (a *AccountDTO) EffectiveTieredLimits() *TieredLimitsDTO {
	if a != nil && a.JwtSettings != nil && a.JwtSettings.Limits != nil {
		return a.JwtSettings.Limits.TieredLimits
	}
	return nil
}

// EffectiveNatsLimits returns core NATS limits from jwt_settings.limits.
// Nil Imports/Exports/Wildcards from the API are treated as defaults for comparison.
func (a *AccountDTO) EffectiveNatsLimits() *NatsLimitsDTO {
	if a != nil && a.JwtSettings != nil && a.JwtSettings.Limits != nil {
		l := a.JwtSettings.Limits
		out := &NatsLimitsDTO{Subs: l.Subs, Payload: l.Payload, Data: l.Data, Conn: l.Conn, Leaf: l.Leaf,
			Import: l.Imports, Export: l.Exports, Wildcard: l.Wildcards}
		if out.Import == nil {
			out.Import = int64Ptr(DefaultImportUnlimited)
		}
		if out.Export == nil {
			out.Export = int64Ptr(DefaultExportUnlimited)
		}
		if out.Wildcard == nil {
			out.Wildcard = boolPtr(DefaultWildcard)
		}
		return out
	}
	return nil
}

// GetAccount returns a single account by ID.
func (c *Client) GetAccount(ctx context.Context, accountID string) (*AccountDTO, error) {
	var out AccountDTO
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAccountRequest is the body for POST /api/core/beta/systems/{systemId}/accounts.
// Per OpenAPI AccountCreateRequest, limits are nested under jwt_settings.limits.
type CreateAccountRequest struct {
	Name        string          `json:"name,omitempty"`
	JwtSettings *JwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// CreateAccount creates an account in the system.
func (c *Client) CreateAccount(ctx context.Context, systemID string, req *CreateAccountRequest) (*AccountDTO, error) {
	var out AccountDTO
	if err := c.do(ctx, http.MethodPost, "systems/"+systemID+"/accounts", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchAccountRequest is the body for PATCH /accounts/{accountId}.
// Limits are sent under jwt_settings.limits per API schema.
type PatchAccountRequest struct {
	JwtSettings *JwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// PatchAccount updates an account.
func (c *Client) PatchAccount(ctx context.Context, accountID string, req *PatchAccountRequest) (*AccountDTO, error) {
	var out AccountDTO
	if err := c.do(ctx, http.MethodPatch, "accounts/"+accountID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAccount deletes an account (DELETE /api/core/beta/accounts/{accountId}).
func (c *Client) DeleteAccount(ctx context.Context, accountID string) error {
	return c.do(ctx, http.MethodDelete, "accounts/"+accountID, nil, nil)
}

// JetStreamResourceItem is a generic item in JetStream list responses (streams, KV/object-store buckets).
type JetStreamResourceItem struct {
	Name string `json:"name"`
}

// ListJetStreamResourcesResponse is the response for JetStream list endpoints.
type ListJetStreamResourcesResponse struct {
	Items []JetStreamResourceItem `json:"items,omitempty"`
}

// ListStreams returns JetStream streams for the given account.
func (c *Client) ListStreams(ctx context.Context, accountID string) (*ListJetStreamResourcesResponse, error) {
	var out ListJetStreamResourcesResponse
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID+"/jetstream/streams", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListKVBuckets returns JetStream KV buckets for the given account.
func (c *Client) ListKVBuckets(ctx context.Context, accountID string) (*ListJetStreamResourcesResponse, error) {
	var out ListJetStreamResourcesResponse
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID+"/jetstream/kv-buckets", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListObjectBuckets returns JetStream object-store buckets for the given account.
func (c *Client) ListObjectBuckets(ctx context.Context, accountID string) (*ListJetStreamResourcesResponse, error) {
	var out ListJetStreamResourcesResponse
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID+"/jetstream/object-buckets", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SigningKeyGroupItem is a signing key group in list responses.
type SigningKeyGroupItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListSigningKeyGroupsResponse is the response for GET /accounts/{accountId}/signing-key-groups.
type ListSigningKeyGroupsResponse struct {
	Items []SigningKeyGroupItem `json:"items,omitempty"`
}

// ListSigningKeyGroups returns signing key groups for the given account.
func (c *Client) ListSigningKeyGroups(ctx context.Context, accountID string) (*ListSigningKeyGroupsResponse, error) {
	var out ListSigningKeyGroupsResponse
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID+"/account-sk-groups", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NatsUserItem is a user in list responses.
type NatsUserItem struct {
	ID string `json:"id"`
}

// ListNatsUsersResponse is the response for GET /accounts/{accountId}/nats-users.
type ListNatsUsersResponse struct {
	NatsUsers []NatsUserItem `json:"items,omitempty"`
}

// ListNatsUsers returns NATS users for the given account.
func (c *Client) ListNatsUsers(ctx context.Context, accountID string) (*ListNatsUsersResponse, error) {
	var out ListNatsUsersResponse
	if err := c.do(ctx, http.MethodGet, "accounts/"+accountID+"/nats-users", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PermissionDTO represents pub or sub permission (allow/deny subject lists).
type PermissionDTO struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ResponsePermissionDTO represents response permissions (max messages and TTL).
type ResponsePermissionDTO struct {
	MaxMsgs int   `json:"max,omitempty"`
	Expires int64 `json:"ttl,omitempty"` // nanoseconds
}

// NatsUserJwtSettingsDTO represents jwt_settings in NATS user responses and requests.
// Per OpenAPI NatsUserJwtSettings / NatsCreateUserJwtSettings, subs/data/payload and permissions
// are at the top level under jwt_settings.
type NatsUserJwtSettingsDTO struct {
	Subs    *int64                 `json:"subs,omitempty"`
	Data    *int64                 `json:"data,omitempty"`
	Payload *int64                 `json:"payload,omitempty"`
	Pub     *PermissionDTO         `json:"pub,omitempty"`
	Sub     *PermissionDTO         `json:"sub,omitempty"`
	Resp    *ResponsePermissionDTO `json:"resp,omitempty"`
}

// NatsUserDTO is the full user in get/patch responses.
type NatsUserDTO struct {
	ID            string                  `json:"id"`
	UserPublicKey string                  `json:"user_public_key,omitempty"`
	JwtSettings   *NatsUserJwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// EffectiveNatsLimits returns user NATS limits from jwt_settings.
func (u *NatsUserDTO) EffectiveNatsLimits() *NatsLimitsDTO {
	if u != nil && u.JwtSettings != nil {
		return &NatsLimitsDTO{
			Subs:    u.JwtSettings.Subs,
			Payload: u.JwtSettings.Payload,
			Data:    u.JwtSettings.Data,
		}
	}
	return nil
}

// GetNatsUser returns a single NATS user by ID.
func (c *Client) GetNatsUser(ctx context.Context, userID string) (*NatsUserDTO, error) {
	var out NatsUserDTO
	if err := c.do(ctx, http.MethodGet, "nats-users/"+userID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateNatsUserRequest is the body for POST /api/core/beta/accounts/{accountId}/nats-users.
// Per OpenAPI NatsUserCreateRequest, limits go under jwt_settings (subs/data/payload).
type CreateNatsUserRequest struct {
	Name        string                  `json:"name,omitempty"`
	SKGroupID   string                  `json:"sk_group_id,omitempty"`
	JwtSettings *NatsUserJwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// CreateNatsUser creates a NATS user under the account.
func (c *Client) CreateNatsUser(ctx context.Context, accountID string, req *CreateNatsUserRequest) (
	*NatsUserDTO, error) {
	var out NatsUserDTO
	if err := c.do(ctx, http.MethodPost, "accounts/"+accountID+"/nats-users", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchNatsUserRequest is the body for PATCH /api/core/beta/nats-users/{userId}.
// Per OpenAPI NatsUserUpdateRequest, limits go under jwt_settings (subs/data/payload).
type PatchNatsUserRequest struct {
	JwtSettings *NatsUserJwtSettingsDTO `json:"jwt_settings,omitempty"`
}

// PatchNatsUser updates a NATS user.
func (c *Client) PatchNatsUser(ctx context.Context, userID string, req *PatchNatsUserRequest) (*NatsUserDTO, error) {
	var out NatsUserDTO
	if err := c.do(ctx, http.MethodPatch, "nats-users/"+userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteNatsUser deletes a NATS user (DELETE /api/core/beta/nats-users/{userId}).
func (c *Client) DeleteNatsUser(ctx context.Context, userID string) error {
	return c.do(ctx, http.MethodDelete, "nats-users/"+userID, nil, nil)
}

// GetNatsUserCreds returns the credentials file body for the user (POST .../creds).
func (c *Client) GetNatsUserCreds(ctx context.Context, userID string) (creds []byte, retErr error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	base := strings.TrimSuffix(c.baseURL, "/")
	u := base + "/" + apiPathPrefix + "/nats-users/" + userID + "/creds"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close response body: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("POST %s: %s (status %d)", u, string(slurp), resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
