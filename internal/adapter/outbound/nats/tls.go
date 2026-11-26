package nats

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/nats-io/nats.go"
)

// buildTLSOptions returns the nats.go options for cfg and the CA source they trust.
// CAs come from the configured bundle, else defaultCAFile, else the system roots. defaultCAFile is read on
// every call so a rotated bundle needs no restart, and is used only when tlsURL is set or cfg is non-nil.
//
// A non-nil cfg always installs a TLS config, even when it is empty, so the connection requires TLS: nats.go
// rejects a server that does not offer it. A nil cfg on a plain URL with no CA file adds no options, which
// leaves TLS opt-in.
func buildTLSOptions(cfg *domain.NatsTLSConfig, defaultCAFile string, tlsURL bool, readFile func(string) ([]byte, error)) ([]nats.Option, domain.NatsCASource, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	source := domain.NatsCASourceSystem

	switch {
	case cfg != nil && len(cfg.CABundlePEM) > 0:
		pool, _, err := domain.ParseCABundle(cfg.CABundlePEM)
		if err != nil {
			return nil, "", fmt.Errorf("configured CA bundle: %w", err)
		}
		tlsConfig.RootCAs = pool
		source = domain.NatsCASourceConfigured
	case defaultCAFile != "" && (cfg != nil || tlsURL):
		bundle, err := readFile(defaultCAFile)
		if err != nil {
			return nil, "", fmt.Errorf("read default CA file %s: %w", defaultCAFile, err)
		}
		pool, _, err := domain.ParseCABundle(bundle)
		if err != nil {
			return nil, "", fmt.Errorf("default CA file %s: %w", defaultCAFile, err)
		}
		tlsConfig.RootCAs = pool
		source = domain.NatsCASourceOperatorFile
	}

	if cfg.HasClientCert() {
		cert, err := tls.X509KeyPair(cfg.ClientCertPEM, cfg.ClientKeyPEM)
		if err != nil {
			return nil, "", fmt.Errorf("TLS client certificate configuration: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	var opts []nats.Option
	if cfg != nil || source != domain.NatsCASourceSystem {
		opts = append(opts, withTLSConfig(tlsConfig))
	}
	if cfg != nil && cfg.HandshakeFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}
	return opts, source, nil
}

// withTLSConfig returns an option that sets the connection TLS config.
func withTLSConfig(tlsConfig *tls.Config) nats.Option {
	return func(o *nats.Options) error {
		o.TLSConfig = tlsConfig
		return nil
	}
}

// urlRequestsTLS reports whether any URL in a comma-separated NATS URL list uses a TLS scheme.
func urlRequestsTLS(natsURL string) bool {
	for _, u := range strings.Split(natsURL, ",") {
		u = strings.ToLower(strings.TrimSpace(u))
		if strings.HasPrefix(u, "tls://") || strings.HasPrefix(u, "wss://") {
			return true
		}
	}
	return false
}
