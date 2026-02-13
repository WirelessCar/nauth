/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package synadia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ListSystems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/teams/team-1/systems", r.URL.Path)
		assert.Equal(t, "Bearer token1", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListSystemsResponse{
			Systems: []SystemItem{{ID: "sys-1", Name: "NGS"}, {ID: "sys-2", Name: "Other"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "token1", nil })
	ctx := context.Background()

	resp, err := client.ListSystems(ctx, "team-1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Systems, 2)
	assert.Equal(t, "sys-1", resp.Systems[0].ID)
	assert.Equal(t, "NGS", resp.Systems[0].Name)
}

func TestClient_GetAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AccountDTO{
			ID:               "acc-1",
			AccountPublicKey: "ACxxx",
			JwtSettings: &JwtSettingsDTO{
				Limits: &JwtSettingsLimitsDTO{
					Subs: int64Ptr(1), Conn: int64Ptr(1),
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	acc, err := client.GetAccount(ctx, "acc-1")
	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, "acc-1", acc.ID)
	assert.Equal(t, "ACxxx", acc.AccountPublicKey)
	assert.Equal(t, int64(1), *acc.EffectiveNatsLimits().Subs)
}

func TestClient_GetAccount_jwt_settings_limits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AccountDTO{
			ID:               "acc-1",
			AccountPublicKey: "ACyyy",
			JwtSettings: &JwtSettingsDTO{
				Limits: &JwtSettingsLimitsDTO{
					Subs:    int64Ptr(10),
					Conn:    int64Ptr(2),
					Payload: int64Ptr(2048),
					TieredLimits: &TieredLimitsDTO{
						R1: &TieredTierDTO{Streams: int64Ptr(5)},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	acc, err := client.GetAccount(ctx, "acc-1")
	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, "acc-1", acc.ID)
	assert.Equal(t, "ACyyy", acc.AccountPublicKey)
	nats := acc.EffectiveNatsLimits()
	require.NotNil(t, nats)
	assert.Equal(t, int64(10), *nats.Subs)
	assert.Equal(t, int64(2), *nats.Conn)
	assert.Equal(t, int64(2048), *nats.Payload)
	tiered := acc.EffectiveTieredLimits()
	require.NotNil(t, tiered)
	require.NotNil(t, tiered.R1)
	assert.Equal(t, int64(5), *tiered.R1.Streams)
}

func TestClient_GetAccount_parses_real_jwt_settings_response(t *testing.T) {
	// Real GET account response shape: jwt_settings.limits (core nats) and tiered_limits (R1/R3 uppercase).
	body := `{
		"id": "38suzPOvm0xTHDQxIwhuYmUrwtj",
		"account_public_key": "ABWBJWCH3V2S5EIJRYLZUULUYTQ2LDB27UAEOQROMP2OOTLXEC3HMJP6",
		"jwt_settings": {
			"limits": {
				"subs": 2,
				"data": 1024,
				"payload": 500,
				"conn": 10,
				"tiered_limits": {
					"R1": {
						"disk_storage": 1073741824,
						"streams": 1,
						"consumer": 2,
						"disk_max_stream_bytes": 1073741824,
						"max_bytes_required": true
					},
					"R3": {
						"disk_storage": 1073741824,
						"streams": 1,
						"consumer": 2,
						"disk_max_stream_bytes": 1073741824,
						"max_bytes_required": true
					}
				}
			}
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	acc, err := client.GetAccount(ctx, "acc-1")
	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, "38suzPOvm0xTHDQxIwhuYmUrwtj", acc.ID)
	assert.Equal(t, "ABWBJWCH3V2S5EIJRYLZUULUYTQ2LDB27UAEOQROMP2OOTLXEC3HMJP6", acc.AccountPublicKey)

	nats := acc.EffectiveNatsLimits()
	require.NotNil(t, nats)
	assert.Equal(t, int64(2), *nats.Subs)
	assert.Equal(t, int64(1024), *nats.Data)
	assert.Equal(t, int64(500), *nats.Payload)
	assert.Equal(t, int64(10), *nats.Conn)

	tiered := acc.EffectiveTieredLimits()
	require.NotNil(t, tiered)
	require.NotNil(t, tiered.R1)
	assert.Equal(t, int64(1073741824), *tiered.R1.DiskStorage)
	assert.Equal(t, int64(1), *tiered.R1.Streams)
	assert.Equal(t, int64(2), *tiered.R1.Consumer)
	assert.Equal(t, int64(1073741824), *tiered.R1.DiskMaxStreamBytes)
	assert.True(t, *tiered.R1.MaxBytesRequired)

	require.NotNil(t, tiered.R3)
	assert.Equal(t, int64(1073741824), *tiered.R3.DiskStorage)
	assert.Equal(t, int64(1), *tiered.R3.Streams)
	assert.Equal(t, int64(2), *tiered.R3.Consumer)
	assert.Equal(t, int64(1073741824), *tiered.R3.DiskMaxStreamBytes)
	assert.True(t, *tiered.R3.MaxBytesRequired)
}

func TestClient_CreateAccount(t *testing.T) {
	var capturedBody CreateAccountRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/systems/sys-1/accounts", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AccountDTO{
			ID:               "new-acc",
			AccountPublicKey: "ACnew",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	req := &CreateAccountRequest{
		Name: "my-account",
		JwtSettings: &JwtSettingsDTO{
			Limits: &JwtSettingsLimitsDTO{
				Subs: int64Ptr(1), Conn: int64Ptr(1), Payload: int64Ptr(1024), Data: int64Ptr(1024), Leaf: int64Ptr(0),
			},
		},
	}
	acc, err := client.CreateAccount(ctx, "sys-1", req)
	require.NoError(t, err)
	require.NotNil(t, acc)
	assert.Equal(t, "new-acc", acc.ID)
	assert.Equal(t, "my-account", capturedBody.Name)
	require.NotNil(t, capturedBody.JwtSettings)
	require.NotNil(t, capturedBody.JwtSettings.Limits)
	assert.Equal(t, int64(1), *capturedBody.JwtSettings.Limits.Subs)
}

func TestClient_PatchAccount(t *testing.T) {
	var patchBody PatchAccountRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1", r.URL.Path)
		assert.Equal(t, http.MethodPatch, r.Method)
		_ = json.NewDecoder(r.Body).Decode(&patchBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AccountDTO{ID: "acc-1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	_, err := client.PatchAccount(ctx, "acc-1", &PatchAccountRequest{
		JwtSettings: &JwtSettingsDTO{
			Limits: &JwtSettingsLimitsDTO{
				Subs: int64Ptr(5),
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, patchBody.JwtSettings)
	require.NotNil(t, patchBody.JwtSettings.Limits)
	assert.Equal(t, int64(5), *patchBody.JwtSettings.Limits.Subs)
}

func TestClient_ListNatsUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1/nats-users", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListNatsUsersResponse{
			NatsUsers: []NatsUserItem{{ID: "user-1"}, {ID: "user-2"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	resp, err := client.ListNatsUsers(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, resp.NatsUsers, 2)
	assert.Equal(t, "user-1", resp.NatsUsers[0].ID)
}

func TestClient_CreateNatsUser(t *testing.T) {
	var capturedBody CreateNatsUserRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1/nats-users", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NatsUserDTO{
			ID: "nu-1", UserPublicKey: "Uxxx",
			JwtSettings: &NatsUserJwtSettingsDTO{Subs: int64Ptr(10)},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	u, err := client.CreateNatsUser(ctx, "acc-1", &CreateNatsUserRequest{
		Name:        "u1",
		JwtSettings: &NatsUserJwtSettingsDTO{Subs: int64Ptr(10)},
	})
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "nu-1", u.ID)
	assert.Equal(t, "u1", capturedBody.Name)
	require.NotNil(t, capturedBody.JwtSettings)
	assert.Equal(t, int64(10), *capturedBody.JwtSettings.Subs)
}

func TestClient_GetNatsUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/nats-users/nu-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NatsUserDTO{
			ID: "nu-1", UserPublicKey: "Uxxx",
			JwtSettings: &NatsUserJwtSettingsDTO{Subs: int64Ptr(5), Data: int64Ptr(-1), Payload: int64Ptr(1024)},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	u, err := client.GetNatsUser(ctx, "nu-1")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "nu-1", u.ID)
	require.NotNil(t, u.JwtSettings)
	assert.Equal(t, int64(5), *u.JwtSettings.Subs)
	nats := u.EffectiveNatsLimits()
	require.NotNil(t, nats)
	assert.Equal(t, int64(5), *nats.Subs)
}

func TestClient_GetNatsUserCreds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/nats-users/nu-1/creds", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("-----BEGIN NATS USER JWT-----\ncreds\n-----END NATS USER JWT-----"))
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	creds, err := client.GetNatsUserCreds(ctx, "nu-1")
	require.NoError(t, err)
	assert.Contains(t, string(creds), "BEGIN NATS USER JWT")
}

func TestClient_do_get_token_error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "", assert.AnError })
	ctx := context.Background()

	_, err := client.ListSystems(ctx, "team-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get token")
}

func TestClient_do_http_error_status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	_, err := client.GetAccount(ctx, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestClient_DeleteAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	err := client.DeleteAccount(ctx, "acc-1")
	require.NoError(t, err)
}

func TestClient_ListStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1/jetstream/streams", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{
			Items: []JetStreamResourceItem{{Name: "ORDERS"}, {Name: "EVENTS"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	resp, err := client.ListStreams(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "ORDERS", resp.Items[0].Name)
	assert.Equal(t, "EVENTS", resp.Items[1].Name)
}

func TestClient_ListKVBuckets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1/jetstream/kv-buckets", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{
			Items: []JetStreamResourceItem{{Name: "config"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	resp, err := client.ListKVBuckets(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "config", resp.Items[0].Name)
}

func TestClient_ListObjectBuckets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/accounts/acc-1/jetstream/object-buckets", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{
			Items: []JetStreamResourceItem{{Name: "assets"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	resp, err := client.ListObjectBuckets(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "assets", resp.Items[0].Name)
}

func TestClient_DeleteNatsUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/core/beta/nats-users/nu-1", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, func(context.Context) (string, error) { return "t", nil })
	ctx := context.Background()

	err := client.DeleteNatsUser(ctx, "nu-1")
	require.NoError(t, err)
}
