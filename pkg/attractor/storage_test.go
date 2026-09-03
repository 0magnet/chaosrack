package attractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every localStorage access has to go through storage_js.go, and this is the
// test that says so.
//
// The reason is not tidiness. A call into JavaScript that throws comes back
// into Go as a panic, and a panic in wasm ends the PROGRAM — the canvas keeps
// its last frame, every knob stops answering, and nothing on screen says why.
// localStorage throws in situations that are nobody's bug: the origin's quota
// is full, Safari private browsing refuses writes, a browser set to block site
// data throws on the property access itself.
//
// This was not hypothetical. Fourteen call sites wrote to localStorage
// directly, and making setItem throw and then toggling one module switch killed
// the whole app — measured, with the driver reporting "30 calls into an exited
// Go program". The helpers in storage_js.go take the throw (in the page's own
// try/catch, which is the only thing both the Go and TinyGo runtimes agree on)
// and hand back a value instead.
//
// So a new `ls.Call("setItem", ...)` anywhere else is not a style regression,
// it is a way for a full disk to take the application down, and it should fail
// here rather than in somebody's browser.
func TestLocalStorageGoesThroughTheHelpers(t *testing.T) {
	const allowed = "storage_js.go"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || name == allowed {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue // tests name the API they are testing
		}
		src, err := os.ReadFile(name) //nolint:gosec // a filename from this package's own directory
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		checked++
		for _, line := range strings.Split(string(src), "\n") {
			code := line
			if i := strings.Index(code, "//"); i >= 0 {
				code = code[:i] // a comment may discuss localStorage; only code counts
			}
			if strings.Contains(code, `"localStorage"`) ||
				strings.Contains(code, `"setItem"`) ||
				strings.Contains(code, `"getItem"`) ||
				strings.Contains(code, `"removeItem"`) {
				t.Errorf("%s reaches localStorage directly:\n\t%s\n"+
					"use lsGet/lsSet/lsRemove — a throw from a full or blocked store "+
					"is a panic, and a panic in wasm takes the whole app down", name, strings.TrimSpace(line))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source files were scanned, so this test proves nothing")
	}
}
