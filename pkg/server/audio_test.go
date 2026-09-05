//go:build !js

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The routing switch changes the machine's default sink, and the server binds
// every interface by default. Whoever is watching the visualizer from across the
// LAN must not be able to silence the machine it is running on.
func TestOnlyThisMachineCanSwitchTheRouting(t *testing.T) {
	for _, c := range []struct {
		remote string
		want   bool
	}{
		{"127.0.0.1:51000", true},
		{"[::1]:51000", true},
		{"192.168.1.24:51000", false},
		{"10.0.0.5:51000", false},
		{"", false},        // no address to vouch for
		{"garbage", false}, // unparsable is not loopback
	} {
		r := httptest.NewRequest(http.MethodPost, "/audio/wobbulate", nil)
		r.RemoteAddr = c.remote
		if got := fromThisMachine(r); got != c.want {
			t.Errorf("fromThisMachine(%q) = %v, want %v", c.remote, got, c.want)
		}
	}
}

// The other half of the guard. A page on another site cannot read this server's
// answers, but a request it manages to SEND still swaps the sink, which is the
// whole of the damage — so a cross-site Origin is refused outright.
func TestCrossSiteRequestsAreRefused(t *testing.T) {
	for _, c := range []struct {
		origin, host string
		want         bool
	}{
		{"", "127.0.0.1:8080", true}, // curl: no Origin at all
		{"http://127.0.0.1:8080", "127.0.0.1:8080", true},
		{"http://localhost:8080", "127.0.0.1:8080", false}, // a different origin, even here
		{"http://evil.example", "127.0.0.1:8080", false},
		{"http://127.0.0.1:9999", "127.0.0.1:8080", false},
	} {
		r := httptest.NewRequest(http.MethodPost, "/audio/wobbulate", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := sameOriginRequest(r); got != c.want {
			t.Errorf("sameOriginRequest(origin=%q host=%q) = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

// The endpoint's own refusals, end to end. Nothing here starts a capture or
// touches PulseAudio: every case is rejected before setWobbulate is reached,
// which is exactly the property being tested.
func TestWobbulateEndpointRefusals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wobbulateCtl = true
	r := gin.New()
	mountWobbulateCtl(r)

	cases := []struct {
		name        string
		remote      string
		origin      string
		contentType string
		body        string
		want        int
	}{
		{"from the LAN", "192.168.1.24:5000", "", "application/json", `{"on":true}`, http.StatusForbidden},
		{"cross-site", "127.0.0.1:5000", "http://evil.example", "application/json", `{"on":true}`, http.StatusForbidden},
		// A form post is a "simple request" that needs no preflight, so it is
		// the shape an attacking page would actually be able to send.
		{"form post", "127.0.0.1:5000", "", "application/x-www-form-urlencoded", "on=true", http.StatusUnsupportedMediaType},
		{"text/plain", "127.0.0.1:5000", "", "text/plain", `{"on":true}`, http.StatusUnsupportedMediaType},
		{"malformed json", "127.0.0.1:5000", "", "application/json", `{"on":`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/audio/wobbulate", strings.NewReader(c.body))
			req.RemoteAddr = c.remote
			req.Header.Set("Content-Type", c.contentType)
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != c.want {
				t.Errorf("status %d, want %d (body %s)", w.Code, c.want, w.Body.String())
			}
		})
	}
}

// GET is readable by anything that can reach the server: it reports, it does not
// change anything. It must answer even where the routing cannot be installed at
// all, because "not available" is the answer the page needs.
func TestWobbulateStatusIsReadable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wobbulateCtl = true
	wobbulateSink = "fvf_in"
	r := gin.New()
	mountWobbulateCtl(r)

	req := httptest.NewRequest(http.MethodGet, "/audio/wobbulate", nil)
	req.RemoteAddr = "192.168.1.24:5000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"sink":"fvf_in"`) {
		t.Errorf("status body has no sink name: %s", w.Body.String())
	}
}

// --wobbulate-ctl=false is a refusal, not a hint: the endpoints must not exist.
func TestWobbulateCtlCanBeRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wobbulateCtl = false
	defer func() { wobbulateCtl = true }()
	r := gin.New()
	mountWobbulateCtl(r)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/audio/wobbulate", strings.NewReader(`{"on":true}`))
		req.RemoteAddr = "127.0.0.1:5000"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s answered %d; with the control refused there should be no route at all", method, w.Code)
		}
	}
}

// The page is told about the switch only when every part of it is real.
func TestWobbulateCtlIsNotOfferedWithoutCapture(t *testing.T) {
	audioOn, wobbulateCtl = false, true
	if wobbulateCtlOffered() {
		t.Error("a server that is not capturing offered the routing switch")
	}
	audioOn, wobbulateCtl = true, false
	if wobbulateCtlOffered() {
		t.Error("the switch was offered after --wobbulate-ctl=false")
	}
	audioOn, wobbulateCtl = false, false
}
