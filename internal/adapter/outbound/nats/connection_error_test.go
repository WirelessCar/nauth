package nats

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestDescribeNATSConnectionError(t *testing.T) {
	wrap := func(err error) error { return fmt.Errorf("dial: %w", err) }
	unknownAuthority := wrap(x509.UnknownAuthorityError{})
	hostname := wrap(x509.HostnameError{Certificate: &x509.Certificate{}, Host: "nats.example"})
	expired := wrap(x509.CertificateInvalidError{Reason: x509.Expired})
	recordHeader := wrap(tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"})
	unknownCAAlert := wrap(errors.New("remote error: tls: unknown certificate authority"))
	certRequiredAlert := wrap(tls.AlertError(116))
	deadline := wrap(&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded})

	tests := []struct {
		name     string
		err      error
		caSource domain.NatsCASource
		want     string
	}{
		{name: "nil", err: nil, want: ""},

		// Straightforward mappings for known errors. x509 mappings keep the original error.
		{name: "unknown_authority", err: unknownAuthority, caSource: domain.NatsCASourceSystem,
			want: "tls: server certificate signed by unknown authority (trusted CAs from system roots): " + unknownAuthority.Error()},
		{name: "hostname", err: hostname,
			want: "tls: server certificate is not valid for host nats.example: " + hostname.Error()},
		{name: "expired", err: expired,
			want: "tls: server certificate expired or not yet valid: " + expired.Error()},
		{name: "secure_wanted", err: wrap(nats.ErrSecureConnWanted), want: "tls configured but server does not offer TLS"},
		{name: "secure_required", err: wrap(nats.ErrSecureConnRequired), want: "server requires TLS"},
		{name: "authorization", err: nats.ErrAuthorization, want: "authentication failed: authorization violation"},
		{name: "no_servers", err: nats.ErrNoServers, want: "no NATS servers available"},
		{name: "nats_timeout", err: nats.ErrTimeout, want: "connection attempt timed out"},

		// Everything else keeps the original error instead of an assumed diagnosis.
		{name: "record_header_is_original", err: recordHeader, want: recordHeader.Error()},
		{name: "remote_unknown_ca_alert_is_original", err: unknownCAAlert, want: unknownCAAlert.Error()},
		{name: "certificate_required_alert_is_original", err: certRequiredAlert, want: certRequiredAlert.Error()},
		{name: "network_deadline_is_original", err: deadline, want: deadline.Error()},
		{name: "unknown_is_original", err: errors.New("boom"), want: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, describeNATSConnectionError(tt.err, tt.caSource, credentialRedactor{}))
		})
	}
}

func TestDescribeNATSConnectionError_RedactsCredentials(t *testing.T) {
	// A throwaway user seed and a JWT-shaped token; neither grants access to anything.
	kp, err := nkeys.CreateUser()
	require.NoError(t, err)
	seedBytes, err := kp.Seed()
	require.NoError(t, err)
	seed := string(seedBytes)
	const userJWT = "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJYWFgifQ.c2lnbmF0dXJl"
	tests := []struct {
		name    string
		err     error
		secrets []string
		keep    []string
	}{
		{
			name:    "url_userinfo",
			err:     errors.New(`parse "nats://alice:s3cret@nats.example:4222": invalid port`),
			secrets: []string{"alice", "s3cret"},
			keep:    []string{"nats://[REDACTED]@nats.example:4222", "invalid port"},
		},
		{
			name:    "url_token_only",
			err:     errors.New("dial tls://mytoken@nats.example:4443 failed"),
			secrets: []string{"mytoken"},
			keep:    []string{"tls://[REDACTED]@nats.example:4443"},
		},
		{
			name:    "nkey_seed",
			err:     fmt.Errorf("bad creds %s here", seed),
			secrets: []string{seed},
			keep:    []string{"bad creds [REDACTED] here"},
		},
		{
			name:    "jwt",
			err:     fmt.Errorf("rejected jwt %s", userJWT),
			secrets: []string{userJWT, "eyJ0eXAi"},
			keep:    []string{"rejected jwt [REDACTED]"},
		},
		{
			name: "url_without_userinfo_untouched",
			err:  errors.New("dial tcp nats://nats.example:4222: connection refused"),
			keep: []string{"nats://nats.example:4222: connection refused"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeNATSConnectionError(tt.err, domain.NatsCASourceSystem, credentialRedactor{})
			for _, s := range tt.secrets {
				require.NotContains(t, got, s)
			}
			for _, k := range tt.keep {
				require.Contains(t, got, k)
			}
		})
	}

	t.Run("mapped_cause_keeps_original_redacted", func(t *testing.T) {
		err := fmt.Errorf("nats://bob:pw@h:4222: %w", x509.UnknownAuthorityError{})
		got := describeNATSConnectionError(err, domain.NatsCASourceSystem, credentialRedactor{})
		require.True(t, strings.HasPrefix(got, "tls: server certificate signed by unknown authority"))
		require.NotContains(t, got, "bob:pw")
	})
}

func TestCredentialRedactor(t *testing.T) {
	creds := newUserCreds(t, newAccount(t, newOperator(t), nil))
	userJWT, err := jwt.ParseDecoratedJWT(creds)
	require.NoError(t, err)
	kp, err := jwt.ParseDecoratedNKey(creds)
	require.NoError(t, err)
	seed, err := kp.Seed()
	require.NoError(t, err)

	tests := []struct {
		name    string
		natsURL string
		msg     string
		secrets []string
	}{
		{
			name:    "at_sign_in_password",
			natsURL: "nats://alice:p@ss@nats.example:4222",
			msg:     `parse "nats://alice:p@ss@nats.example:4222": invalid port`,
			secrets: []string{"alice", "p@ss", "ss@"},
		},
		{
			name:    "quote_and_space_in_password",
			natsURL: `nats://alice:p"s s@nats.example:4222`,
			msg:     `dial "nats://alice:p"s s@nats.example:4222" failed`,
			secrets: []string{`p"s s`, "s s@", `s@nats`},
		},
		{
			name:    "slash_in_password",
			natsURL: "nats://alice:pa/ss@nats.example:4222",
			msg:     "dial nats://alice:pa/ss@nats.example:4222 failed",
			secrets: []string{"pa/ss", "ss@"},
		},
		{
			name:    "percent_encoded_password_seen_decoded",
			natsURL: "nats://alice:p%40ss@nats.example:4222",
			msg:     "server said: bad password p@ss",
			secrets: []string{"p@ss"},
		},
		{
			name:    "second_url_in_list",
			natsURL: "nats://a.example:4222, tls://bob:hunter2@b.example:4443",
			msg:     "hunter2 rejected",
			secrets: []string{"hunter2"},
		},
		{
			name:    "creds_jwt_and_seed",
			natsURL: "nats://nats.example:4222",
			msg:     "creds " + userJWT + " " + string(seed),
			secrets: []string{userJWT, string(seed)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newCredentialRedactor(tt.natsURL, creds).redact(tt.msg)
			for _, s := range tt.secrets {
				require.NotContains(t, got, s)
			}
			require.Contains(t, got, redacted)
		})
	}

	t.Run("pattern_fallback_does_not_leak_tail", func(t *testing.T) {
		// Without known values, the URL pattern must still cover everything up to the last @.
		got := credentialRedactor{}.redact(`parse "nats://alice:p@ss@nats.example:4222": invalid port`)
		require.NotContains(t, got, "alice")
		require.NotContains(t, got, "ss@")
		require.Contains(t, got, "nats://"+redacted+"@nats.example:4222")
	})

	t.Run("no_credentials_untouched", func(t *testing.T) {
		msg := "dial tcp 10.0.0.1:4222: connection refused"
		require.Equal(t, msg, newCredentialRedactor("nats://nats.example:4222", nil).redact(msg))
	})
}

func TestCredentialRedactor_QuotedURLs(t *testing.T) {
	// url.Error and other %q formatting escape quotes and backslashes but leave spaces as is.
	const natsURL = `nats://alice:p"s\s x@nats.example:4222`
	quoted := fmt.Sprintf("parse %q: invalid port", natsURL)

	t.Run("known_values_match_escaped_form", func(t *testing.T) {
		got := newCredentialRedactor(natsURL, nil).redact(quoted)
		for _, s := range []string{"alice", `p\"s`, `s\\s`, "s x", "x@"} {
			require.NotContains(t, got, s)
		}
		require.Contains(t, got, "nats.example:4222")
	})

	t.Run("pattern_covers_quoted_url_with_space", func(t *testing.T) {
		got := credentialRedactor{}.redact(quoted)
		for _, s := range []string{"alice", `p\"s`, "s x", "x@"} {
			require.NotContains(t, got, s)
		}
		require.Contains(t, got, `"nats://`+redacted+`@nats.example:4222"`)
	})
}
