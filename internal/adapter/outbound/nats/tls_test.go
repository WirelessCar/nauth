package nats

import (
	cryptotls "crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/nats-io/jwt/v2"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestBuildTLSOptions_Precedence(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	failRead := func(string) ([]byte, error) {
		t.Fatal("default CA file must not be read when a CA bundle is configured")
		return nil, nil
	}
	readCA := func(string) ([]byte, error) { return pki.CAPEM, nil }

	tests := []struct {
		name          string
		cfg           *domain.NatsTLSConfig
		defaultCAFile string
		tlsURL        bool
		readFile      func(string) ([]byte, error)
		wantSource    domain.NatsCASource
		wantOptions   bool
		wantErr       string
	}{
		{
			name:          "cluster_ca_wins_and_file_is_not_read",
			cfg:           &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM},
			defaultCAFile: "/x",
			readFile:      failRead,
			wantSource:    domain.NatsCASourceConfigured,
			wantOptions:   true,
		},
		{
			name:          "operator_file_used_when_cluster_ca_empty",
			cfg:           &domain.NatsTLSConfig{HandshakeFirst: true},
			defaultCAFile: "/x",
			readFile:      readCA,
			wantSource:    domain.NatsCASourceOperatorFile,
			wantOptions:   true,
		},
		{
			name:          "operator_file_used_for_tls_url_without_spec_tls",
			defaultCAFile: "/x",
			tlsURL:        true,
			readFile:      readCA,
			wantSource:    domain.NatsCASourceOperatorFile,
			wantOptions:   true,
		},
		{
			name:          "operator_file_ignored_for_plain_url_without_spec_tls",
			defaultCAFile: "/x",
			readFile:      failRead,
			wantSource:    domain.NatsCASourceSystem,
		},
		{
			name:        "empty_spec_tls_requires_tls_with_system_roots",
			cfg:         &domain.NatsTLSConfig{},
			readFile:    failRead,
			wantSource:  domain.NatsCASourceSystem,
			wantOptions: true,
		},
		{
			name:       "system_roots_when_nothing_configured",
			readFile:   failRead,
			wantSource: domain.NatsCASourceSystem,
		},
		{
			name:          "missing_operator_file",
			defaultCAFile: "/x",
			tlsURL:        true,
			readFile:      func(string) ([]byte, error) { return nil, os.ErrNotExist },
			wantErr:       "read default CA file /x",
		},
		{
			name:          "invalid_operator_file",
			defaultCAFile: "/x",
			tlsURL:        true,
			readFile:      func(string) ([]byte, error) { return []byte("garbage"), nil },
			wantErr:       "default CA file /x: CA bundle contains no PEM certificates",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, source, err := buildTLSOptions(tt.cfg, tt.defaultCAFile, tt.tlsURL, tt.readFile)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSource, source)
			require.Equal(t, tt.wantOptions, len(opts) > 0)
		})
	}
}

func TestBuildTLSOptions_ErrorsDoNotNameCRDFields(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	tests := []struct {
		name    string
		cfg     *domain.NatsTLSConfig
		wantErr string
	}{
		{
			name:    "invalid_ca_bundle",
			cfg:     &domain.NatsTLSConfig{CABundlePEM: []byte("garbage")},
			wantErr: "configured CA bundle: CA bundle contains no PEM certificates",
		},
		{
			name:    "invalid_client_certificate",
			cfg:     &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM, ClientCertPEM: []byte("bad"), ClientKeyPEM: []byte("bad")},
			wantErr: "TLS client certificate configuration: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := buildTLSOptions(tt.cfg, "", false, os.ReadFile)
			require.ErrorContains(t, err, tt.wantErr)
			require.NotContains(t, err.Error(), "spec.")
		})
	}
}

func TestBuildTLSOptions_EmptySpecTLS_ShouldInstallVerifyingConfig(t *testing.T) {
	opts, source, err := buildTLSOptions(&domain.NatsTLSConfig{}, "", false, os.ReadFile)
	require.NoError(t, err)
	require.Equal(t, domain.NatsCASourceSystem, source)

	var o nats.Options
	for _, opt := range opts {
		require.NoError(t, opt(&o))
	}
	require.NotNil(t, o.TLSConfig, "spec.tls present must install a TLS config so nats.go requires TLS")
	require.Nil(t, o.TLSConfig.RootCAs, "nil RootCAs means the system roots")
	require.Empty(t, o.TLSConfig.Certificates)
	require.False(t, o.TLSConfig.InsecureSkipVerify)
	require.False(t, o.TLSHandshakeFirst, "an empty block keeps the INFO-first handshake")
}

func TestBuildTLSOptions_NoSpecTLSPlainURL_ShouldLeaveTLSOptIn(t *testing.T) {
	// Helm default: nats.tls.caFile is empty and the NatsCluster has no spec.tls.
	opts, _, err := buildTLSOptions(nil, "", false, os.ReadFile)
	require.NoError(t, err)
	require.Empty(t, opts)
}

func TestURLRequestsTLS(t *testing.T) {
	require.True(t, urlRequestsTLS("tls://a:4222"))
	require.True(t, urlRequestsTLS("nats://a:4222, TLS://b:4222"))
	require.True(t, urlRequestsTLS("wss://a:443"))
	require.False(t, urlRequestsTLS("nats://a:4222"))
	require.False(t, urlRequestsTLS("ws://a:80,nats://b:4222"))
}

func TestConnect_ClusterCA_ShouldConnectToPrivateCAServer(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.url, &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.NoError(t, conn.VerifySystemAccountAccess())
}

func TestConnect_OperatorCAFileRotated_ShouldUseNewBundle(t *testing.T) {
	oldPKI := testutil.NewTestPKI(t)
	newPKI := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = newPKI.ServerTLS
	})
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, oldPKI.CAPEM, 0o600))
	client := NewSysClient(ClientOptions{DefaultCAFile: caFile})

	first, err := client.Connect(server.tlsURL(), nil, server.sysCreds)
	require.NoError(t, err)
	require.Error(t, first.VerifySystemAccountAccess(), "server certificate must not verify against the old CA only")
	first.Disconnect()

	require.NoError(t, os.WriteFile(caFile, append(append([]byte{}, oldPKI.CAPEM...), newPKI.CAPEM...), 0o600))
	second, err := client.Connect(server.tlsURL(), nil, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(second.Disconnect)
	require.NoError(t, second.VerifySystemAccountAccess())
}

func TestConnect_OperatorCA_ShouldNotForceTLSOnPlainServer(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, nil)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, pki.CAPEM, 0o600))

	conn, err := NewSysClient(ClientOptions{DefaultCAFile: caFile}).Connect(server.url, nil, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.NoError(t, conn.VerifySystemAccountAccess())
}

func TestConnect_OperatorCAFileMissing_ShouldFail(t *testing.T) {
	server := runTLSServer(t, nil)

	_, err := NewSysClient(ClientOptions{DefaultCAFile: filepath.Join(t.TempDir(), "missing.crt")}).Connect(server.tlsURL(), nil, server.sysCreds)

	require.ErrorContains(t, err, "read default CA file")
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestConnect_HandshakeFirst_ShouldConnect(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
		o.TLSHandshakeFirst = true
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM, HandshakeFirst: true}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.NoError(t, conn.VerifySystemAccountAccess())
}

func TestConnect_HandshakeFirstWithOperatorCA_ShouldConnect(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
		o.TLSHandshakeFirst = true
	})
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, pki.CAPEM, 0o600))

	conn, err := NewSysClient(ClientOptions{DefaultCAFile: caFile}).Connect(server.tlsURL(), &domain.NatsTLSConfig{HandshakeFirst: true}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.NoError(t, conn.VerifySystemAccountAccess())
}

// runVerifyingTLSServer starts a TLS server that requires a client certificate signed by pki.
func runVerifyingTLSServer(t *testing.T, pki *testutil.TestPKI) tlsTestServer {
	t.Helper()
	serverTLS := pki.ServerTLS.Clone()
	serverTLS.ClientAuth = cryptotls.RequireAndVerifyClientCert
	return runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSVerify = true
		o.TLSConfig = serverTLS
	})
}

func TestConnect_ClientCert_ShouldConnectToVerifyingServer(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runVerifyingTLSServer(t, pki)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{
		CABundlePEM:   pki.CAPEM,
		ClientCertPEM: pki.ClientCertPEM,
		ClientKeyPEM:  pki.ClientKeyPEM,
	}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.NoError(t, conn.VerifySystemAccountAccess())
}

func TestConnect_NoClientCert_ShouldFailAgainstVerifyingServer(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runVerifyingTLSServer(t, pki)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	require.Error(t, conn.VerifySystemAccountAccess())
}

func TestConnect_UnknownAuthority_ShouldNameCASource(t *testing.T) {
	serverPKI := testutil.NewTestPKI(t)
	otherPKI := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = serverPKI.ServerTLS
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: otherPKI.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "signed by unknown authority (trusted CAs from configured CA bundle)")
}

func TestConnect_UnknownAuthorityWithOperatorCA_ShouldNameCASource(t *testing.T) {
	serverPKI := testutil.NewTestPKI(t)
	otherPKI := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = serverPKI.ServerTLS
	})
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, otherPKI.CAPEM, 0o600))

	conn, err := NewSysClient(ClientOptions{DefaultCAFile: caFile}).Connect(server.tlsURL(), nil, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "signed by unknown authority (trusted CAs from default CA file)")
}

func TestConnect_HandshakeFirstMismatch_ShouldReportOriginalError(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
		o.TLSHandshakeFirst = true
		o.TLSTimeout = handshakeMismatchServerTLSTimeout
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	requireOriginalTimeout(t, err)
}

func TestConnect_HandshakeFirstMismatchWithSystemRoots_ShouldReportOriginalError(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
		o.TLSHandshakeFirst = true
		o.TLSTimeout = handshakeMismatchServerTLSTimeout
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), nil, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	requireOriginalTimeout(t, err)
}

func TestConnect_HandshakeFirstMismatch_Reverse_ShouldReportOriginalError(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
	})

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM, HandshakeFirst: true}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "first record does not look like a TLS handshake")
	require.NotContains(t, err.Error(), "handshake setting")
}

func TestConnect_TLSConfiguredPlainServer_ShouldReport(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, nil)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.url, &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "tls configured but server does not offer TLS")
}

func TestConnect_EmptySpecTLSPlainServer_ShouldRejectPlaintext(t *testing.T) {
	server := runTLSServer(t, nil)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.url, &domain.NatsTLSConfig{}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "tls configured but server does not offer TLS")
}

func TestConnect_EmptySpecTLS_ShouldVerifyAgainstSystemRoots(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runTLSServer(t, func(o *natsserver.Options) {
		o.TLS = true
		o.TLSConfig = pki.ServerTLS
	})

	// A nats:// URL with an empty spec.tls still negotiates and verifies TLS; the test CA is not a system root.
	// The wording differs by platform (macOS uses the OS verifier), so only assert an x509 failure.
	conn, err := NewSysClient(ClientOptions{}).Connect(server.url, &domain.NatsTLSConfig{}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	err = conn.VerifySystemAccountAccess()
	require.ErrorContains(t, err, "x509:")
}

func TestConnect_ClientCertMissing_ShouldReportRejection(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	server := runVerifyingTLSServer(t, pki)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{CABundlePEM: pki.CAPEM}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	requireRejectedWithCause(t, conn.VerifySystemAccountAccess())
}

func TestConnect_ClientCertUnknownCA_ShouldReportRejection(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	otherPKI := testutil.NewTestPKI(t)
	server := runVerifyingTLSServer(t, pki)

	conn, err := NewSysClient(ClientOptions{}).Connect(server.tlsURL(), &domain.NatsTLSConfig{
		CABundlePEM:   pki.CAPEM,
		ClientCertPEM: otherPKI.ClientCertPEM,
		ClientKeyPEM:  otherPKI.ClientKeyPEM,
	}, server.sysCreds)
	require.NoError(t, err)
	t.Cleanup(conn.Disconnect)

	requireRejectedWithCause(t, conn.VerifySystemAccountAccess())
}

// handshakeMismatchServerTLSTimeout is the server TLS timeout, in seconds, for the handshake-first mismatch
// tests. It is well above the client's 2s connect timeout, so the client always times out first. With equal
// timeouts the server could close the connection first and the client would report EOF instead.
const handshakeMismatchServerTLSTimeout = 10.0

// requireRejectedWithCause asserts that the server rejected the connection and that an underlying cause is
// reported. A rejected client certificate surfaces as a TLS alert, a broken pipe or a connection reset depending
// on timing (with TLS 1.3 the client finishes its side of the handshake before the server verifies the
// certificate), so the exact alert text is asserted by TestDescribeNATSConnectionError instead.
func requireRejectedWithCause(t *testing.T, err error) {
	t.Helper()
	require.ErrorContains(t, err, "NATS connection is unavailable")
	require.Regexp(t, `; cause: \S`, err.Error())
	require.NotContains(t, err.Error(), "x509:", "the client trusts the server, so the rejection must come from the server")
}

// requireOriginalTimeout asserts err reports the underlying timeout rather than an assumed TLS diagnosis.
func requireOriginalTimeout(t *testing.T, err error) {
	t.Helper()
	require.ErrorContains(t, err, "timeout")
	require.NotContains(t, err.Error(), "during TLS setup")
	require.NotContains(t, err.Error(), "handshake setting")
}

// tlsTestServer is a running test server and its system account credentials.
type tlsTestServer struct {
	url      string
	sysCreds domain.NatsUserCreds
}

// tlsURL returns the server URL with the tls:// scheme.
func (s tlsTestServer) tlsURL() string {
	return strings.Replace(s.url, "nats://", "tls://", 1)
}

// runTLSServer starts an operator-mode nats-server with a system account. mutate, if non-nil, adjusts its options.
func runTLSServer(t *testing.T, mutate func(*natsserver.Options)) tlsTestServer {
	t.Helper()
	op := newOperator(t)
	sysAcc := newAccount(t, op, nil)
	resolver, err := natsserver.NewDirAccResolver(t.TempDir(), 0, time.Minute, natsserver.NoDelete)
	require.NoError(t, err)
	require.NoError(t, resolver.Store(sysAcc.key.PublicKey, sysAcc.jwt))

	opts := &natsserver.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		DisableShortFirstPing: true,
		TrustedOperators:      []*jwt.OperatorClaims{op.claims},
		AccountResolver:       resolver,
		SystemAccount:         sysAcc.key.PublicKey,
	}
	if mutate != nil {
		mutate(opts)
	}
	server, err := natsserver.NewServer(opts)
	require.NoError(t, err)
	go server.Start()
	require.True(t, server.ReadyForConnections(3*time.Second), "nats-server did not become ready in time")
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
		resolver.Close()
	})

	creds, err := domain.NewNatsUserCreds(newUserCreds(t, sysAcc))
	require.NoError(t, err)
	return tlsTestServer{url: server.ClientURL(), sysCreds: *creds}
}

func TestConnect_InvalidURLWithCredentials_ShouldRedact(t *testing.T) {
	server := runTLSServer(t, nil)

	for _, tt := range []struct{ name, url, password string }{
		{name: "plain", url: "nats://alice:s3cret@127.0.0.1:notaport", password: "s3cret"},
		{name: "at_sign", url: "nats://alice:p@ss@127.0.0.1:notaport", password: "ss@"},
		{name: "space_and_quote", url: `nats://alice:p"s s@127.0.0.1:notaport`, password: "s s"},
		{name: "backslash", url: `nats://alice:p\\ss@127.0.0.1:notaport`, password: "ss@"},
		{name: "no_at_sign", url: "nats://alice:s3cret", password: "s3cret"},
		{name: "second_entry", url: "nats://127.0.0.1:4222,nats://bob:hunter2@127.0.0.1:notaport", password: "hunter2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSysClient(ClientOptions{}).Connect(tt.url, nil, server.sysCreds)
			require.ErrorContains(t, err, "invalid NATS URL (entry ")
			require.NotContains(t, err.Error(), tt.password)
			require.NotContains(t, err.Error(), "alice")
			require.NotContains(t, err.Error(), "bob")
		})
	}
}
