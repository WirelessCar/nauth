package nats

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
)

const redacted = "[REDACTED]"

var (
	// urlUserinfo matches URL userinfo such as "user:pass@" in "nats://user:pass@host". It is greedy up to the
	// last @ before whitespace, so a password containing @, / or quotes is never partly left behind.
	urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)\S*@`)
	// quotedURLUserinfo matches userinfo in a double-quoted, Go-escaped URL such as `parse "nats://u:p s@h"`,
	// where %q leaves spaces unescaped. It runs to the last @ before the closing quote.
	quotedURLUserinfo = regexp.MustCompile(`"([A-Za-z][A-Za-z0-9+.-]*://)(?:\\.|[^"\\])*@`)
	// natsJWT matches a JWT whose header starts with `{"`, as every NATS JWT does.
	natsJWT = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	// nkeySeed matches an encoded nkey seed (58 base32 characters starting with S).
	nkeySeed = regexp.MustCompile(`\bS[A-Z2-7]{57}\b`)
)

// describeNATSConnectionError returns the cause of a failed NATS connection, or "" when err is nil.
// Known errors get a short description; x509 descriptions also keep the original error. Any other
// error is returned as is. The result is redacted because it ends up in resource status.
func describeNATSConnectionError(err error, caSource domain.NatsCASource, r credentialRedactor) string {
	if err == nil {
		return ""
	}
	original := r.redact(err.Error())

	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	switch {
	case errors.As(err, &unknownAuthority):
		return fmt.Sprintf("tls: server certificate signed by unknown authority (trusted CAs from %s): %s", caSource, original)
	case errors.As(err, &hostname):
		return fmt.Sprintf("tls: server certificate is not valid for host %s: %s", r.redact(hostname.Host), original)
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return "tls: server certificate expired or not yet valid: " + original
	case errors.Is(err, nats.ErrSecureConnWanted):
		return "tls configured but server does not offer TLS"
	case errors.Is(err, nats.ErrSecureConnRequired):
		return "server requires TLS"
	case errors.Is(err, nats.ErrAuthorization):
		return "authentication failed: authorization violation"
	case errors.Is(err, nats.ErrAuthExpired):
		return "authentication expired"
	case errors.Is(err, nats.ErrAuthRevoked):
		return "authentication revoked"
	case errors.Is(err, nats.ErrAccountAuthExpired):
		return "account authentication expired"
	case errors.Is(err, nats.ErrNoServers):
		return "no NATS servers available"
	case errors.Is(err, nats.ErrTimeout):
		return "connection attempt timed out"
	}
	return original
}

// credentialRedactor masks credentials in error text before it reaches resource status. Known values
// (URL userinfo and the user JWT and seed) are replaced exactly; patterns catch anything else that
// looks like URL userinfo, a NATS JWT or an nkey seed. The zero value applies only the patterns.
type credentialRedactor struct {
	// secrets are known credential values, longest first so that no value is left partly visible.
	secrets []string
}

// newCredentialRedactor returns a redactor for the credentials in natsURL and creds.
func newCredentialRedactor(natsURL string, creds []byte) credentialRedactor {
	seen := map[string]bool{}
	var secrets []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			secrets = append(secrets, s)
		}
	}
	for _, userinfo := range urlUserinfos(natsURL) {
		add(userinfo)
		if decoded, err := url.PathUnescape(userinfo); err == nil {
			add(decoded)
		}
		if _, password, ok := strings.Cut(userinfo, ":"); ok {
			add(password)
			if decoded, err := url.PathUnescape(password); err == nil {
				add(decoded)
			}
		}
	}
	if len(creds) > 0 {
		if userJWT, err := jwt.ParseDecoratedJWT(creds); err == nil {
			add(userJWT)
		}
		if kp, err := jwt.ParseDecoratedNKey(creds); err == nil {
			if seed, err := kp.Seed(); err == nil {
				add(string(seed))
			}
		}
	}
	// Errors often show values through %q, which escapes quotes, backslashes and non-printable runes.
	for _, s := range secrets {
		if q := strconv.Quote(s); q[1:len(q)-1] != s {
			add(q[1 : len(q)-1])
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return credentialRedactor{secrets: secrets}
}

// urlUserinfos returns the raw userinfo of each URL in a comma-separated NATS URL list. The userinfo
// ends at the last @, because a password may itself contain @ or /.
func urlUserinfos(natsURL string) []string {
	var result []string
	for _, u := range strings.Split(natsURL, ",") {
		_, rest, ok := strings.Cut(strings.TrimSpace(u), "://")
		if !ok {
			continue
		}
		if at := strings.LastIndex(rest, "@"); at > 0 {
			result = append(result, rest[:at])
		}
	}
	return result
}

// redact masks known credential values, then anything shaped like a credential, in msg.
func (r credentialRedactor) redact(msg string) string {
	for _, s := range r.secrets {
		msg = strings.ReplaceAll(msg, s, redacted)
	}
	msg = quotedURLUserinfo.ReplaceAllString(msg, `"${1}`+redacted+"@")
	msg = urlUserinfo.ReplaceAllString(msg, "${1}"+redacted+"@")
	msg = natsJWT.ReplaceAllString(msg, redacted)
	return nkeySeed.ReplaceAllString(msg, redacted)
}

// redactedError wraps err so that its message is redacted while errors.Is and errors.As still see err.
type redactedError struct {
	err      error
	redactor credentialRedactor
}

func (e redactedError) Error() string { return e.redactor.redact(e.err.Error()) }

func (e redactedError) Unwrap() error { return e.err }
