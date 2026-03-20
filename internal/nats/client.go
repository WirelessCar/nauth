package nats

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nats.go"
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

type Client struct {
}

func NewClient() *Client {
	return &Client{}
}

func (n *Client) Connect(natsURL string, userCreds domain.NatsUserCreds) (outbound.NatsConnection, error) {
	if natsURL == "" {
		return nil, fmt.Errorf("NATS URL is required")
	}

	if err := userCreds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NATS user credentials: %w", err)
	}

	c := &Connection{
		natsURL:   natsURL,
		userCreds: userCreds,
	}
	if err := c.EnsureConnected(); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}

	return c, nil
}

type Connection struct {
	natsURL   string
	userCreds domain.NatsUserCreds
	conn      *nats.Conn
}

func (n *Connection) EnsureConnected() error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.connect()
}

func (n *Connection) Disconnect() {
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

func (n *Connection) LookupAccountJWT(accountID string) (string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return "", fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, natsMaxTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to lookup account JWT: %w", err)
	}

	return string(msg.Data), nil
}

func (n *Connection) UploadAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.UPDATE", jwt)
}

func (n *Connection) DeleteAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.DELETE", jwt)
}

func (n *Connection) updateClaimsJWT(subject string, jwt string) error {
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

func (n *Connection) connect() error {
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
var _ outbound.NatsClient = (*Client)(nil)
var _ outbound.NatsConnection = (*Connection)(nil)
