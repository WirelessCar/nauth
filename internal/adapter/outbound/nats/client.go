package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	natsMaxTimeout = 3 * time.Second
)

type ServerAPIClaimUpdateResponse struct {
	Data  *ClaimUpdateStatus `json:"data,omitempty"`
	Error *ClaimUpdateError  `json:"error,omitempty"`
}

type ClaimUpdateStatus struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type ClaimUpdateError struct {
	Code        int    `json:"code"`
	Description string `json:"description,omitempty"`
}

type SysClient struct{}

func NewSysClient() *SysClient {
	return &SysClient{}
}

func (n *SysClient) Connect(natsURL string, userCreds domain.NatsUserCreds) (outbound.NatsSysConnection, error) {
	return connect(natsURL, userCreds)
}

type AccountClient struct{}

func NewAccountClient() *AccountClient {
	return &AccountClient{}
}

func (c AccountClient) Connect(natsURL string, userCreds domain.NatsUserCreds) (outbound.NatsAccountConnection, error) {
	return connect(natsURL, userCreds)
}

func connect(natsURL string, userCreds domain.NatsUserCreds) (*connection, error) {
	if natsURL == "" {
		return nil, fmt.Errorf("NATS URL is required")
	}

	if err := userCreds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NATS user credentials: %w", err)
	}

	c := &connection{
		natsURL:   natsURL,
		userCreds: userCreds,
	}
	if err := c.EnsureConnected(); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}

	return c, nil
}

type connection struct {
	natsURL   string
	userCreds domain.NatsUserCreds
	conn      *nats.Conn
}

func (n *connection) EnsureConnected() error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.connect()
}

func (n *connection) Disconnect() {
	if n.conn == nil {
		return
	}

	if n.conn.IsConnected() {
		if err := n.conn.Drain(); err != nil {
			n.conn.Close()
		}
	} else {
		n.conn.Close()
	}
}

func (n *connection) VerifySystemAccountAccess() error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	_, err := n.conn.Request("$SYS.REQ.SERVER.PING", nil, natsMaxTimeout)
	if err != nil {
		return fmt.Errorf("failed system account ping: %w", err)
	}

	return nil
}

func (n *connection) ListAccountStreams() ([]string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return nil, fmt.Errorf("NATS connection is not established or lost")
	}

	ctx, cancel := context.WithTimeout(context.Background(), natsMaxTimeout)
	defer cancel()

	js, err := jetstream.New(n.conn)
	if err != nil {
		return nil, fmt.Errorf("create JetStream client: %w", err)
	}

	if _, err := js.AccountInfo(ctx); err != nil {
		if errors.Is(err, jetstream.ErrJetStreamNotEnabled) || errors.Is(err, jetstream.ErrJetStreamNotEnabledForAccount) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("lookup JetStream account state: %w", err)
	}

	streamNames := js.StreamNames(ctx)
	names := make([]string, 0)
	for name := range streamNames.Name() {
		names = append(names, name)
	}
	if err := streamNames.Err(); err != nil {
		return nil, fmt.Errorf("list JetStream streams: %w", err)
	}

	sort.Strings(names)
	return names, nil
}

func (n *connection) LookupAccountJWT(accountID string) (string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return "", fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, natsMaxTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to lookup account JWT: %w", err)
	}

	return string(msg.Data), nil
}

func (n *connection) UploadAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.UPDATE", jwt)
}

func (n *connection) DeleteAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.DELETE", jwt)
}

func (n *connection) updateClaimsJWT(subject string, jwt string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(subject, []byte(jwt), natsMaxTimeout)
	if err != nil {
		return fmt.Errorf("unable to post jwt request: %w", err)
	}

	res := &ServerAPIClaimUpdateResponse{}

	err = json.Unmarshal(msg.Data, res)
	if err != nil {
		return fmt.Errorf("failed to unmarshal nats response from jwt request: %w", err)
	}

	if res.Error != nil {
		return fmt.Errorf("jwt request error <code:%d> <description:%s>", res.Error.Code, res.Error.Description)
	}

	if res.Data == nil {
		// This should not happen, unless NATS changed their API.
		return fmt.Errorf("jwt request returned no status nor error")
	}

	if res.Data.Code != 200 {
		return fmt.Errorf("jwt request failed <code:%d> <message:%s>", res.Data.Code, res.Data.Message)
	}

	return nil
}

func (n *connection) connect() error {
	var err error

	n.conn, err = nats.Connect(
		n.natsURL,
		nats.UserCredentialBytes(n.userCreds.Creds),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(7),
		nats.ReconnectWait(time.Second),
	)
	if err != nil {
		return fmt.Errorf("unable to connect to NATS cluster: %w", err)
	}

	return err
}

// Compile-time assertion that implementations fulfills ports
var _ outbound.NatsSysClient = (*SysClient)(nil)
var _ outbound.NatsAccountClient = (*AccountClient)(nil)
var _ outbound.NatsSysConnection = (*connection)(nil)
var _ outbound.NatsAccountConnection = (*connection)(nil)
