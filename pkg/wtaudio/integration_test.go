//go:build !js

// Excluded from the js/wasm test run, which has no UDP sockets. The
// browser side of this transport cannot be tested at all — there is no
// WebTransport in Node — so this is as close as it gets: a real QUIC
// handshake against a real HTTP/3 server, pinning the same generated
// certificate by the same SHA-256 the page is given, and reassembling the
// datagrams with the same code the page runs.
//
// What it does NOT cover is the browser: the WebTransport constructor,
// serverCertificateHashes, and the datagrams.readable reader are all
// implemented by Chrome, and nothing here can exercise them.

package wtaudio

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"math"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"github.com/0magnet/chaosrack/pkg/audiosrc"
)

func TestServerStreamsToARealQUICClient(t *testing.T) {
	// Take a port from the kernel and hand the socket to the server, so
	// there is no window in which something else could take it.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback here: %v", err)
	}
	defer pc.Close() //nolint:errcheck // the server closes it; this is belt and braces

	chunk := samples(2400) // one PulseAudio chunk: 0.1 s at 24 kHz
	srv, err := New(Config{
		Addr:       pc.LocalAddr().String(),
		Path:       "/wt",
		SampleRate: 24000,
		Logf:       t.Logf,
		Capture: func(write func([]float32) error) (func(), error) {
			done := make(chan struct{})
			go func() {
				tick := time.NewTicker(5 * time.Millisecond)
				defer tick.Stop()
				for {
					select {
					case <-done:
						return
					case <-tick.C:
						if err := write(chunk); err != nil {
							return // session gone
						}
					}
				}
			}()
			return func() { close(done) }, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(pc) }() //nolint:errcheck // returns when Close is called below
	defer srv.Close()                 //nolint:errcheck // teardown

	info := srv.Info("127.0.0.1:1")

	// Exactly what serverCertificateHashes does in a browser: trust this
	// one certificate because its fingerprint matches, and nothing else.
	tlsConf := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // replaced by the fingerprint check below, which is stricter
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{http3.NextProtoH3},
		// VerifyConnection rather than VerifyPeerCertificate because it
		// also runs on a resumed session, which the latter does not —
		// a resumption would otherwise skip the only check there is.
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("no certificate presented")
			}
			if sha256.Sum256(state.PeerCertificates[0].Raw) != srv.Cert().SHA256 {
				return errors.New("certificate fingerprint does not match /wt-info")
			}
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tr := &webtransport.Transport{TLSClientConfig: tlsConf}
	defer tr.Close() //nolint:errcheck // teardown
	rsp, sess, err := tr.Dial(ctx, info.URL, nil)
	if err != nil {
		t.Fatalf("dialing %s: %v", info.URL, err)
	}
	if rsp.StatusCode != 200 {
		t.Fatalf("CONNECT answered %d", rsp.StatusCode)
	}
	defer sess.CloseWithError(0, "") //nolint:errcheck // teardown

	var r audiosrc.Reassembler
	var got []float32
	for got == nil {
		dg, err := sess.ReceiveDatagram(ctx)
		if err != nil {
			t.Fatalf("receiving a datagram: %v (reassembler dropped %d)", err, r.Dropped)
		}
		if len(dg) > audiosrc.DefaultMaxDatagramSize {
			t.Fatalf("a %d-byte datagram arrived, over the %d assumed", len(dg), audiosrc.DefaultMaxDatagramSize)
		}
		if payload := r.Push(dg); payload != nil {
			got = audiosrc.BytesToFloat32(payload)
		}
	}

	if len(got) != len(chunk) {
		t.Fatalf("received %d samples, want %d", len(got), len(chunk))
	}
	for i := range chunk {
		if math.Float32bits(got[i]) != math.Float32bits(chunk[i]) {
			t.Fatalf("sample %d = %v, want %v", i, got[i], chunk[i])
		}
	}
	t.Logf("one chunk of %d samples arrived over QUIC datagrams; %d incomplete messages dropped", len(got), r.Dropped)
}

// A client that does not know the fingerprint must be refused. This is
// the other half of what the generated certificate means: no CA has
// signed it, so the only thing that makes it acceptable is the hash the
// page was handed.
func TestUnpinnedClientIsRefused(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP loopback here: %v", err)
	}
	defer pc.Close() //nolint:errcheck // teardown

	srv, err := New(Config{
		Addr:    pc.LocalAddr().String(),
		Logf:    t.Logf,
		Capture: func(func([]float32) error) (func(), error) { return func() {}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(pc) }() //nolint:errcheck // returns when Close is called below
	defer srv.Close()                 //nolint:errcheck // teardown

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tr := &webtransport.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{http3.NextProtoH3},
	}}
	defer tr.Close() //nolint:errcheck // teardown
	if _, _, err := tr.Dial(ctx, srv.Info("127.0.0.1:1").URL, nil); err == nil {
		t.Error("a client that verified nothing was allowed to connect")
	}
}
