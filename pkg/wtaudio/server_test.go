package wtaudio

import (
	"encoding/json"
	"net/http"
	"testing"
)

func newTestServer(t *testing.T, addr string) *Server {
	t.Helper()
	s, err := New(Config{
		Addr:       addr,
		Capture:    func(func([]float32) error) (func(), error) { return func() {}, nil },
		SampleRate: 24000,
		Logf:       func(string, ...interface{}) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The URL the page is told to connect to has to be reachable FROM THE
// PAGE. Deriving the hostname from the page's own Host header is what
// makes the remote case — the only case this transport is for — work; a
// hard-coded localhost would send a browser on another machine to itself.
func TestInfoURLFollowsThePageHost(t *testing.T) {
	s := newTestServer(t, ":8080")
	cases := []struct{ pageHost, want string }{
		{"127.0.0.1:8080", "https://127.0.0.1:8080/wt"},
		{"192.168.1.20:8080", "https://192.168.1.20:8080/wt"},
		{"box.local:9999", "https://box.local:8080/wt"}, // the UDP port is ours, not the page's
		{"box.local", "https://box.local:8080/wt"},
		{"[::1]:8080", "https://[::1]:8080/wt"},
		{"", "https://127.0.0.1:8080/wt"},
	}
	for _, c := range cases {
		if got := s.Info(c.pageHost).URL; got != c.want {
			t.Errorf("Info(%q).URL = %q, want %q", c.pageHost, got, c.want)
		}
	}
}

// The page turns this JSON straight into serverCertificateHashes, so the
// field names and the encoding are load-bearing.
func TestInfoJSONCarriesWhatTheBrowserNeeds(t *testing.T) {
	s := newTestServer(t, ":8080")
	b, err := json.Marshal(s.Info("127.0.0.1:8080"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"url", "certHash", "algorithm", "expiresAt"} {
		if _, ok := got[k]; !ok {
			t.Errorf("/wt-info JSON has no %q field: %s", k, b)
		}
	}
	if got["certHash"] != s.Cert().Base64() {
		t.Error("the published hash is not the certificate's")
	}
	if got["algorithm"] != "sha-256" {
		t.Errorf("algorithm = %v, want sha-256 (what the browser API names)", got["algorithm"])
	}
}

// The page is served over TCP and the session opened over UDP; when the
// two port numbers differ the library's default same-origin check refuses
// the connection, which reads exactly like a certificate problem. Same
// host, any port, is the right rule for one process serving both.
func TestCheckOriginAllowsTheSameHostOnAnotherPort(t *testing.T) {
	s := newTestServer(t, ":8443")
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"", "127.0.0.1:8443", true},                      // not a browser
		{"http://127.0.0.1:8080", "127.0.0.1:8443", true}, // the point of this override
		{"http://127.0.0.1:8443", "127.0.0.1:8443", true}, // same port
		{"http://localhost:8080", "localhost:8443", true}, // by name
		{"https://evil.example", "127.0.0.1:8443", false}, // a page on another host
		{"http://127.0.0.1.evil.example", "127.0.0.1:8443", false},
	}
	for _, c := range cases {
		r := &http.Request{Host: c.host, Header: http.Header{}}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := s.checkOrigin(r); got != c.want {
			t.Errorf("Origin %q to Host %q: allowed=%v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

func TestNewRequiresACapture(t *testing.T) {
	if _, err := New(Config{Addr: ":8080"}); err == nil {
		t.Error("a server with no audio source was accepted")
	}
	if _, err := New(Config{Addr: "not-an-address", Capture: func(func([]float32) error) (func(), error) { return nil, nil }}); err == nil {
		t.Error("an unparsable address was accepted")
	}
}
