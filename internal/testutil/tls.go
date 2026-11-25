package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

// TestPKI is a throw-away certificate authority with one server and one client certificate.
// Keys are ECDSA P-256 signed with SHA-256, which keeps tests valid under FIPS 140-3 mode.
type TestPKI struct {
	// CAPEM is the PEM-encoded CA certificate.
	CAPEM []byte
	// CAPool contains only the CA certificate.
	CAPool *x509.CertPool
	// ServerTLS is a server-side TLS config presenting the server certificate.
	ServerTLS *tls.Config
	// ServerLeaf is the parsed server certificate.
	ServerLeaf *x509.Certificate
	// ClientCertPEM and ClientKeyPEM hold the PEM-encoded client certificate and key.
	ClientCertPEM []byte
	ClientKeyPEM  []byte
}

// NewTestPKI creates a CA, a server certificate valid for hosts (default 127.0.0.1 and localhost)
// and a client certificate. Certificates are valid for one hour.
func NewTestPKI(t testing.TB, hosts ...string) *TestPKI {
	t.Helper()
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1", "localhost"}
	}

	caKey := newTestKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          newSerial(t),
		Subject:               pkix.Name{CommonName: "nauth-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	serverCertPEM, serverKeyPEM := issueTestCert(t, caCert, caKey, "nauth-test-server", hosts, x509.ExtKeyUsageServerAuth)
	serverPair, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("load server key pair: %v", err)
	}
	serverLeaf, err := x509.ParseCertificate(serverPair.Certificate[0])
	if err != nil {
		t.Fatalf("parse server certificate: %v", err)
	}
	clientCertPEM, clientKeyPEM := issueTestCert(t, caCert, caKey, "nauth-test-client", nil, x509.ExtKeyUsageClientAuth)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	return &TestPKI{
		CAPEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		CAPool: pool,
		ServerTLS: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{serverPair},
			ClientCAs:    pool,
		},
		ServerLeaf:    serverLeaf,
		ClientCertPEM: clientCertPEM,
		ClientKeyPEM:  clientKeyPEM,
	}
}

// issueTestCert returns a PEM certificate and key for cn, signed by ca.
func issueTestCert(t testing.TB, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, hosts []string, usage x509.ExtKeyUsage) ([]byte, []byte) {
	t.Helper()
	key := newTestKey(t)
	template := &x509.Certificate{
		SerialNumber: newSerial(t),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create certificate %s: %v", cn, err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key %s: %v", cn, err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// newTestKey returns a new ECDSA P-256 key.
func newTestKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

// newSerial returns a random certificate serial number.
func newSerial(t testing.TB) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	return serial
}
