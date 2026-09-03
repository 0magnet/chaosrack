// Package wtaudio serves live audio over WebTransport (HTTP/3 over QUIC)
// as an OPTIONAL alternative to the WebSocket path in cmd/audiows. The
// WebSocket remains the default and is untouched by anything here.
//
// The audio rides unreliable DATAGRAMS rather than a WebTransport stream.
// Over loopback that buys nothing — the byte rate is about 96 KB/s and
// TCP never drops a packet locally, which is why this is an option and
// not a replacement. It pays off on a remote or lossy link, where one
// lost packet holds up every byte behind it on a stream and the
// visualizer freezes and then fast-forwards. A visualizer would rather
// lose 5 ms of audio than stutter, so it takes the transport that drops.
// The framing that makes that survivable is in pkg/audiosrc/datagram.go,
// shared with the browser.
//
// The payload bytes are identical to the WebSocket path's
// (audiosrc.Float32ToBytes), so the browser has exactly one decoder.
package wtaudio

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// Capture starts an audio capture that calls write with each chunk of
// samples, and returns a function that stops it. It is called once per
// session — one capture per browser tab, matching what the WebSocket
// handler does, so one tab going away cannot take the others down.
//
// write returning an error means the session is gone; the capture should
// unwind.
type Capture func(write func([]float32) error) (stop func(), err error)

// Config configures a Server. Only Addr and Capture are required.
type Config struct {
	// Addr is the UDP address to listen on, e.g. ":8080". QUIC is UDP,
	// so this may safely be the same port number the TCP web server
	// uses — and sharing the number is worth it, because the default
	// origin check compares the page's origin against the authority the
	// WebTransport session was opened to.
	Addr string

	// Path is the endpoint the browser connects to. Defaults to "/wt".
	Path string

	// Hosts are the names and addresses to put in the generated
	// certificate. Defaults to localhost, 127.0.0.1 and ::1.
	Hosts []string

	// Capture supplies the audio, once per session.
	Capture Capture

	// SampleRate is reported by Info purely so a human can see it. The
	// wire format carries no rate — the browser is told out of band,
	// exactly as for the WebSocket — so a mismatch shows up as a pitch
	// shift, and having the server's number visible in /wt-info is the
	// quickest way to spot one.
	SampleRate int

	// Logf receives operational messages. Defaults to log.Printf.
	Logf func(format string, args ...interface{})
}

// Server is a WebTransport endpoint streaming audio datagrams.
type Server struct {
	cfg  Config
	cert *Cert
	wt   *webtransport.Server
	port string
}

// Info is the JSON a page needs before it can open a WebTransport
// session: where to connect, and the fingerprint that makes the browser
// accept a certificate no CA has ever seen.
type Info struct {
	URL string `json:"url"`
	// CertHash is the SHA-256 of the certificate's DER encoding, in
	// unpadded standard base64. It goes straight into the browser's
	// serverCertificateHashes option.
	CertHash string `json:"certHash"`
	// Algorithm names the hash so the page does not hard-code it, and so
	// this endpoint can grow another one without a flag day.
	Algorithm string `json:"algorithm"`
	// ExpiresAt is when the certificate stops working — at most 14 days
	// out, because that is the cap the hash-pinning API imposes.
	ExpiresAt time.Time `json:"expiresAt"`
	// SampleRate is informational; see Config.SampleRate.
	SampleRate int `json:"sampleRate,omitempty"`
}

// New generates the certificate and builds the server without listening,
// so the certificate hash is available to the page-serving HTTP server
// before the first request arrives.
func New(cfg Config) (*Server, error) {
	if cfg.Capture == nil {
		return nil, errors.New("wtaudio: no Capture configured")
	}
	if cfg.Path == "" {
		cfg.Path = "/wt"
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if len(cfg.Hosts) == 0 {
		cfg.Hosts = []string{"localhost", "127.0.0.1", "::1"}
	}
	_, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return nil, err
	}
	cert, err := GenerateCert(cfg.Hosts...)
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: cfg, cert: cert, port: port}
	h3 := &http3.Server{
		Addr:      cfg.Addr,
		TLSConfig: http3.ConfigureTLSConfig(cert.TLSConfig),
		// Without this the session negotiates no datagram support and
		// every SendDatagram fails — the audio path IS the datagrams.
		QUICConfig: &quic.Config{EnableDatagrams: true},
	}
	mux := http.NewServeMux()
	h3.Handler = mux
	s.wt = &webtransport.Server{
		H3: h3,
		// The page is served from the TCP port and the session opened to
		// the UDP one; when the two port numbers differ, the library's
		// default same-origin check refuses. Comparing hostnames instead
		// is right for a single-process dev harness where both ports are
		// this program's, and is the only thing that makes -wt-port
		// usable at all.
		CheckOrigin: s.checkOrigin,
	}
	mux.HandleFunc(cfg.Path, s.handleSession)
	return s, nil
}

// Cert is the generated certificate, whose Base64 the page needs.
func (s *Server) Cert() *Cert { return s.cert }

// Info describes this endpoint to a page served from pageHost (the Host
// header of the page request). The hostname is taken from the page's own
// request rather than from a flag so that a browser on another machine —
// the case this transport is actually for — gets an address it can reach
// instead of "localhost".
func (s *Server) Info(pageHost string) Info {
	host, _, err := net.SplitHostPort(pageHost)
	if err != nil {
		host = pageHost // no port in the Host header
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return Info{
		URL:        "https://" + net.JoinHostPort(host, s.port) + s.cfg.Path,
		CertHash:   s.cert.Base64(),
		Algorithm:  "sha-256",
		ExpiresAt:  s.cert.NotAfter,
		SampleRate: s.cfg.SampleRate,
	}
}

// ListenAndServe binds the UDP socket and serves until Close.
func (s *Server) ListenAndServe() error { return s.wt.ListenAndServe() }

// Serve serves on a socket the caller already bound. Config.Addr must
// still name the same port, because that is the port Info publishes.
// Exists so a test can take a port from the kernel and hand it over
// without a bind race.
func (s *Server) Serve(conn net.PacketConn) error { return s.wt.Serve(conn) }

// Close stops the server and every session on it.
func (s *Server) Close() error { return s.wt.Close() }

// checkOrigin accepts a page from the same host on any port. See the
// comment where it is installed.
func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // not a browser, or a same-origin navigation
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Hostname() == hostOnly(r.Host)
}

// hostOnly strips any port from a host[:port] string, leaving IPv6
// brackets handled by net.SplitHostPort.
func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// handleSession upgrades one CONNECT into a session, starts a capture for
// it, and streams datagrams until the browser goes away.
//
// Errors end this session only, never the server — the same rule the
// WebSocket handler follows, and the reason a capture is started per
// session rather than shared.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.wt.Upgrade(w, r)
	if err != nil {
		s.cfg.Logf("wtaudio: upgrade from %s failed: %v", r.RemoteAddr, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func() {
		_ = sess.CloseWithError(0, "") //nolint:errcheck // teardown on the way out; nothing is left to report a failure to
	}()

	sender := NewSender(sess, 0)
	stop, err := s.cfg.Capture(sender.Send)
	if err != nil {
		s.cfg.Logf("wtaudio: starting capture for %s: %v", sess.RemoteAddr(), err)
		return
	}
	defer stop()

	s.cfg.Logf("wtaudio: session from %s streaming audio datagrams", sess.RemoteAddr())
	<-sess.Context().Done()
	if sender.Shrunk > 0 {
		s.cfg.Logf("wtaudio: session from %s ended; datagram size settled at %d bytes after %d rejection(s)",
			sess.RemoteAddr(), sender.MaxDatagramSize(), sender.Shrunk)
	} else {
		s.cfg.Logf("wtaudio: session from %s ended", sess.RemoteAddr())
	}
}
