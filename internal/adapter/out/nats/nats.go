package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/internal/core/domain"
	"github.com/WirelessCar/nauth/internal/core/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
)

const (
	natsMaxTimeout = 3 * time.Second
)

type NatsResponse struct {
	Data NatsData `json:"data"`
}

type NatsListResponse struct {
	Data []string `json:"data"`
}

type NatsData struct {
	Account string `json:"account,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type natsClient struct {
	natsURL      string
	secretStorer ports.SecretStorer
	conn         *nats.Conn
}

func NewNATSClient(natsURL string, secretStorer ports.SecretStorer) ports.NATSClient {
	return &natsClient{
		natsURL:      natsURL,
		secretStorer: secretStorer,
	}
}

func (n *natsClient) EnsureConnected(namespace string) error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.connect(namespace)
}

func (n *natsClient) Disconnect() {
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

func (n *natsClient) LookupAccountJWT(accountID string) (string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return "", fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, natsMaxTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to lookup account JWT: %w", err)
	}

	return string(msg.Data), nil
}

func (n *natsClient) ListAccountJWTs() ([]string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return []string{}, fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request("$SYS.REQ.CLAIMS.LIST", nil, natsMaxTimeout)
	if err != nil {
		return []string{}, fmt.Errorf("failed to list account JWTs: %w", err)
	}

	res := &NatsListResponse{}
	if err := json.Unmarshal(msg.Data, res); err != nil {
		return []string{}, fmt.Errorf("failed to unmarshal account JWT response: %w", err)
	}

	return res.Data, nil
}

func (n *natsClient) UploadAccountJWT(jwt string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request("$SYS.REQ.CLAIMS.UPDATE", []byte(jwt), natsMaxTimeout)
	if err != nil {
		return fmt.Errorf("unable to post jwt update request: %w", err)
	}

	res := &NatsResponse{}

	err = json.Unmarshal(msg.Data, res)
	if err != nil {
		return fmt.Errorf("failed to unmarshal nats resonse from jwt update request: %w", err)
	}

	if res.Data.Code != 200 {
		return fmt.Errorf("jwt update failed <code:%d> <message:%s>", res.Data.Code, res.Data.Message)
	}

	return nil
}

func (n *natsClient) DeleteAccountJWT(jwt string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request("$SYS.REQ.CLAIMS.DELETE", []byte(jwt), natsMaxTimeout)
	if err != nil {
		return fmt.Errorf("unable to post jwt update request: %w", err)
	}

	res := &NatsResponse{}

	err = json.Unmarshal(msg.Data, res)
	if err != nil {
		return fmt.Errorf("failed to unmarshal nats resonse from jwt update request: %w", err)
	}

	if res.Data.Code != 200 {
		return fmt.Errorf("jwt update failed <code:%d> <message:%s>", res.Data.Code, res.Data.Message)
	}

	return nil
}

func (n *natsClient) connect(namespace string) error {
	adminCreds, err := n.getOperatorAdminCredentials(context.Background(), namespace)
	if err != nil {
		return fmt.Errorf("failed to fetch admin user creds: %w", err)
	}

	userJwt, err := jwt.ParseDecoratedJWT(adminCreds)
	if err != nil {
		return fmt.Errorf("admin jwt invalid: %w", err)
	}

	userKey, err := jwt.ParseDecoratedUserNKey(adminCreds)
	if err != nil {
		return fmt.Errorf("admin creds invalid: %w", err)
	}

	userSeed, err := userKey.Seed()
	if err != nil {
		return fmt.Errorf("error extracting admin user seed: %w", err)
	}

	n.conn, err = nats.Connect(
		n.natsURL,
		nats.UserJWTAndSeed(userJwt, string(userSeed)),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(7),
		nats.ReconnectWait(time.Second),
	)
	if err != nil {
		return fmt.Errorf("unable to connect to NATS cluster: %w", err)
	}

	return err
}

func (n *natsClient) getOperatorAdminCredentials(ctx context.Context, namespace string) ([]byte, error) {
	labels := map[string]string{
		domain.LabelSecretType: domain.SecretTypeSystemAccountUserCreds,
	}
	secrets, err := n.secretStorer.GetSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	if len(secrets.Items) < 1 {
		return nil, errors.New("missing secret for operator credentials")
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("multiple operator user credentials found, make sure only one secret has the %s: %s label", domain.LabelSecretType, domain.SecretTypeSystemAccountUserCreds)
	}

	creds, ok := secrets.Items[0].Data[domain.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("operator credentials secret key (%s) missing", domain.DefaultSecretKeyName)
	}
	return creds, nil
}

var _ ports.NATSClient = (*natsClient)(nil)
