package wtaudio

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"time"
)

// CertValidity is how long the generated certificate is good for.
//
// This is not a taste decision. The browser API that lets a page trust a
// specific certificate by its fingerprint —
//
//	new WebTransport(url, {serverCertificateHashes: [{algorithm: "sha-256", value: …}]})
//
// — only accepts a certificate whose total validity period is at most 14
// days (and which is ECDSA over P-256; RSA is refused outright). That cap
// exists so a leaked private key for a hash-pinned certificate cannot be
// abused for long, and it is precisely why generating a fresh certificate
// on every run is the natural fit rather than a shortcut: anything
// long-lived enough to be worth keeping on disk would be rejected.
//
// 13 days rather than 14 leaves room for clock skew between the machine
// that signed it and the machine viewing the page. NotBefore is backdated
// an hour for the same reason — a browser whose clock is a few minutes
// behind must not see the certificate as not-yet-valid.
const CertValidity = 13 * 24 * time.Hour

// Cert is a self-signed certificate and the fingerprint a browser needs
// in order to accept it.
type Cert struct {
	// TLSConfig is ready to hand to an HTTP/3 server. QUIC mandates TLS
	// 1.3, so there is no plaintext option to fall back on the way http
	// falls back for the WebSocket path — a certificate is required even
	// to talk to 127.0.0.1.
	TLSConfig *tls.Config

	// SHA256 is the SHA-256 of the certificate's DER encoding: exactly
	// the bytes `openssl x509 -outform der | sha256sum` would print, and
	// exactly what goes in serverCertificateHashes.
	SHA256 [32]byte

	// DER is the certificate itself, kept so a caller can hash it a
	// different way or write it out.
	DER []byte

	NotBefore, NotAfter time.Time
}

// Base64 is the fingerprint in standard base64 without padding, which is
// the form /wt-info publishes and the page turns back into bytes. Base64
// rather than hex only because it is a third shorter in a URL or a log
// line; nothing on either side cares which.
func (c *Cert) Base64() string { return base64.RawStdEncoding.EncodeToString(c.SHA256[:]) }

// GenerateCert makes a fresh self-signed certificate for the given host
// names, valid from an hour ago until CertValidity from now.
//
// Generated per run rather than persisted: see CertValidity for why a
// long-lived certificate would not work here anyway. A fresh key each run
// also means the fingerprint changes each run, which is why the page
// fetches it from /wt-info instead of having it compiled in.
func GenerateCert(hosts ...string) (*Cert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	// Whole seconds: what the certificate can record, so NotAfter here is
	// the expiry it states.
	notBefore := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	notAfter := notBefore.Add(time.Hour + CertValidity)

	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "chaosrack-audio"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	// Names still go in even though the hash-pinned path does not check
	// them: a browser that was pointed at this endpoint WITHOUT the hash
	// (an operator who installed the certificate in a trust store, say)
	// would check them, and a certificate that only works one way is a
	// confusing thing to hand somebody.
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			continue
		}
		tmpl.DNSNames = append(tmpl.DNSNames, h)
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &Cert{
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13, // QUIC has no lower option; stated so the intent is not read as an omission
			Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		},
		SHA256:    sha256.Sum256(der),
		DER:       der,
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}, nil
}

// Save writes the certificate and its key to path as PEM, readable by this
// user alone: the key is what a pinned page trusts.
func (c *Cert) Save(path string) error {
	key, ok := c.TLSConfig.Certificates[0].PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return errors.New("wtaudio: a certificate without an ECDSA key")
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.DER})
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})...)
	return os.WriteFile(path, out, 0o600)
}

// LoadCert reads a certificate Save wrote.
func LoadCert(path string) (*Cert, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the caller's own file
	if err != nil {
		return nil, err
	}
	var der, keyDER []byte
	for {
		var blk *pem.Block
		if blk, b = pem.Decode(b); blk == nil {
			break
		}
		switch blk.Type {
		case "CERTIFICATE":
			der = blk.Bytes
		case "EC PRIVATE KEY":
			keyDER = blk.Bytes
		}
	}
	if der == nil || keyDER == nil {
		return nil, errors.New("wtaudio: " + path + " holds no certificate and key")
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return nil, err
	}
	return &Cert{
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		},
		SHA256:    sha256.Sum256(der),
		DER:       der,
		NotBefore: crt.NotBefore,
		NotAfter:  crt.NotAfter,
	}, nil
}
