package nats

import (
	"encoding/json"
	"fmt"
)

const (
	accountLoadRequestSubject = "$SYS.REQ.ACCOUNT.NSUBS"
	// accountLoadSubject is an arbitrary literal required by ACCOUNT.NSUBS while calculating
	// subscription counts; NAuth ignores the count and uses the Account lookup as the side effect.
	accountLoadSubject = "x"
)

type accountLoadRequest struct {
	Account string `json:"acc"`
	Subject string `json:"subject"`
}

func (n *connection) RequestAccountLoad(accountID string) error {
	if n.conn == nil || !n.conn.IsConnected() {
		return fmt.Errorf("NATS connection is not established or lost")
	}
	if accountID == "" {
		return fmt.Errorf("account ID is required")
	}

	payload, err := json.Marshal(accountLoadRequest{
		Account: accountID,
		Subject: accountLoadSubject,
	})
	if err != nil {
		return fmt.Errorf("failed to encode account load request: %w", err)
	}

	if err := n.conn.Publish(accountLoadRequestSubject, payload); err != nil {
		return fmt.Errorf("failed to publish account load request: %w", err)
	}
	if err := n.conn.Flush(); err != nil {
		return fmt.Errorf("failed to flush account load request: %w", err)
	}

	return nil
}
