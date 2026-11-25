package domain

import (
	"crypto/x509"
	"testing"

	"github.com/WirelessCar/nauth/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNatsTLSConfig_Nil_ShouldBeValid(t *testing.T) {
	var cfg *NatsTLSConfig
	require.NoError(t, cfg.Validate())
}

func TestNatsTLSConfig_HandshakeFirstOnly_ShouldBeValid(t *testing.T) {
	require.NoError(t, (&NatsTLSConfig{HandshakeFirst: true}).Validate())
}

func TestNatsTLSConfig_MultiCABundle_ShouldTrustAll(t *testing.T) {
	pkiA := testutil.NewTestPKI(t)
	pkiB := testutil.NewTestPKI(t)
	bundle := append(append([]byte{}, pkiA.CAPEM...), pkiB.CAPEM...)

	pool, count, err := ParseCABundle(bundle)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	for _, leaf := range []*x509.Certificate{pkiA.ServerLeaf, pkiB.ServerLeaf} {
		_, err := leaf.Verify(x509.VerifyOptions{Roots: pool})
		require.NoError(t, err)
	}
	require.NoError(t, (&NatsTLSConfig{CABundlePEM: bundle}).Validate())
}

func TestParseCABundle_NoCertificates_ShouldFail(t *testing.T) {
	_, _, err := ParseCABundle([]byte("not a pem"))
	require.EqualError(t, err, "CA bundle contains no PEM certificates")
}

func TestParseCABundle_PrivateKeyOnly_ShouldFail(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	_, _, err := ParseCABundle(pki.ClientKeyPEM)
	require.EqualError(t, err, "CA bundle contains no PEM certificates")
}

func TestNatsTLSConfig_InvalidCABundle_ShouldFail(t *testing.T) {
	err := (&NatsTLSConfig{CABundlePEM: []byte("garbage")}).Validate()
	require.ErrorContains(t, err, "CA bundle contains no PEM certificates")
	require.NotContains(t, err.Error(), "garbage")
}

func TestNatsTLSConfig_ClientCertWithoutKey_ShouldFail(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	err := (&NatsTLSConfig{ClientCertPEM: pki.ClientCertPEM}).Validate()
	require.ErrorContains(t, err, "client certificate and key must both be set")
}

func TestNatsTLSConfig_MismatchedKeyPair_ShouldFail(t *testing.T) {
	pkiA := testutil.NewTestPKI(t)
	pkiB := testutil.NewTestPKI(t)
	err := (&NatsTLSConfig{ClientCertPEM: pkiA.ClientCertPEM, ClientKeyPEM: pkiB.ClientKeyPEM}).Validate()
	require.ErrorContains(t, err, "invalid client certificate")
	require.NotContains(t, err.Error(), "PRIVATE KEY")
}

func TestNatsTLSConfig_ValidClientCert_ShouldBeValid(t *testing.T) {
	pki := testutil.NewTestPKI(t)
	require.NoError(t, (&NatsTLSConfig{ClientCertPEM: pki.ClientCertPEM, ClientKeyPEM: pki.ClientKeyPEM}).Validate())
}
