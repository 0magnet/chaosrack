package wtaudio

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"testing"
	"time"
)

// The 14-day cap is a browser rule, not a preference: a certificate
// pinned by hash in serverCertificateHashes is rejected outright if its
// validity period is longer. Exceeding it would fail only in a browser,
// which nothing here can run, so it is checked at the source.
func TestCertFitsTheHashPinningWindow(t *testing.T) {
	c, err := GenerateCert("localhost")
	if err != nil {
		t.Fatal(err)
	}
	if span := c.NotAfter.Sub(c.NotBefore); span > 14*24*time.Hour {
		t.Errorf("certificate is valid for %v, over the 14-day cap the browser imposes", span)
	}
	if !c.NotBefore.Before(time.Now()) {
		t.Error("certificate is not valid yet; a viewer whose clock is a minute slow would refuse it")
	}
	if !c.NotAfter.After(time.Now()) {
		t.Error("certificate has already expired")
	}
	// The parsed certificate must agree with the struct — the hash is
	// taken over the DER, so a mismatch would mean pinning something
	// other than what is served.
	parsed, err := x509.ParseCertificate(c.DER)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.NotAfter.Equal(c.NotAfter.Truncate(time.Second)) && parsed.NotAfter.Sub(c.NotAfter).Abs() > time.Second {
		t.Errorf("parsed NotAfter %v != reported %v", parsed.NotAfter, c.NotAfter)
	}
}

// A browser only accepts an ECDSA P-256 certificate for hash pinning; RSA
// is refused however short its validity.
func TestCertIsECDSAP256(t *testing.T) {
	c, err := GenerateCert("localhost")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(c.DER)
	if err != nil {
		t.Fatal(err)
	}
	pub, ok := parsed.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("public key is %T, want *ecdsa.PublicKey", parsed.PublicKey)
	}
	if pub.Curve != elliptic.P256() {
		t.Errorf("curve is %v, want P-256", pub.Curve.Params().Name)
	}
}

// The hash is the whole handshake: get it wrong and the browser refuses
// the connection with no useful diagnostic. It must be the SHA-256 of
// exactly the DER bytes served, and the base64 must decode back to it.
func TestCertHashIsOverTheServedDER(t *testing.T) {
	c, err := GenerateCert("localhost")
	if err != nil {
		t.Fatal(err)
	}
	served := c.TLSConfig.Certificates[0].Certificate[0]
	want := sha256.Sum256(served)
	if c.SHA256 != want {
		t.Error("SHA256 is not the hash of the certificate in TLSConfig")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(c.Base64())
	if err != nil {
		t.Fatalf("Base64() does not decode: %v", err)
	}
	if len(decoded) != sha256.Size {
		t.Fatalf("Base64() decodes to %d bytes, want %d", len(decoded), sha256.Size)
	}
	for i := range want {
		if decoded[i] != want[i] {
			t.Fatalf("byte %d of the published hash is %#02x, want %#02x", i, decoded[i], want[i])
		}
	}
	if c.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Error("QUIC requires TLS 1.3; the config allows less")
	}
}

// Two runs must produce different certificates. If they did not, the
// per-run generation would be pointless and a stale hash cached by a page
// would appear to work.
func TestCertIsFreshEachRun(t *testing.T) {
	a, err := GenerateCert("localhost")
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateCert("localhost")
	if err != nil {
		t.Fatal(err)
	}
	if a.Base64() == b.Base64() {
		t.Error("two generated certificates share a fingerprint")
	}
}

// Names go in so a browser that was NOT given the hash (an operator who
// trusted the certificate instead) still validates.
func TestCertCarriesItsNames(t *testing.T) {
	c, err := GenerateCert("localhost", "127.0.0.1", "::1", "audio.example")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(c.DER)
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.VerifyHostname("localhost"); err != nil {
		t.Errorf("localhost does not verify: %v", err)
	}
	if err := parsed.VerifyHostname("audio.example"); err != nil {
		t.Errorf("audio.example does not verify: %v", err)
	}
	found := false
	for _, ip := range parsed.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			found = true
		}
	}
	if !found {
		t.Error("127.0.0.1 is not in the certificate")
	}
}
