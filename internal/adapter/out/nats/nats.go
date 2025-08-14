package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
)

const (
	NATS_MAX_TIMEOUT = 3 * time.Second
)

type NatsResponse struct {
	Data NatsData `json:"data"`
}

type NatsData struct {
	Account string `json:"account,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type NATSClient struct {
	natsUrl      string
	secretStorer ports.SecretStorer
	conn         *nats.Conn
}

func NewNATSClient(natsUrl string, secretStorer ports.SecretStorer) *NATSClient {
	return &NATSClient{
		natsUrl:      natsUrl,
		secretStorer: secretStorer,
	}
}

func (n *NATSClient) Connect(namespace string, secretName string) error {
	credsSecretValue, err := n.secretStorer.GetSecret(context.Background(), namespace, secretName)
	if err != nil {
		return fmt.Errorf("failed to fetch admin user creds: %w", err)
	}
	adminCreds := credsSecretValue[domain.DefaultSecretKeyName]

	userKey, err := jwt.ParseDecoratedUserNKey([]byte(adminCreds))
	if err != nil {
		return fmt.Errorf("admin creds invalid: %w", err)
	}

	userSeed, err := userKey.Seed()
	if err != nil {
		return fmt.Errorf("error extracting admin user seed: %w", err)
	}

	userJwt, err := jwt.ParseDecoratedJWT([]byte(adminCreds))
	if err != nil {
		return fmt.Errorf("admin jwt invalid: %w", err)
	}

	n.conn, err = nats.Connect(
		n.natsUrl,
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

func (n *NATSClient) EnsureConnected(namespace string, secretName string) error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.Connect(namespace, secretName)
}

func (n *NATSClient) Disconnect() {
	if n.conn != nil && n.conn.IsConnected() {
		n.conn.Drain()
		n.conn.Close()
	} else if n.conn != nil {
		n.conn.Close()
	}
}

func (n *NATSClient) UploadAccountJWT(jwt string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request("$SYS.REQ.CLAIMS.UPDATE", []byte(jwt), NATS_MAX_TIMEOUT)
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

func (n *NATSClient) DeleteAccountJWT(jwt string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request("$SYS.REQ.CLAIMS.DELETE", []byte(jwt), NATS_MAX_TIMEOUT)
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
