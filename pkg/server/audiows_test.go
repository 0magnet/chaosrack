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
