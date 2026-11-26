package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/outbound"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	natsMaxTimeout = 3 * time.Second
	// connectErrorWait bounds how long a failed connection waits for nats.go to report its first error.
	connectErrorWait = 500 * time.Millisecond
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

// ClientOptions configures operator-wide behaviour shared by all NATS connections.
type ClientOptions struct {
	// DefaultCAFile is the path to the operator-wide PEM CA bundle. Empty means none.
	DefaultCAFile string
}

// SysClient connects to NATS as a system account user.
type SysClient struct {
	opts ClientOptions
}

// NewSysClient returns a SysClient that applies opts to every connection.
func NewSysClient(opts ClientOptions) *SysClient {
	return &SysClient{opts: opts}
}

// Connect opens a system account connection to natsURL using tlsConfig and userCreds.
func (n *SysClient) Connect(natsURL string, tlsConfig *domain.NatsTLSConfig, userCreds domain.NatsUserCreds) (outbound.NatsSysConnection, error) {
	return connect(natsURL, tlsConfig, userCreds, n.opts)
}

// AccountClient connects to NATS as a regular account user.
type AccountClient struct {
	opts ClientOptions
}

// NewAccountClient returns an AccountClient that applies opts to every connection.
func NewAccountClient(opts ClientOptions) *AccountClient {
	return &AccountClient{opts: opts}
}

// Connect opens an account connection to natsURL using tlsConfig and userCreds.
func (c AccountClient) Connect(natsURL string, tlsConfig *domain.NatsTLSConfig, userCreds domain.NatsUserCreds) (outbound.NatsAccountConnection, error) {
	return connect(natsURL, tlsConfig, userCreds, c.opts)
}

// validateNATSURLs parses each entry of a comma-separated NATS URL list the way nats.go does. Errors
// name only the entry position: URL parse errors echo the URL, including any embedded credentials.
func validateNATSURLs(natsURL string) error {
	entry := 0
	for _, u := range strings.Split(natsURL, ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		entry++
		if !strings.Contains(u, "://") {
			u = "nats://" + u
		}
		if _, err := url.Parse(u); err != nil {
			return fmt.Errorf("invalid NATS URL (entry %d): check its format", entry)
		}
	}
	return nil
}

// connect validates its inputs, builds the TLS options and opens a connection.
func connect(natsURL string, tlsConfig *domain.NatsTLSConfig, userCreds domain.NatsUserCreds, clientOpts ClientOptions) (*connection, error) {
	if natsURL == "" {
		return nil, fmt.Errorf("NATS URL is required")
	}
	if err := validateNATSURLs(natsURL); err != nil {
		return nil, err
	}

	if err := userCreds.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NATS user credentials: %w", err)
	}

	tlsOpts, caSource, err := buildTLSOptions(tlsConfig, clientOpts.DefaultCAFile, urlRequestsTLS(natsURL), os.ReadFile)
	if err != nil {
		return nil, fmt.Errorf("configure NATS TLS: %w", err)
	}

	c := &connection{
		natsURL:   natsURL,
		userCreds: userCreds,
		tlsOpts:   tlsOpts,
		caSource:  caSource,
		redactor:  newCredentialRedactor(natsURL, userCreds.Creds),
	}
	if err := c.EnsureConnected(); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS cluster: %w", err)
	}

	return c, nil
}

type connection struct {
	natsURL   string
	userCreds domain.NatsUserCreds
	tlsOpts   []nats.Option
	caSource  domain.NatsCASource
	redactor  credentialRedactor
	conn      *nats.Conn

	errMu    sync.Mutex
	lastErr  error
	firstErr chan struct{}
}

// recordConnectError keeps the latest connect error. With RetryOnFailedConnect, nats.go reports
// initial-connect failures (TLS, auth, timeouts) only through ReconnectErrCB, not through LastError().
func (n *connection) recordConnectError(_ *nats.Conn, err error) {
	if err == nil {
		return
	}
	n.errMu.Lock()
	defer n.errMu.Unlock()
	if n.lastErr == nil && n.firstErr != nil {
		close(n.firstErr)
	}
	n.lastErr = err
}

// connectError returns the most recent connect error, waiting briefly for the first one because
// nats.go delivers it asynchronously.
func (n *connection) connectError() error {
	n.errMu.Lock()
	err, first := n.lastErr, n.firstErr
	n.errMu.Unlock()
	if err != nil || first == nil {
		return err
	}
	select {
	case <-first:
	case <-time.After(connectErrorWait):
	}
	n.errMu.Lock()
	defer n.errMu.Unlock()
	return n.lastErr
}

func (n *connection) checkConnection() error {
	if n.conn == nil {
		return fmt.Errorf("NATS connection is not initialized")
	}
	if n.conn.IsConnected() {
		return nil
	}

	status := n.conn.Status()
	err := n.conn.LastError()
	if err == nil {
		err = n.connectError()
	}
	if cause := describeNATSConnectionError(err, n.caSource, n.redactor); cause != "" {
		return fmt.Errorf("NATS connection is unavailable (status: %s; cause: %s)", status, cause)
	}
	return fmt.Errorf("NATS connection is unavailable (status: %s)", status)
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
	if err := n.checkConnection(); err != nil {
		return err
	}

	_, err := n.conn.Request("$SYS.REQ.SERVER.PING", nil, natsMaxTimeout)
	if err != nil {
		return fmt.Errorf("failed system account ping: %w", err)
	}

	return nil
}

func (n *connection) ListAccountStreams() ([]string, error) {
	if err := n.checkConnection(); err != nil {
		return nil, err
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
	if err := n.checkConnection(); err != nil {
		return "", err
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
	if err := n.checkConnection(); err != nil {
		return err
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

	n.errMu.Lock()
	n.lastErr = nil
	n.firstErr = make(chan struct{})
	n.errMu.Unlock()

	opts := []nats.Option{
		nats.ReconnectErrHandler(n.recordConnectError),
		nats.UserCredentialBytes(n.userCreds.Creds),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(7),
		nats.ReconnectWait(time.Second),
	}
	n.conn, err = nats.Connect(n.natsURL, append(opts, n.tlsOpts...)...)
	if err != nil {
		return fmt.Errorf("unable to connect to NATS cluster: %w", redactedError{err: err, redactor: n.redactor})
	}

	return err
}

// Compile-time assertion that implementations fulfills ports
var _ outbound.NatsSysClient = (*SysClient)(nil)
var _ outbound.NatsAccountClient = (*AccountClient)(nil)
var _ outbound.NatsSysConnection = (*connection)(nil)
var _ outbound.NatsAccountConnection = (*connection)(nil)
