//go:build !js

package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"github.com/0magnet/chaosrack/pkg/pairing"
	"github.com/0magnet/chaosrack/pkg/racklink"
	"github.com/0magnet/chaosrack/pkg/wtaudio"
)

// pairingFor turns pairing on as a server bound to the LAN would, in a
// directory of the test's own.
func pairingFor(t *testing.T) *pairState {
	t.Helper()
	wasBind, wasDir := bindAddr, pairDir
	bindAddr, pairDir = "192.0.2.1", t.TempDir()
	t.Cleanup(func() { bindAddr, pairDir, pairs = wasBind, wasDir, nil })
	if err := preparePairing(); err != nil {
		t.Fatal(err)
	}
	if pairs == nil {
		t.Fatal("a server bound to the LAN did not pair")
	}
	return pairs
}

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func proofOf(t *testing.T, q url.Values) pairing.Proof {
	t.Helper()
	p, err := pairing.ParseProof(q)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// On loopback nothing pairs, unless asked to with --pair.
func TestALoopbackServerPairsNothing(t *testing.T) {
	was, wasHere := bindAddr, pairHere
	t.Cleanup(func() { bindAddr, pairHere, pairs = was, wasHere, nil })
	for _, b := range []string{"", "127.0.0.1", "localhost", "[::1]:8080"} {
		bindAddr = b
		if pairingWanted() {
			t.Errorf("--bind %q pairs", b)
		}
	}
	for _, b := range []string{"0.0.0.0", ":8080", "192.168.1.5"} {
		bindAddr = b
		if !pairingWanted() {
			t.Errorf("--bind %q does not pair", b)
		}
	}
	pairHere = true
	for _, b := range []string{"", "127.0.0.1", "localhost"} {
		bindAddr = b
		if !pairingWanted() {
			t.Errorf("--bind %q --pair does not pair", b)
		}
		if !boundToLoopback() {
			t.Errorf("--bind %q is not taken for loopback", b)
		}
		if a := reachableAddrs(); len(a) != 1 || (a[0] != "127.0.0.1" && a[0] != "localhost") {
			t.Errorf("--bind %q --pair links to %v", b, a)
		}
	}
}

// A key is let in once it is paired, and paired only with the secret; a
// proof opens one session, once.
func TestPairingAdmitsKeysItKnows(t *testing.T) {
	p := pairingFor(t)
	sk, now := newKey(t), time.Now()

	if err := p.admit(proofOf(t, pairing.Sign(sk, "/wt", now)), "/wt", now); err == nil {
		t.Fatal("an unpaired key was let in")
	}
	wrong := pairing.Sign(sk, "/wt", now.Add(time.Second))
	wrong.Set(pairing.QPair, "not-the-secret")
	if err := p.admit(proofOf(t, wrong), "/wt", now); err == nil {
		t.Fatal("a key paired with the wrong secret")
	}
	q := pairing.Sign(sk, "/wt", now.Add(2*time.Second))
	q.Set(pairing.QPair, p.secret)
	if err := p.admit(proofOf(t, q), "/wt", now); err != nil {
		t.Fatalf("pairing with the secret: %v", err)
	}
	if err := p.admit(proofOf(t, q), "/wt", now); err == nil {
		t.Error("a proof was used twice")
	}
	later := pairing.Sign(sk, "/rack/page", now.Add(3*time.Second))
	if err := p.admit(proofOf(t, later), "/rack/page", now); err != nil {
		t.Errorf("a paired key, later: %v", err)
	}
	if err := p.admit(proofOf(t, pairing.Sign(sk, "/wt", now.Add(4*time.Second))), "/rack/page", now); err == nil {
		t.Error("a proof for /wt opened /rack/page")
	}

	// Kept: the next run knows the key, in full.
	b, err := os.ReadFile(filepath.Join(p.dir, pairedFile))
	if err != nil {
		t.Fatal(err)
	}
	key := proofOf(t, q).KeyHex()
	if !strings.HasPrefix(string(b), key+" ") || len(key) != 64 {
		t.Errorf("the paired list reads %q", b)
	}
	pairs = nil
	if err := preparePairing(); err != nil || !pairs.keys[key] {
		t.Errorf("the next run does not know the key (%v)", err)
	}
	if pairs.secret == p.secret {
		t.Error("the pair secret outlived its run")
	}
}

// The certificate is kept across runs, a next one is made a week before
// the current one ends, and it takes over a day before.
func TestTheCertificateIsKeptAndRenewed(t *testing.T) {
	p := pairingFor(t)
	first := p.cur.Base64()
	if p.next != nil {
		t.Error("a fresh certificate already has a successor")
	}
	pairs = nil
	if err := preparePairing(); err != nil || pairs.cur.Base64() != first {
		t.Fatalf("the next run serves %s, this one %s (%v)", pairs.cur.Base64(), first, err)
	}

	p = pairs
	end := p.cur.NotAfter
	if changed, err := p.rotate(end.Add(-6 * 24 * time.Hour)); err != nil || changed || p.next == nil {
		t.Fatalf("six days before the end: changed %v, next %v (%v)", changed, p.next != nil, err)
	}
	if got := p.certs(); len(got) != 2 || got[0] != first {
		t.Errorf("the hashes to pin are %v", got)
	}
	next := p.next.Base64()
	// The next certificate is a real one, made now; its end is moved here
	// to stand for one made six days ago.
	p.next.NotAfter = end.Add(7 * 24 * time.Hour)
	if changed, err := p.rotate(end.Add(-12 * time.Hour)); err != nil || !changed || p.cur.Base64() != next {
		t.Errorf("half a day before the end: changed %v, serving %s, want %s (%v)", changed, p.cur.Base64(), next, err)
	}
}

// pairServer is a WebTransport listener with the server's rule and its
// pairing: the hello, and a rack page end that answers one line.
func pairServer(t *testing.T) (*wtaudio.Server, string) {
	t.Helper()
	p := pairingFor(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := wtaudio.New(wtaudio.Config{Addr: pc.LocalAddr().String(), Cert: p.cur, Allow: allowWT, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	srv.Handle(pairing.PathHello, serveHello)
	srv.Handle(racklink.PathPage, func(sess *webtransport.Session, _ *http.Request) {
		str, err := sess.AcceptStream(sess.Context())
		if err != nil {
			return
		}
		_, _ = str.Write([]byte("page\n")) //nolint:errcheck // the test reads it or fails
		<-sess.Context().Done()
	})
	go func() { _ = srv.Serve(pc) }()     //nolint:errcheck // until Close
	t.Cleanup(func() { _ = srv.Close() }) //nolint:errcheck // teardown
	return srv, "https://" + pc.LocalAddr().String()
}

// dialPinned opens a session at url as the Pages build would: from its
// origin, trusting the one certificate whose hash it was given.
func dialPinned(ctx context.Context, srv *wtaudio.Server, url string) (*webtransport.Session, int, error) {
	want := srv.Cert().SHA256
	tr := &webtransport.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // replaced by the fingerprint check
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{http3.NextProtoH3},
		VerifyConnection: func(s tls.ConnectionState) error {
			if len(s.PeerCertificates) == 0 || sha256.Sum256(s.PeerCertificates[0].Raw) != want {
				return errors.New("not the pinned certificate")
			}
			return nil
		},
	}}
	h := http.Header{"Origin": []string{"https://chaosrack.magnetosphere.net"}}
	rsp, sess, err := tr.Dial(ctx, url, h)
	if rsp != nil {
		return sess, rsp.StatusCode, err
	}
	return sess, 0, err
}

// A browser on another machine: refused without a key, paired by the hello
// with the secret, and let onto the rack with its key from then on.
func TestAPairedBrowserReachesTheRack(t *testing.T) {
	srv, base := pairServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sk := newKey(t)

	if _, code, _ := dialPinned(ctx, srv, base+racklink.PathPage); code != http.StatusForbidden { //nolint:errcheck,bodyclose // a refused CONNECT has none to close
		t.Errorf("a page from another origin with no key: %d, want 403", code)
	}
	unpaired := base + racklink.PathPage + "?" + pairing.Sign(sk, racklink.PathPage, time.Now()).Encode()
	if _, code, _ := dialPinned(ctx, srv, unpaired); code != http.StatusForbidden { //nolint:errcheck,bodyclose // likewise
		t.Errorf("an unpaired key: %d, want 403", code)
	}

	q := pairing.Sign(sk, pairing.PathHello, time.Now())
	q.Set(pairing.QPair, pairs.secret)
	sess, code, err := dialPinned(ctx, srv, base+pairing.PathHello+"?"+q.Encode()) //nolint:bodyclose // a session, closed below
	if err != nil || code != http.StatusOK {
		t.Fatalf("the hello: %d, %v", code, err)
	}
	str, err := sess.AcceptUniStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var h pairing.Hello
	if err := json.NewDecoder(str).Decode(&h); err != nil {
		t.Fatal(err)
	}
	_ = sess.CloseWithError(0, "") //nolint:errcheck // read
	if h.V != pairing.Version || h.Key != proofOf(t, q).KeyHex() || len(h.Certs) == 0 || h.Certs[0] != srv.Cert().Base64() {
		t.Errorf("the hello: %+v", h)
	}

	paired := base + racklink.PathPage + "?" + pairing.Sign(sk, racklink.PathPage, time.Now()).Encode()
	sess, code, err = dialPinned(ctx, srv, paired) //nolint:bodyclose // a session, closed below
	if err != nil || code != http.StatusOK {
		t.Fatalf("the paired key on the rack: %d, %v", code, err)
	}
	s, err := sess.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A stream is announced by its first bytes; a page writes its answers on it.
	if _, err := s.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := s.Read(buf); err != nil || string(buf) != "page\n" {
		t.Errorf("the rack's end said %q (%v)", buf, err)
	}
	_ = sess.CloseWithError(0, "") //nolint:errcheck // done

	if _, code, _ := dialPinned(ctx, srv, paired); code != http.StatusForbidden { //nolint:errcheck,bodyclose // refused
		t.Errorf("the same proof again: %d, want 403", code)
	}
}
