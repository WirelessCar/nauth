package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

type jetStreamDeletionFixture struct {
	server      *natsserver.Server
	sysConn     *nats.Conn
	observer    *nats.Conn
	js          nats.JetStreamContext
	opKey       nkeys.KeyPair
	opID        string
	accountID   string
	accountFile string
	opts        *jetStreamOptions
	logs        *raceLog
}

func newJetStreamDeletionFixture(t *testing.T) *jetStreamDeletionFixture {
	t.Helper()
	op, err := nkeys.CreateOperator()
	require.NoError(t, err)
	opID, err := op.PublicKey()
	require.NoError(t, err)
	opClaims := jwt.NewOperatorClaims(opID)
	opJWT, err := opClaims.Encode(op)
	require.NoError(t, err)
	opClaims, err = jwt.DecodeOperatorClaims(opJWT)
	require.NoError(t, err)
	resolver, err := natsserver.NewDirAccResolver(t.TempDir(), 0, time.Minute, natsserver.HardDelete)
	require.NoError(t, err)
	t.Cleanup(resolver.Close)
	newAccount := func(jetstream bool) (nkeys.KeyPair, string, *jwt.AccountClaims) {
		key, keyErr := nkeys.CreateAccount()
		require.NoError(t, keyErr)
		id, keyErr := key.PublicKey()
		require.NoError(t, keyErr)
		claims := jwt.NewAccountClaims(id)
		if jetstream {
			claims.Limits.JetStreamLimits = jwt.JetStreamLimits{MemoryStorage: -1, DiskStorage: -1, Streams: -1, Consumer: -1}
		}
		token, encodeErr := claims.Encode(op)
		require.NoError(t, encodeErr)
		require.NoError(t, resolver.Store(id, token))
		return key, id, claims
	}
	sysKey, sysID, _ := newAccount(false)
	accountKey, accountID, _ := newAccount(true)
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true,
		TrustedOperators: []*jwt.OperatorClaims{opClaims}, AccountResolver: resolver,
		SystemAccount: sysID, JetStream: true, StoreDir: t.TempDir(),
	})
	require.NoError(t, err)
	go srv.Start()
	require.True(t, srv.ReadyForConnections(3*time.Second))
	t.Cleanup(func() { srv.Shutdown(); srv.WaitForShutdown() })
	connect := func(key nkeys.KeyPair, id string) *nats.Conn {
		seed, keyErr := key.Seed()
		require.NoError(t, keyErr)
		creds, keyErr := createUserCreds(id, string(seed))
		require.NoError(t, keyErr)
		// System users need access beyond the helper's JetStream permissions.
		user, keyErr := jwt.DecodeUserClaims(creds.jwt)
		require.NoError(t, keyErr)
		user.Permissions = jwt.Permissions{}
		creds.jwt, keyErr = user.Encode(key)
		require.NoError(t, keyErr)
		conn, keyErr := nats.Connect(srv.ClientURL(), nats.UserJWTAndSeed(creds.jwt, creds.seed), nats.NoReconnect())
		require.NoError(t, keyErr)
		t.Cleanup(conn.Close)
		return conn
	}
	sysConn := connect(sysKey, sysID)
	observer := connect(accountKey, accountID)
	js, err := observer.JetStream()
	require.NoError(t, err)
	_, err = js.AddStream(&nats.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	seed, err := accountKey.Seed()
	require.NoError(t, err)
	accountFile := installJetStreamKubectl(t, accountID, seed)
	logs := &raceLog{deadline: make(chan struct{})}
	return &jetStreamDeletionFixture{
		server: srv, sysConn: sysConn, observer: observer, js: js,
		opKey: op, opID: opID, accountID: accountID, accountFile: accountFile, logs: logs,
		opts: &jetStreamOptions{
			namespace: "race", accountName: "example-account", stream: "ORDERS",
			natsURL: srv.ClientURL(), timeoutSeconds: 4, log: newLogger(logs),
			expectAccountDeletion: true,
		},
	}
}

func installJetStreamKubectl(t *testing.T, accountID string, seed []byte) string {
	t.Helper()
	binDir := t.TempDir()
	accountFile := filepath.Join(binDir, "account.json")
	require.NoError(t, os.WriteFile(accountFile, []byte(`{"metadata":{"uid":"race-account-uid","deletionTimestamp":"2026-01-01T00:00:00Z"}}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "kubectl"), []byte(`#!/bin/sh
case "$1 $2" in
  'get accounts.nauth.io')
    case "$*" in
      *' -o json')
        if [ -f "$RACE_ACCOUNT_FILE.unavailable" ]; then
          echo 'Kubernetes API unavailable' >&2
          exit 1
        fi
        cat "$RACE_ACCOUNT_FILE" ;;
      *) printf '%s' "$RACE_ACCOUNT_ID" ;;
    esac ;;
  'get secret')
    if [ "$RACE_REMOVE_CREDS" = true ]; then
      : > "$RACE_ACCOUNT_FILE"
      exit 1
    fi
    printf '%s' "$RACE_ACCOUNT_SEED_B64" ;;
  *) exit 1 ;;
esac
`), 0700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RACE_ACCOUNT_ID", accountID)
	t.Setenv("RACE_ACCOUNT_FILE", accountFile)
	t.Setenv("RACE_ACCOUNT_SEED_B64", base64.StdEncoding.EncodeToString(seed))
	return accountFile
}

// Exercise the real helper, request deadline, JetStream deletion, and JWT
// deletion. Only Kubernetes lookups and transport faults are injected.
func TestJetStreamDeleteLostReplyAccountExpiry(t *testing.T) {
	f := newJetStreamDeletionFixture(t)
	lostReply := make(chan struct{})
	f.opts.natsURL = interceptStreamDelete(t, f.server.Addr().String(), dropDeleteReply, lostReply)
	result := make(chan error, 1)
	go func() {
		result <- runJetStreamDeleteStream(t.Context(), f.opts)
	}()
	select {
	case <-lostReply:
	case <-time.After(3 * time.Second):
		t.Fatal("the helper never received a successful server-side deletion")
	}
	_, err := f.js.StreamInfo("ORDERS")
	require.ErrorIs(t, err, nats.ErrStreamNotFound, "independent connection must prove the stream was deleted")
	t.Log("independent JetStream connection confirms ORDERS no longer exists")
	select {
	case <-f.logs.deadline:
	case <-time.After(3 * time.Second):
		t.Fatal("expected the helper's one-second request deadline")
	}
	f.deleteAccountJWT(t)
	require.NoError(t, os.WriteFile(f.accountFile, nil, 0600))
	err = <-result
	t.Log(f.logs.String())
	require.Contains(t, f.logs.String(), "context deadline exceeded")
	require.Contains(t, f.logs.String(), "nats: connection closed")
	require.Contains(t, f.logs.String(), "independently confirmed deletion of Account")
	require.NoError(t, err, "the requested stream was deleted; account expiry must not turn it into a failed E2E step")
}

func (f *jetStreamDeletionFixture) deleteAccountJWT(t *testing.T) {
	t.Helper()
	deleteClaims := jwt.NewGenericClaims(f.opID)
	deleteClaims.Data["accounts"] = []string{f.accountID}
	deleteJWT, err := deleteClaims.Encode(f.opKey)
	require.NoError(t, err)
	response, err := f.sysConn.Request("$SYS.REQ.CLAIMS.DELETE", []byte(deleteJWT), time.Second)
	require.NoError(t, err)
	require.NoError(t, assertClaimsUpdateResponse(response.Data))
	require.Eventually(t, f.observer.IsClosed, time.Second, time.Millisecond)
}

func TestJetStreamDeleteLostReplyStreamNotFound(t *testing.T) {
	f := newJetStreamDeletionFixture(t)
	f.opts.expectAccountDeletion = false
	f.opts.natsURL = interceptStreamDelete(t, f.server.Addr().String(), dropDeleteReply, make(chan struct{}))
	require.NoError(t, runJetStreamDeleteStream(t.Context(), f.opts))
	_, err := f.js.StreamInfo("ORDERS")
	require.ErrorIs(t, err, nats.ErrStreamNotFound)
	require.Contains(t, f.logs.String(), "context deadline exceeded")
	require.NotContains(t, f.logs.String(), "independently confirmed deletion of Account")
}

func TestJetStreamDeleteFailureKeepsStream(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fault     deleteFault
		stream    string
		wantError string
	}{
		{name: "delete_request_lost", fault: dropDeleteRequest, stream: "ORDERS", wantError: "context deadline exceeded"},
		{name: "connection_closed_before_delete", fault: closeBeforeDelete, stream: "ORDERS", wantError: "nats: connection closed"},
		{name: "invalid_stream_name", stream: "ORDERS.*", wantError: "invalid stream name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newJetStreamDeletionFixture(t)
			f.opts.timeoutSeconds = 2
			f.opts.stream = tc.stream
			if tc.fault != "" {
				f.opts.natsURL = interceptStreamDelete(t, f.server.Addr().String(), tc.fault, make(chan struct{}))
			}
			err := runJetStreamDeleteStream(t.Context(), f.opts)
			require.ErrorContains(t, err, tc.wantError)
			info, err := f.js.StreamInfo("ORDERS")
			require.NoError(t, err, "a failed deletion must leave the real stream observable")
			require.Equal(t, "ORDERS", info.Config.Name)
			require.NotContains(t, f.logs.String(), "independently confirmed deletion of Account")
		})
	}
}

func TestJetStreamDeleteRequiresIndependentAccountEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *jetStreamDeletionFixture)
	}{
		{name: "account_still_exists", change: func(*testing.T, *jetStreamDeletionFixture) {}},
		{name: "kubernetes_unavailable", change: func(t *testing.T, f *jetStreamDeletionFixture) {
			require.NoError(t, os.WriteFile(f.accountFile+".unavailable", nil, 0600))
		}},
		{name: "account_replaced", change: func(t *testing.T, f *jetStreamDeletionFixture) {
			require.NoError(t, os.WriteFile(f.accountFile, []byte(`{"metadata":{"uid":"replacement"}}`), 0600))
		}},
		{name: "mode_not_enabled", change: func(t *testing.T, f *jetStreamDeletionFixture) {
			require.NoError(t, os.WriteFile(f.accountFile, nil, 0600))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newJetStreamDeletionFixture(t)
			f.opts.timeoutSeconds = 3
			f.opts.expectAccountDeletion = tc.name != "mode_not_enabled"
			lostReply := make(chan struct{})
			f.opts.natsURL = interceptStreamDelete(t, f.server.Addr().String(), dropDeleteReply, lostReply)
			result := make(chan error, 1)
			go func() { result <- runJetStreamDeleteStream(t.Context(), f.opts) }()
			select {
			case <-lostReply:
			case <-time.After(3 * time.Second):
				t.Fatal("delete reply was not intercepted")
			}
			_, err := f.js.StreamInfo("ORDERS")
			require.ErrorIs(t, err, nats.ErrStreamNotFound)
			select {
			case <-f.logs.deadline:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not time out")
			}
			f.deleteAccountJWT(t)
			tc.change(t, f)
			require.Error(t, <-result)
			require.NotContains(t, f.logs.String(), "independently confirmed deletion of Account")
		})
	}
}

func TestJetStreamDeleteAccountCleanupDuringCredentialLookup(t *testing.T) {
	f := newJetStreamDeletionFixture(t)
	require.NoError(t, f.js.DeleteStream("ORDERS"))
	f.deleteAccountJWT(t)
	t.Setenv("RACE_REMOVE_CREDS", "true")
	require.NoError(t, runJetStreamDeleteStream(t.Context(), f.opts))
	require.Contains(t, f.logs.String(), "independently confirmed deletion of Account")
}

func TestJetStreamDeleteRequiresTerminatingAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account string
	}{
		{name: "account_missing"},
		{name: "account_active", account: `{"metadata":{"uid":"active"}}`},
		{name: "uid_missing", account: `{"metadata":{"deletionTimestamp":"2026-01-01T00:00:00Z"}}`},
		{name: "malformed_response", account: `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accountFile := installJetStreamKubectl(t, "unused", nil)
			require.NoError(t, os.WriteFile(accountFile, []byte(tc.account), 0600))
			err := runJetStreamDeleteStream(t.Context(), &jetStreamOptions{
				namespace: "race", accountName: "example-account", stream: "ORDERS",
				timeoutSeconds: 1, expectAccountDeletion: true, log: newLogger(io.Discard),
			})
			require.Error(t, err)
		})
	}
}

type raceLog struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	deadline chan struct{}
	once     sync.Once
}

func (l *raceLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if bytes.Contains(p, []byte("context deadline exceeded")) {
		l.once.Do(func() { close(l.deadline) })
	}
	return l.buf.Write(p)
}

func (l *raceLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type deleteFault string

const (
	dropDeleteReply   deleteFault = "drop_reply"
	dropDeleteRequest deleteFault = "drop_request"
	closeBeforeDelete deleteFault = "close_before_delete"
)

func interceptStreamDelete(t *testing.T, upstream string, fault deleteFault, triggered chan struct{}) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		client, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = client.Close() }()
		server, dialErr := net.Dial("tcp", upstream)
		if dialErr != nil {
			return
		}
		defer func() { _ = server.Close() }()
		var once sync.Once
		go func() {
			copyNATSFrames(server, client, func(line string, _ []byte) bool {
				if fault == dropDeleteReply || !strings.HasPrefix(line, "PUB $JS.API.STREAM.DELETE.") {
					return false
				}
				once.Do(func() { close(triggered) })
				if fault == closeBeforeDelete {
					_ = client.Close()
					_ = server.Close()
				}
				return true
			})
		}()
		dropped := false
		copyNATSFrames(client, server, func(_ string, payload []byte) bool {
			if fault != dropDeleteReply || dropped || !bytes.Contains(payload, []byte("stream_delete_response")) {
				return false
			}
			dropped = true
			once.Do(func() { close(triggered) })
			return true
		})
	}()
	return fmt.Sprintf("nats://%s", listener.Addr())
}

func copyNATSFrames(dst io.Writer, src io.Reader, drop func(string, []byte) bool) {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		var payload []byte
		fields := strings.Fields(line)
		if len(fields) > 0 && (fields[0] == "PUB" || fields[0] == "HPUB" || fields[0] == "MSG" || fields[0] == "HMSG") {
			size, err := strconv.Atoi(fields[len(fields)-1])
			if err != nil {
				return
			}
			payload = make([]byte, size+2)
			if _, err := io.ReadFull(reader, payload); err != nil {
				return
			}
		}
		if drop(line, payload) {
			continue
		}
		if _, err := io.WriteString(dst, line); err != nil {
			return
		}
		if _, err := dst.Write(payload); err != nil {
			return
		}
	}
}
