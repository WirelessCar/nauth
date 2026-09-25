package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type NatsOperatorSigningKey nkeys.KeyPair

type NatsAccountStateStatus string

const (
	NatsAccountStateUnknown    NatsAccountStateStatus = "Unknown"
	NatsAccountStateComplete   NatsAccountStateStatus = "Complete"
	NatsAccountStateIncomplete NatsAccountStateStatus = "Incomplete"
)

type NatsAccountState struct {
	Status     NatsAccountStateStatus
	ServerID   string
	AccountID  string
	ClaimsHash string
	Imports    []NatsAccountImport
}

type NatsAccountImport struct {
	AccountID    string
	Subject      string
	LocalSubject string
	Type         string
	Invalid      bool
}

func (s NatsAccountState) HasInvalidImports() bool {
	for _, imp := range s.Imports {
		if imp.Invalid {
			return true
		}
	}
	return false
}

func (s NatsAccountState) MatchesClaimsHash(expected string) bool {
	return expected != "" && s.ClaimsHash != "" && s.ClaimsHash == expected
}

// HashNatsAccountJWTClaims returns a stable hash of the claims in an Account JWT.
// Unstable JWT metadata is excluded so equivalent Account content hashes the same
// across reconciliations.
func HashNatsAccountJWTClaims(accountJWT string) (string, error) {
	claims, err := jwt.DecodeAccountClaims(accountJWT)
	if err != nil {
		return "", fmt.Errorf("failed to decode account JWT claims for hashing: %w", err)
	}
	claims.IssuedAt = 0
	claims.ID = ""

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

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
