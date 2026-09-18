package nats

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	natsclient "github.com/nats-io/nats.go"
)

const accountzRequestSubject = "$SYS.REQ.SERVER.%s.ACCOUNTZ"

const (
	accountzSupportedSinceMajor = 2
	accountzSupportedSinceMinor = 2
)

type connectedNatsServer struct {
	ID      string
	Version string
}

type accountzRequest struct {
	Account string `json:"account"`
}

type accountzResponse struct {
	Server *accountzServer `json:"server"`
	Data   *accountzData   `json:"data"`
	Error  *accountzError  `json:"error"`
}

type accountzServer struct {
	ID string `json:"id"`
}

type accountzData struct {
	ServerID string           `json:"server_id"`
	Account  *accountzAccount `json:"account_detail"`
}

type accountzAccount struct {
	AccountName string           `json:"account_name"`
	Complete    *bool            `json:"complete"`
	JWT         string           `json:"jwt"`
	Imports     []accountzImport `json:"imports"`
}

type accountzImport struct {
	Account      string `json:"account"`
	Subject      string `json:"subject"`
	LocalSubject string `json:"local_subject"`
	Type         string `json:"type"`
	Invalid      bool   `json:"invalid"`
}

type accountzError struct {
	Code        int    `json:"code"`
	ErrCode     int    `json:"err_code"`
	Description string `json:"description"`
}

func (e accountzError) String() string {
	if e.Description != "" {
		return e.Description
	}
	if e.ErrCode != 0 {
		return fmt.Sprintf("error code %d", e.ErrCode)
	}
	if e.Code != 0 {
		return fmt.Sprintf("error code %d", e.Code)
	}
	return "unknown error"
}

func (n *connection) LookupAccountState(accountID string) (domain.NatsAccountState, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return unknownAccountState("NATS connection is not established or lost")
	}
	if accountID == "" {
		return unknownAccountState("account ID is required")
	}

	payload, err := json.Marshal(accountzRequest{Account: accountID})
	if err != nil {
		return unknownAccountState(fmt.Sprintf("failed to encode ACCOUNTZ request: %v", err))
	}

	connectedServerBefore := connectedNatsServerInfo(n.conn)
	if supported, known := accountzSupport(connectedServerBefore.Version); known && !supported {
		return unknownAccountState(formatUnsupportedAccountzServerError(connectedServerBefore))
	}
	if connectedServerBefore.ID == "" {
		return unknownAccountState("connected NATS server identity is unavailable; account state is Unknown")
	}

	msg, err := n.conn.Request(fmt.Sprintf(accountzRequestSubject, connectedServerBefore.ID), payload, natsMaxTimeout)
	if err != nil {
		connectedServerAfter := connectedNatsServerInfo(n.conn)
		return unknownAccountState(formatAccountzRequestError(connectedServerBefore, connectedServerAfter, err))
	}

	state, err := parseAccountzResponse(msg.Data, accountID)
	if err != nil {
		return state, err
	}
	if state.ServerID != connectedServerBefore.ID {
		return unknownAccountState(fmt.Sprintf("ACCOUNTZ response came from server %q, expected connected server %q", state.ServerID, connectedServerBefore.ID))
	}

	return state, nil
}

func parseAccountzResponse(payload []byte, expectedAccountID string) (domain.NatsAccountState, error) {
	var response accountzResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return unknownAccountState(fmt.Sprintf("failed to decode ACCOUNTZ response: %v", err))
	}
	if response.Error != nil {
		return unknownAccountState(fmt.Sprintf("ACCOUNTZ returned an error: %s", response.Error.String()))
	}
	if response.Data == nil {
		return unknownAccountState("ACCOUNTZ response did not contain data")
	}
	if response.Server == nil || response.Server.ID == "" {
		if response.Data.ServerID == "" {
			return unknownAccountState("ACCOUNTZ response did not contain server identity")
		}
	} else if response.Data.ServerID != "" && response.Server.ID != response.Data.ServerID {
		return unknownAccountState(fmt.Sprintf("ACCOUNTZ response contained conflicting server identities %q and %q", response.Server.ID, response.Data.ServerID))
	}
	if response.Data.Account == nil {
		return unknownAccountState("ACCOUNTZ response did not contain account data")
	}

	account := response.Data.Account
	if account.AccountName == "" {
		return unknownAccountState("ACCOUNTZ response did not contain an account ID")
	}
	if account.AccountName != expectedAccountID {
		return unknownAccountState(fmt.Sprintf("ACCOUNTZ response contained account %q, expected %q", account.AccountName, expectedAccountID))
	}
	if account.Complete == nil {
		return unknownAccountState("ACCOUNTZ response did not contain Complete state")
	}
	if account.JWT == "" {
		return unknownAccountState("ACCOUNTZ response did not contain an Account JWT")
	}
	claims, err := jwt.DecodeAccountClaims(account.JWT)
	if err != nil {
		return unknownAccountState(fmt.Sprintf("failed to decode Account JWT: %v", err))
	}
	if claims.Subject != expectedAccountID {
		return unknownAccountState(fmt.Sprintf("ACCOUNTZ response contained JWT for account %q, expected %q", claims.Subject, expectedAccountID))
	}

	claimsHash, err := domain.HashNatsAccountJWTClaims(account.JWT)
	if err != nil {
		return unknownAccountState(err.Error())
	}

	imports := make([]domain.NatsAccountImport, 0, len(account.Imports))
	for _, imp := range account.Imports {
		imports = append(imports, domain.NatsAccountImport{
			AccountID:    imp.Account,
			Subject:      imp.Subject,
			LocalSubject: imp.LocalSubject,
			Type:         imp.Type,
			Invalid:      imp.Invalid,
		})
	}

	serverID := response.Data.ServerID
	if response.Server != nil && response.Server.ID != "" {
		serverID = response.Server.ID
	}

	status := domain.NatsAccountStateComplete
	if !*account.Complete {
		status = domain.NatsAccountStateIncomplete
	}
	return domain.NatsAccountState{
		Status:     status,
		ServerID:   serverID,
		AccountID:  account.AccountName,
		ClaimsHash: claimsHash,
		Imports:    imports,
	}, nil
}

func unknownAccountState(message string) (domain.NatsAccountState, error) {
	return domain.NatsAccountState{Status: domain.NatsAccountStateUnknown}, fmt.Errorf("%s", message)
}

func connectedNatsServerInfo(conn *natsclient.Conn) connectedNatsServer {
	return connectedNatsServer{
		ID:      conn.ConnectedServerId(),
		Version: conn.ConnectedServerVersion(),
	}
}

func formatAccountzRequestError(before, after connectedNatsServer, err error) string {
	if before == after && before.ID != "" {
		if before.Version != "" {
			return fmt.Sprintf("failed to lookup Account state on connected NATS server %q version %q: %v", before.ID, before.Version, err)
		}
		return fmt.Sprintf("failed to lookup Account state on connected NATS server %q: %v", before.ID, err)
	}

	return fmt.Sprintf("failed to lookup Account state: %v", err)
}

func formatUnsupportedAccountzServerError(server connectedNatsServer) string {
	return fmt.Sprintf("ACCOUNTZ is unavailable on connected NATS server %q version %q; account state is Unknown", server.ID, server.Version)
}

func accountzSupport(version string) (supported, known bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false, false
	}

	return major > accountzSupportedSinceMajor ||
		(major == accountzSupportedSinceMajor && minor >= accountzSupportedSinceMinor), true
}
