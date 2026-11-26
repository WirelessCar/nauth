package domain

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// NatsCASource names where the CA bundle trusted for a NATS connection came from.
type NatsCASource string

// CA sources, in precedence order.
const (
	NatsCASourceConfigured   NatsCASource = "configured CA bundle"
	NatsCASourceOperatorFile NatsCASource = "default CA file"
	NatsCASourceSystem       NatsCASource = "system roots"
)

var errNoPEMCertificates = errors.New("CA bundle contains no PEM certificates")

// NatsTLSConfig is the resolved TLS material for connecting to a NATS cluster.
// A nil *NatsTLSConfig means no TLS settings were supplied.
type NatsTLSConfig struct {
	// CABundlePEM is the PEM CA bundle to trust instead of the defaults. Empty means not set.
	CABundlePEM []byte
	// HandshakeFirst performs the TLS handshake before the NATS INFO exchange.
	HandshakeFirst bool
	// ClientCertPEM and ClientKeyPEM hold the mTLS client certificate. Both or neither must be set.
	ClientCertPEM []byte
	ClientKeyPEM  []byte
}

// HasClientCert reports whether a client certificate is configured.
func (c *NatsTLSConfig) HasClientCert() bool {
	return c != nil && len(c.ClientCertPEM) > 0
}

// Validate reports whether the configured material parses. Errors never include PEM content.
func (c *NatsTLSConfig) Validate() error {
	if c == nil {
		return nil
	}
	if len(c.CABundlePEM) > 0 {
		if _, _, err := ParseCABundle(c.CABundlePEM); err != nil {
			return err
		}
	}
	if (len(c.ClientCertPEM) > 0) != (len(c.ClientKeyPEM) > 0) {
		return errors.New("client certificate and key must both be set")
	}
	if len(c.ClientCertPEM) > 0 {
		if _, err := tls.X509KeyPair(c.ClientCertPEM, c.ClientKeyPEM); err != nil {
			return fmt.Errorf("invalid client certificate: %w", err)
		}
	}
	return nil
}

// ParseCABundle returns a pool of the CERTIFICATE blocks in bundle and how many it added.
func ParseCABundle(bundle []byte) (*x509.CertPool, int, error) {
	pool := x509.NewCertPool()
	count := 0
	rest := bundle
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, 0, fmt.Errorf("CA bundle certificate %d is invalid: %w", count+1, err)
		}
		pool.AddCert(cert)
		count++
	}
	if count == 0 {
		return nil, 0, errNoPEMCertificates
	}
	return pool, count, nil
}
