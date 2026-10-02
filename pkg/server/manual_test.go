//go:build !js

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// /manual is the rack in its manual mode, and /manual/src.json the markdown,
// the built-in copy or the files of --manual-dir.
func TestTheManualIsTheRacksOwn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	manualRoutes(r)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		return w
	}
	if w := get("/manual"); w.Code != http.StatusFound || w.Header().Get("Location") != "/?manual" {
		t.Errorf("/manual: %d to %q, want the rack with ?manual", w.Code, w.Header().Get("Location"))
	}
	var src map[string]string
	if err := json.Unmarshal(get("/manual/src.json").Body.Bytes(), &src); err != nil || src["routing"] == "" {
		t.Errorf("src.json: %v, with routing: %v", err, src["routing"] != "")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "routing.md"), []byte("# Edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := manualSources(dir); got["routing"] != "# Edited\n" || len(got) != 1 {
		t.Errorf("--manual-dir gave %v, want the edited file alone", got)
	}
}
