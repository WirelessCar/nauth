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
	"time"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testToken             = "token"
	testAccountPath       = "/api/core/beta/accounts/acc-1"
	testStreamsPath       = "/api/core/beta/accounts/acc-1/jetstream/streams"
	testKVBucketsPath     = "/api/core/beta/accounts/acc-1/jetstream/kv-buckets"
	testObjectBucketsPath = "/api/core/beta/accounts/acc-1/jetstream/object-buckets"
)

func TestProvider_CreateOrUpdateAccount_create_sets_RequeueAfter(t *testing.T) {
	reconcileInterval := 2 * time.Minute
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec: synadiav1alpha1.SystemSpec{
			APIEndpoint:       "https://api.example.com",
			ReconcileInterval: &metav1.Duration{Duration: reconcileInterval},
		},
		Status: synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	var createReq CreateAccountRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/core/beta/systems/sys-1/accounts" && r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&createReq)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(AccountDTO{
				ID: "acc-new", AccountPublicKey: "ACxxx",
				JwtSettings: &JwtSettingsDTO{
					Limits: &JwtSettingsLimitsDTO{Subs: int64Ptr(1), Conn: int64Ptr(1)},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
		Spec:       nauthv1alpha1.AccountSpec{DisplayName: "My Account"},
	}
	ctx := context.Background()

	result, err := provider.CreateAccount(ctx, acc)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acc-new", result.AccountID)
	require.NotNil(t, result.RequeueAfter)
	assert.Equal(t, reconcileInterval, *result.RequeueAfter)
	assert.Equal(t, "My Account", createReq.Name)
}

func TestProvider_CreateOrUpdateAccount_observe_returns_state_and_RequeueAfter(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAccountPath && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(AccountDTO{
				ID: "acc-1", AccountPublicKey: "ACobs",
				JwtSettings: &JwtSettingsDTO{
					Limits: &JwtSettingsLimitsDTO{Subs: int64Ptr(5), Conn: int64Ptr(1)},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels: map[string]string{
				k8s.LabelAccountID:        "acc-1",
				k8s.LabelManagementPolicy: k8s.LabelManagementPolicyObserveValue,
			},
		},
	}
	ctx := context.Background()

	result, err := provider.ImportAccount(ctx, acc)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "acc-1", result.AccountID)
	assert.Equal(t, "ACobs", result.AccountSignedBy)
	require.NotNil(t, result.RequeueAfter)
	assert.Equal(t, 5*time.Minute, *result.RequeueAfter) // default
}

func TestProvider_CreateOrUpdateAccount_update_detects_nkey_rotation(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	// Return limits matching default so provider does not PATCH (we only care about nkey rotation).
	defaultLimits := NatsLimitsFromAccount(&nauthv1alpha1.AccountSpec{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAccountPath && (r.Method == http.MethodGet || r.Method == http.MethodPatch) {
			w.Header().Set("Content-Type", "application/json")
			// API returns a different public key than what we have stored (rotation)
			_ = json.NewEncoder(w).Encode(AccountDTO{
				ID: "acc-1", AccountPublicKey: "ACrotated",
				JwtSettings: &JwtSettingsDTO{
					Limits: &JwtSettingsLimitsDTO{
						Subs: defaultLimits.Subs, Payload: defaultLimits.Payload, Data: defaultLimits.Data,
						Conn: defaultLimits.Conn, Leaf: defaultLimits.Leaf,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels: map[string]string{
				k8s.LabelAccountID:       "acc-1",
				k8s.LabelAccountSignedBy: "ACold", // stored key differs from API
			},
		},
		Spec: nauthv1alpha1.AccountSpec{},
	}
	ctx := context.Background()

	result, err := provider.UpdateAccount(ctx, acc)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.AccountNkeyRotated)
	assert.Equal(t, "ACrotated", result.AccountSignedBy)
}

func TestProvider_CreateOrUpdateAccount_no_systemId_returns_error(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{}, // no SystemID
	}
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient("http://localhost", func(context.Context) (string, error) { return "t", nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	acc := &nauthv1alpha1.Account{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}}
	result, err := provider.CreateAccount(context.Background(), acc)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no systemId in status")
}

func TestProvider_CreateOrUpdateAccount_uses_TieredLimit_when_present(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	var createReq CreateAccountRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/core/beta/systems/sys-1/accounts" && r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&createReq)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(AccountDTO{ID: "acc-1", AccountPublicKey: "ACx"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))

	tieredLimit := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "default"},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1: &synadiav1alpha1.TieredLimitTier{
				DiskStorage: int64Ptr(1 << 30),
				Streams:     int64Ptr(10),
				Consumer:    int64Ptr(5),
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tieredLimit).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
		Spec:       nauthv1alpha1.AccountSpec{},
	}
	ctx := context.Background()

	result, err := provider.CreateAccount(ctx, acc)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, createReq.JwtSettings)
	require.NotNil(t, createReq.JwtSettings.Limits)
	require.NotNil(t, createReq.JwtSettings.Limits.TieredLimits)
	require.NotNil(t, createReq.JwtSettings.Limits.TieredLimits.R1)
	assert.Equal(t, int64(1<<30), *createReq.JwtSettings.Limits.TieredLimits.R1.DiskStorage)
	assert.Equal(t, int64(10), *createReq.JwtSettings.Limits.TieredLimits.R1.Streams)
	assert.Equal(t, int64(5), *createReq.JwtSettings.Limits.TieredLimits.R1.Consumer)
}

func TestProvider_CreateOrUpdateUser_sets_RequeueAfter(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	var capturedCreateReq CreateNatsUserRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/core/beta/accounts/acc-1/account-sk-groups" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ListSigningKeyGroupsResponse{
				Items: []SigningKeyGroupItem{{ID: "skg-default", Name: "Default"}},
			})
		case r.URL.Path == "/api/core/beta/accounts/acc-1/nats-users" && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&capturedCreateReq)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(NatsUserDTO{ID: "nu-1", UserPublicKey: "Uxxx"})
		case r.URL.Path == "/api/core/beta/nats-users/nu-1/creds" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("credentials"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))

	account := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels:    map[string]string{k8s.LabelAccountID: "acc-1"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(account).Build()
	secretApplier := &mockSecretApplier{}

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, secretApplier)

	user := &nauthv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1", Namespace: "default"},
		Spec:       nauthv1alpha1.UserSpec{AccountName: "myacc"},
	}
	ctx := context.Background()

	result, err := provider.CreateOrUpdateUser(ctx, user)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "nu-1", result.UserID)
	require.NotNil(t, result.RequeueAfter)
	assert.Equal(t, "skg-default", capturedCreateReq.SKGroupID)
}

func TestProvider_DeleteAccount_blocks_when_streams_exist(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testStreamsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{
				Items: []JetStreamResourceItem{{Name: "ORDERS"}},
			})
		case testKVBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		case testObjectBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, &mockSecretApplier{})

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels:    map[string]string{k8s.LabelAccountID: "acc-1"},
		},
	}

	err := provider.DeleteAccount(context.Background(), acc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JetStream resources still exist")
	assert.Contains(t, err.Error(), "stream/ORDERS")
}

func TestProvider_DeleteAccount_blocks_when_kv_buckets_exist(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testStreamsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		case testKVBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{
				Items: []JetStreamResourceItem{{Name: "config"}},
			})
		case testObjectBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, &mockSecretApplier{})

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels:    map[string]string{k8s.LabelAccountID: "acc-1"},
		},
	}

	err := provider.DeleteAccount(context.Background(), acc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JetStream resources still exist")
	assert.Contains(t, err.Error(), "kv/config")
}

func TestProvider_DeleteAccount_succeeds_when_no_jetstream_resources(t *testing.T) {
	sys := &synadiav1alpha1.System{
		ObjectMeta: metav1.ObjectMeta{Name: "ngs", Namespace: "default"},
		Spec:       synadiav1alpha1.SystemSpec{APIEndpoint: "https://api.example.com"},
		Status:     synadiav1alpha1.SystemStatus{SystemID: "sys-1"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testStreamsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		case testKVBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		case testObjectBucketsPath:
			_ = json.NewEncoder(w).Encode(ListJetStreamResourcesResponse{})
		case testAccountPath:
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sys.Spec.APIEndpoint = server.URL
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	client := NewClient(server.URL, func(context.Context) (string, error) { return testToken, nil })
	provider := NewProvider(client, sys, k8sClient, &mockSecretApplier{})

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myacc",
			Namespace: "default",
			Labels:    map[string]string{k8s.LabelAccountID: "acc-1"},
		},
	}

	err := provider.DeleteAccount(context.Background(), acc)
	require.NoError(t, err)
}

func TestFactory_CreateProvider_rejects_non_System_config(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	f := NewFactory(k8sClient, &mockSynadiaSecretClient{
		mockTokenReader:   &mockTokenReader{},
		mockSecretApplier: &mockSecretApplier{},
	})

	provider, err := f.CreateProvider(context.Background(), "not-a-system")
	require.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "synadia factory expected *System")
}

// --- TieredLimit selection (one per account, status.selectedForAccount) ---

func TestSelectedForAccountEquals(t *testing.T) {
	tests := []struct {
		name        string
		ref         *synadiav1alpha1.AccountRef
		accountNs   string
		accountName string
		want        bool
	}{
		{"nil ref", nil, "default", "myacc", false},
		{"name mismatch", &synadiav1alpha1.AccountRef{Name: "other", Namespace: "default"}, "default", "myacc", false},
		{"match with namespace", &synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"}, "default", "myacc", true},
		{"match empty namespace", &synadiav1alpha1.AccountRef{Name: "myacc"}, "default", "myacc", true},
		{"namespace mismatch", &synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "other"}, "default", "myacc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectedForAccountEquals(tt.ref, tt.accountNs, tt.accountName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProvider_getTieredLimitForAccount_no_candidates_returns_nil(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	provider := NewProvider(nil, &synadiav1alpha1.System{}, k8sClient, nil)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
	}
	ctx := context.Background()

	tl := provider.getTieredLimitForAccount(ctx, acc)
	assert.Nil(t, tl)
}

func TestProvider_getTieredLimitForAccount_single_candidate_returns_it_and_marks_it(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	tieredLimit := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "default"},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(10)},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tieredLimit).Build()
	provider := NewProvider(nil, &synadiav1alpha1.System{}, k8sClient, nil)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
	}
	ctx := context.Background()

	tl := provider.getTieredLimitForAccount(ctx, acc)
	require.NotNil(t, tl)
	assert.Equal(t, "limits", tl.GetName())
	assert.Equal(t, "default", tl.GetNamespace())
	// Controller marks the selected TieredLimit (set in memory before patch).
	require.NotNil(t, tl.Status.SelectedForAccount)
	assert.Equal(t, "myacc", tl.Status.SelectedForAccount.Name)
	assert.Equal(t, "default", tl.Status.SelectedForAccount.Namespace)
}

func TestProvider_getTieredLimitForAccount_multiple_candidates_none_marked_picks_deterministically_and_marks_one(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	older := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	newer := metav1.NewTime(time.Now())
	tlA := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits-a", Namespace: "default", CreationTimestamp: newer},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(1)},
		},
	}
	tlB := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits-b", Namespace: "default", CreationTimestamp: older},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(2)},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tlA, tlB).Build()
	provider := NewProvider(nil, &synadiav1alpha1.System{}, k8sClient, nil)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
	}
	ctx := context.Background()

	tl := provider.getTieredLimitForAccount(ctx, acc)
	require.NotNil(t, tl)
	// Oldest by CreationTimestamp wins, so limits-b.
	assert.Equal(t, "limits-b", tl.GetName())
	// Controller marks the selected TieredLimit (set in memory before patch).
	require.NotNil(t, tl.Status.SelectedForAccount)
	assert.Equal(t, "myacc", tl.Status.SelectedForAccount.Name)
}

func TestProvider_getTieredLimitForAccount_multiple_candidates_one_already_marked_returns_marked_one(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	tlA := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits-a", Namespace: "default"},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(1)},
		},
	}
	tlB := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits-b", Namespace: "default"},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(2)},
		},
		Status: synadiav1alpha1.TieredLimitStatus{
			SelectedForAccount: &synadiav1alpha1.AccountRef{Name: "myacc", Namespace: "default"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tlA, tlB).Build()
	provider := NewProvider(nil, &synadiav1alpha1.System{}, k8sClient, nil)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
	}
	ctx := context.Background()

	tl := provider.getTieredLimitForAccount(ctx, acc)
	require.NotNil(t, tl)
	assert.Equal(t, "limits-b", tl.GetName(), "marked TieredLimit should be returned")
}

func TestProvider_getTieredLimitForAccount_account_ref_namespace_defaults_to_tiered_limit_namespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, nauthv1alpha1.AddToScheme(scheme))
	require.NoError(t, synadiav1alpha1.AddToScheme(scheme))
	tieredLimit := &synadiav1alpha1.TieredLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "default"},
		Spec: synadiav1alpha1.TieredLimitSpec{
			AccountRef: synadiav1alpha1.AccountRef{Name: "myacc"}, // no Namespace => defaults to TL namespace
			R1:         &synadiav1alpha1.TieredLimitTier{Streams: int64Ptr(5)},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tieredLimit).Build()
	provider := NewProvider(nil, &synadiav1alpha1.System{}, k8sClient, nil)

	acc := &nauthv1alpha1.Account{
		ObjectMeta: metav1.ObjectMeta{Name: "myacc", Namespace: "default"},
	}
	ctx := context.Background()

	tl := provider.getTieredLimitForAccount(ctx, acc)
	require.NotNil(t, tl)
	assert.Equal(t, "limits", tl.GetName())
}

type mockSecretApplier struct{}

func (m *mockSecretApplier) Apply(ctx context.Context, owner *secret.Owner, meta metav1.ObjectMeta, valueMap map[string]string) error {
	return nil
}

func (m *mockSecretApplier) Delete(ctx context.Context, namespace string, name string) error {
	return nil
}

var _ SecretApplier = (*mockSecretApplier)(nil)

type mockTokenReader struct {
	token string
}

func (m *mockTokenReader) Get(ctx context.Context, namespace, name string) (map[string]string, error) {
	if m != nil && m.token != "" {
		return map[string]string{"token": m.token}, nil
	}
	return map[string]string{"token": "test-token"}, nil
}

// mockSynadiaSecretClient implements SecretClient (TokenReader + SecretApplier) for factory tests.
type mockSynadiaSecretClient struct {
	*mockTokenReader
	*mockSecretApplier
}

var _ SecretClient = (*mockSynadiaSecretClient)(nil)

var _ cluster.Provider = (*Provider)(nil)
