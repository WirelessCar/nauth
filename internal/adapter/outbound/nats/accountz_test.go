package nats

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParseAccountzResponse_ShouldReturnCompleteAccountState(t *testing.T) {
	operator := newOperator(t)
	account := newAccount(t, operator, nil)
	complete := true
	payload := marshalAccountzResponse(t, accountzResponse{
		Server: &accountzServer{ID: "server-a"},
		Data: &accountzData{
			ServerID: "server-a",
			Account: &accountzAccount{
				AccountName: account.key.PublicKey,
				Complete:    &complete,
				JWT:         account.jwt,
			},
		},
	})

	state, err := parseAccountzResponse(payload, account.key.PublicKey)

	require.NoError(t, err)
	require.Equal(t, domain.NatsAccountStateComplete, state.Status)
	require.Equal(t, "server-a", state.ServerID)
	require.Equal(t, account.key.PublicKey, state.AccountID)
	require.Empty(t, state.Imports)
	expectedClaimsHash, err := domain.HashNatsAccountJWTClaims(account.jwt)
	require.NoError(t, err)
	require.Equal(t, expectedClaimsHash, state.ClaimsHash)
}

func TestParseAccountzResponse_ShouldSupportNATS220Payload(t *testing.T) {
	operator := newOperator(t)
	account := newAccount(t, operator, nil)

	// Keep this payload independent from the adapter response structs so the
	// test protects the NATS v2.2.0 ACCOUNTZ JSON contract.
	payload := []byte(fmt.Sprintf(
		`{"server":{"id":"server-a"},"data":{"server_id":"server-a","now":"2026-09-21T12:00:00Z","account_detail":{"account_name":%q,"complete":true,"jwt":%q,"imports":[]}}}`,
		account.key.PublicKey,
		account.jwt,
	))

	state, err := parseAccountzResponse(payload, account.key.PublicKey)

	require.NoError(t, err)
	require.Equal(t, domain.NatsAccountStateComplete, state.Status)
	require.Equal(t, "server-a", state.ServerID)
	require.Equal(t, account.key.PublicKey, state.AccountID)
	expectedClaimsHash, err := domain.HashNatsAccountJWTClaims(account.jwt)
	require.NoError(t, err)
	require.Equal(t, expectedClaimsHash, state.ClaimsHash)
}

func TestParseAccountzResponse_ShouldReturnIncompleteStateAndInvalidImports(t *testing.T) {
	operator := newOperator(t)
	account := newAccount(t, operator, nil)
	incomplete := false
	payload := marshalAccountzResponse(t, accountzResponse{
		Data: &accountzData{
			ServerID: "server-a",
			Account: &accountzAccount{
				AccountName: account.key.PublicKey,
				Complete:    &incomplete,
				JWT:         account.jwt,
				Imports: []accountzImport{
					{
						Account:      "EXPORT_ACCOUNT",
						Subject:      "foo.>",
						LocalSubject: "bar.>",
						Type:         "stream",
						Invalid:      true,
					},
				},
			},
		},
	})

	state, err := parseAccountzResponse(payload, account.key.PublicKey)

	require.NoError(t, err)
	require.Equal(t, domain.NatsAccountStateIncomplete, state.Status)
	require.Equal(t, "server-a", state.ServerID)
	require.Equal(t, []domain.NatsAccountImport{
		{
			AccountID:    "EXPORT_ACCOUNT",
			Subject:      "foo.>",
			LocalSubject: "bar.>",
			Type:         "stream",
			Invalid:      true,
		},
	}, state.Imports)
}

func TestParseAccountzResponse_ShouldReturnUnknownWhenValidationDataIsInsufficient(t *testing.T) {
	operator := newOperator(t)
	account := newAccount(t, operator, nil)
	otherAccount := newAccount(t, operator, nil)
	complete := true

	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "malformed_response",
			payload: []byte("{"),
		},
		{
			name:    "missing_data",
			payload: marshalAccountzResponse(t, accountzResponse{}),
		},
		{
			name: "missing_account",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{},
			}),
		},
		{
			name: "missing_complete_state",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: account.key.PublicKey,
					JWT:         account.jwt,
				}},
			}),
		},
		{
			name: "missing_account_jwt",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: account.key.PublicKey,
					Complete:    &complete,
				}},
			}),
		},
		{
			name: "jwt_subject_mismatch",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: account.key.PublicKey,
					Complete:    &complete,
					JWT:         otherAccount.jwt,
				}},
			}),
		},
		{
			name: "wrong_account",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: "OTHER_ACCOUNT",
					Complete:    &complete,
					JWT:         account.jwt,
				}},
			}),
		},
		{
			name: "missing_server_identity",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: account.key.PublicKey,
					Complete:    &complete,
					JWT:         account.jwt,
				}},
			}),
		},
		{
			name: "conflicting_server_identity",
			payload: marshalAccountzResponse(t, accountzResponse{
				Server: &accountzServer{ID: "server-a"},
				Data: &accountzData{
					ServerID: "server-b",
					Account: &accountzAccount{
						AccountName: account.key.PublicKey,
						Complete:    &complete,
						JWT:         account.jwt,
					},
				},
			}),
		},
		{
			name: "invalid_account_jwt",
			payload: marshalAccountzResponse(t, accountzResponse{
				Data: &accountzData{Account: &accountzAccount{
					AccountName: account.key.PublicKey,
					Complete:    &complete,
					JWT:         "not-a-jwt",
				}},
			}),
		},
		{
			name: "server_error",
			payload: marshalAccountzResponse(t, accountzResponse{
				Error: &accountzError{Code: 400, Description: "permission denied"},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := parseAccountzResponse(tt.payload, account.key.PublicKey)

			require.Error(t, err)
			require.Equal(t, domain.NatsAccountStateUnknown, state.Status)
		})
	}
}

func TestConnection_LookupAccountState_ShouldReturnUnknownWhenDisconnected(t *testing.T) {
	state, err := (&connection{}).LookupAccountState("ACCOUNT")

	require.Error(t, err)
	require.Equal(t, domain.NatsAccountStateUnknown, state.Status)
}

func TestFormatAccountzRequestError_ShouldKeepSupportedServerFailureUnknown(t *testing.T) {
	err := fmt.Errorf("nats: timeout")

	message := formatAccountzRequestError(
		connectedNatsServer{ID: "server-a", Version: "2.14.0"},
		connectedNatsServer{ID: "server-a", Version: "2.14.0"},
		err,
	)

	require.Equal(t, `failed to lookup Account state on connected NATS server "server-a" version "2.14.0": nats: timeout`, message)
}

func TestFormatAccountzRequestError_ShouldNotClassifyWhenConnectionChanged(t *testing.T) {
	err := fmt.Errorf("nats: timeout")

	message := formatAccountzRequestError(
		connectedNatsServer{ID: "server-a", Version: "2.0.0"},
		connectedNatsServer{ID: "server-b", Version: "2.14.0"},
		err,
	)

	require.Equal(t, `failed to lookup Account state: nats: timeout`, message)
}

func TestFormatUnsupportedAccountzServerError_ShouldIdentifyServerVersion(t *testing.T) {
	message := formatUnsupportedAccountzServerError(connectedNatsServer{ID: "server-a", Version: "2.0.0"})

	require.Equal(t, `ACCOUNTZ is unavailable on connected NATS server "server-a" version "2.0.0"; account state is Unknown`, message)
}

func TestAccountzSupport(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		supported bool
		known     bool
	}{
		{name: "nats_2_0", version: "2.0.0", supported: false, known: true},
		{name: "nats_2_1", version: "v2.1.99", supported: false, known: true},
		{name: "nats_2_2", version: "2.2.0", supported: true, known: true},
		{name: "nats_2_14", version: "2.14.0", supported: true, known: true},
		{name: "unknown_version", version: "unknown", supported: false, known: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported, known := accountzSupport(tt.version)

			require.Equal(t, tt.supported, supported)
			require.Equal(t, tt.known, known)
		})
	}
}

func marshalAccountzResponse(t *testing.T, response accountzResponse) []byte {
	t.Helper()
	payload, err := json.Marshal(response)
	require.NoError(t, err)
	return payload
}
