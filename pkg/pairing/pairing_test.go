package pairing

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A proof opens the session it was signed for, near the time it was signed,
// and nothing else.
func TestAProofOpensItsOwnSession(t *testing.T) {
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	q := Sign(sk, "/wt", now)
	p, err := ParseProof(q)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Check("/wt", now.Add(30*time.Second)); err != nil {
		t.Errorf("a fresh proof: %v", err)
	}
	if p.Check("/rack/page", now) == nil {
		t.Error("a proof for /wt opened /rack/page")
	}
	if p.Check("/wt", now.Add(Skew+time.Second)) == nil {
		t.Error("a stale proof held")
	}
	_, other, _ := ed25519.GenerateKey(nil) //nolint:errcheck // the reader is crypto/rand
	q.Set(QKey, Sign(other, "/wt", now).Get(QKey))
	if p, err := ParseProof(q); err != nil || p.Check("/wt", now) == nil {
		t.Error("a signature held for another key")
	}
	if _, err := ParseProof(map[string][]string{}); err != ErrNoProof {
		t.Errorf("no query: %v", err)
	}
}

// The link comes back as it went.
func TestTheLinkRoundTrips(t *testing.T) {
	l := Link{WT: "192.168.1.5:8080", Certs: []string{"a+b/c", "d"}, Secret: "s3cret", Audio: "wt"}
	got, ok := ParseFragment("#" + l.Fragment())
	if !ok || got.WT != l.WT || strings.Join(got.Certs, ",") != "a+b/c,d" || got.Secret != l.Secret || got.Audio != "wt" {
		t.Errorf("%+v from %q", got, l.Fragment())
	}
	if _, ok := ParseFragment("#manual"); ok {
		t.Error("an ordinary fragment read as a pair link")
	}
}

// A page is this machine's by its names: a rebound name with a matching
// Origin is not, and a program with no Origin is only from here.
func TestBrowserPagesAreThisMachinesByName(t *testing.T) {
	for _, c := range []struct {
		origin, host, remote string
		want                 bool
	}{
		{"http://localhost:8080", "localhost:8080", "127.0.0.1:5000", true},
		{"http://127.0.0.1:8080", "127.0.0.1:8080", "127.0.0.1:5000", true},
		{"http://[::1]:8080", "[::1]:8080", "[::1]:5000", true},
		{"http://evil.example:8080", "evil.example:8080", "127.0.0.1:5000", false}, // rebound
		{"http://evil.example", "localhost:8080", "127.0.0.1:5000", false},         // cross-site
		{"http://localhost:3000", "localhost:8080", "127.0.0.1:5000", false},       // another port's page
		{"http://x.mesh.localhost:8080", "x.mesh.localhost:8080", "127.0.0.1:5000", false},
		{"https://rack.example.com", "rack.example.com", "127.0.0.1:5000", true}, // --host
		{"", "anything", "127.0.0.1:5000", true},
		{"", "anything", "192.168.1.9:5000", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/ws", http.NoBody)
		r.Host, r.RemoteAddr = c.host, c.remote
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := Browser(r, []string{"rack.example.com"}); got != c.want {
			t.Errorf("Origin %q Host %q from %s: %v, want %v", c.origin, c.host, c.remote, got, c.want)
		}
	}
}
