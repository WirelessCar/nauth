package ports

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type NatsOperatorSigningKey nkeys.KeyPair

type NatsUserCreds struct {
	Creds     []byte
	AccountID string
}

func NewNatsUserCreds(creds []byte) (*NatsUserCreds, error) {
	if len(creds) == 0 {
		return nil, fmt.Errorf("NATS User Credentials cannot be empty")
	}

	userJWT, err := jwt.ParseDecoratedJWT(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user credentials JWT: %w", err)
	}

	userClaims, err := jwt.DecodeUserClaims(userJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to decode user claims from JWT: %w", err)
	}

	accountID := userClaims.IssuerAccount
	if accountID == "" {
		return nil, fmt.Errorf("user credentials JWT does not contain an issuer account ID")
	}

	n := &NatsUserCreds{
		Creds:     creds,
		AccountID: accountID,
	}

	if err := n.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NATS user credentials: %w", err)
	}

	return n, nil
}

func (n *NatsUserCreds) Validate() error {
	if len(n.Creds) == 0 {
		return fmt.Errorf("credentials cannot be empty")
	}
	if n.AccountID == "" {
		return fmt.Errorf("user credentials must include an account ID")
	}
	return nil
}

type NatsClient interface {
	Connect(natsURL string, userCreds NatsUserCreds) (NatsConnection, error)
}

type NatsConnection interface {
	Disconnect()
	EnsureConnected() error
	LookupAccountJWT(accountID string) (string, error)
	UploadAccountJWT(jwt string) error
	DeleteAccountJWT(jwt string) error
}
