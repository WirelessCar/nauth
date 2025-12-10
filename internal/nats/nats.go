package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	v1 "k8s.io/api/core/v1"
)

const (
	natsMaxTimeout = 3 * time.Second
)

type SecretGetter interface {
	GetSecretsByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
}

type NatsResponse struct {
	Data NatsData `json:"data"`
}

type NatsData struct {
	Account string `json:"account,omitempty"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type NatsClient struct {
	natsURL      string
	secretGetter SecretGetter
	conn         *nats.Conn
}

func NewNATSClient(natsURL string, secretGetter SecretGetter) *NatsClient {
	return &NatsClient{
		natsURL:      natsURL,
		secretGetter: secretGetter,
	}
}

func (n *NatsClient) EnsureConnected(namespace string) error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.connect(namespace)
}

func (n *NatsClient) Disconnect() {
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

func (n *NatsClient) LookupAccountJWT(accountID string) (string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return "", fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, natsMaxTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to lookup account JWT: %w", err)
	}

	return string(msg.Data), nil
}

func (n *NatsClient) UploadAccountJWT(jwt string) error {
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

func (n *NatsClient) DeleteAccountJWT(jwt string) error {
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

func (n *NatsClient) connect(namespace string) error {
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

func (n *NatsClient) getOperatorAdminCredentials(ctx context.Context, namespace string) ([]byte, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
	}
	secrets, err := n.secretGetter.GetSecretsByLabels(ctx, namespace, labels)
	if err != nil {
		return nil, err
	}

	if len(secrets.Items) < 1 {
		return nil, errors.New("missing secret for operator credentials")
	}

	if len(secrets.Items) > 1 {
		return nil, fmt.Errorf("multiple operator user credentials found, make sure only one secret has the %s: %s label", k8s.LabelSecretType, k8s.SecretTypeSystemAccountUserCreds)
	}

	creds, ok := secrets.Items[0].Data[k8s.DefaultSecretKeyName]
	if !ok {
		return nil, fmt.Errorf("operator credentials secret key (%s) missing", k8s.DefaultSecretKeyName)
	}
	return creds, nil
}
