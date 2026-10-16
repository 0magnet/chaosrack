//go:build !js

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/websocket"
)

// The capture's socket answers the page it served and clients that send no
// Origin, and refuses another site's page.
func TestAudioSocketRefusesOtherSites(t *testing.T) {
	for origin, ok := range map[string]bool{
		"":                      true,
		"http://127.0.0.1:8080": true,
		"https://evil.example":  false,
		"http://127.0.0.1:9999": false,
		"http://localhost:8080": false,
	} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/ws", http.NoBody)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		err := audioHandshake(&websocket.Config{}, req)
		if (err == nil) != ok {
			t.Errorf("origin %q: %v, want allowed %v", origin, err, ok)
		}
	}
}

// The server is this machine's unless --bind says otherwise, and its UDP
// socket goes where its page does.
func TestTheServerBindsLoopbackByDefault(t *testing.T) {
	was, wasPort := bindAddr, webPort
	t.Cleanup(func() { bindAddr, webPort = was, wasPort })
	webPort = 8080
	for bind, want := range map[string]string{
		"":          "127.0.0.1:8080",
		"0.0.0.0":   "0.0.0.0:8080",
		":9000":     ":9000",
		"127.0.0.2": "127.0.0.2:8080",
	} {
		bindAddr = bind
		if got := listenAddress(); got != want {
			t.Errorf("--bind %q listens on %s, want %s", bind, got, want)
		}
	}
	bindAddr = ""
	if h := listenHost(); h != "127.0.0.1" {
		t.Errorf("the UDP socket binds %q, want 127.0.0.1", h)
	}
}
