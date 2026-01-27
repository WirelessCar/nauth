package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	v1 "k8s.io/api/core/v1"
)

const (
	natsMaxTimeout = 3 * time.Second
)

type SecretGetter interface {
	Get(ctx context.Context, namespace string, name string) (map[string]string, error)
	GetByLabels(ctx context.Context, namespace string, labels map[string]string) (*v1.SecretList, error)
}

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
	natsURL      string
	secretGetter SecretGetter
	conn         *nats.Conn
	system       *natsv1alpha1.System // Optional System CRD for secretRef-based config
}

func NewClient(natsURL string, secretGetter SecretGetter) *Client {
	return &Client{
		natsURL:      natsURL,
		secretGetter: secretGetter,
	}
}

// NewClientWithSystem creates a client configured to use System CRD's secretRefs
func NewClientWithSystem(natsURL string, secretGetter SecretGetter, system *natsv1alpha1.System) *Client {
	return &Client{
		natsURL:      natsURL,
		secretGetter: secretGetter,
		system:       system,
	}
}

func (n *Client) EnsureConnected(namespace string) error {
	if n.conn != nil && n.conn.IsConnected() {
		return nil
	}
	return n.connect(namespace)
}

func (n *Client) Disconnect() {
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

func (n *Client) LookupAccountJWT(accountID string) (string, error) {
	if n.conn == nil || !n.conn.IsConnected() {
		return "", fmt.Errorf("NATS connection is not established or lost")
	}

	msg, err := n.conn.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", accountID), nil, natsMaxTimeout)
	if err != nil {
		return "", fmt.Errorf("failed to lookup account JWT: %w", err)
	}

	return string(msg.Data), nil
}

func (n *Client) UploadAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.UPDATE", jwt)
}

func (n *Client) DeleteAccountJWT(jwt string) error {
	return n.updateClaimsJWT("$SYS.REQ.CLAIMS.DELETE", jwt)
}

func (n *Client) updateClaimsJWT(subject string, jwt string) error {
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
		return fmt.Errorf("failed to unmarshal nats resonse from jwt request: %w", err)
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

func (n *Client) connect(namespace string) error {
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

func (n *Client) getOperatorAdminCredentials(ctx context.Context, namespace string) ([]byte, error) {
	// If System is configured, use its secretRef
	if n.system != nil {
		return n.getOperatorAdminCredentialsFromSystem(ctx)
	}

	// Legacy label-based lookup
	return n.getOperatorAdminCredentialsFromLabels(ctx, namespace)
}

func (n *Client) getOperatorAdminCredentialsFromSystem(ctx context.Context) ([]byte, error) {
	ref := n.system.Spec.SystemAccountUserCredsSecretRef
	sysNamespace := n.system.GetNamespace()

	secretData, err := n.secretGetter.Get(ctx, sysNamespace, ref.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get system account user creds secret %s/%s: %w", sysNamespace, ref.Name, err)
	}

	key := ref.Key
	if key == "" {
		key = k8s.DefaultSecretKeyName
	}

	creds, ok := secretData[key]
	if !ok {
		return nil, fmt.Errorf("system account user creds secret %s/%s does not contain key %q", sysNamespace, ref.Name, key)
	}

	return []byte(creds), nil
}

func (n *Client) getOperatorAdminCredentialsFromLabels(ctx context.Context, namespace string) ([]byte, error) {
	labels := map[string]string{
		k8s.LabelSecretType: k8s.SecretTypeSystemAccountUserCreds,
	}
	secrets, err := n.secretGetter.GetByLabels(ctx, namespace, labels)
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
