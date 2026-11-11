//go:build js && wasm

package attractor

import (
	"syscall/js"
	"testing"
)

// The crash is asked for by name and by nothing else: a page that panicked on
// a query it merely resembled would take every such link down with it.
func TestOnlyTheCrashParameterAsksForACrash(t *testing.T) {
	old := js.Global().Get("location")
	defer js.Global().Set("location", old)
	for search, want := range map[string]bool{
		"":                   false,
		"?wasm=tinygo":       false,
		"?crashes=1":         false,
		"?note=crash":        false,
		"?crash":             true,
		"?wasm=tinygo&crash": true,
	} {
		loc := js.Global().Get("Object").New()
		loc.Set("search", search)
		js.Global().Set("location", loc)
		if got := crashRequested(); got != want {
			t.Errorf("search %q: crash requested is %v, want %v", search, got, want)
		}
	}
	js.Global().Set("location", js.Undefined())
	if crashRequested() {
		t.Error("with no location at all a crash was requested")
	}
}
