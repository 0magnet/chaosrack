//go:build !js

package server

import (
	"bytes"
	"strings"
	"testing"
)

// The page template is shared by pkg/server and cmd/audiows, and its fields
// are resolved at RENDER time — so a field renamed on one side compiles
// perfectly and then answers every request with
//
//	template error: can't evaluate field WasmGzB64 in type interface {}
//
// which is exactly what happened when the inlined wasm started being gzipped.
// Rendering it here turns that into a failing test.
func TestRenderPageExecutesTheTemplate(t *testing.T) {
	html, err := RenderPage(PageOptions{
		Wasm:          []byte("\x00asm\x01\x00\x00\x00"), // a wasm header is enough
		WasmExecJs:    "/* wasm_exec */",
		Title:         "Go",
		OtherLink:     "index.html",
		OtherLabel:    "go",
		CanonicalPath: "index.html",
	})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	s := string(html)
	// A template that silently resolved a missing field would leave the
	// literal <no value> behind rather than failing.
	if strings.Contains(s, "<no value>") {
		t.Error("the rendered page contains <no value>: a field the template wants is not being supplied")
	}
	for _, want := range []string{"<canvas", "gocanvas", "DecompressionStream", "wasmGzB64"} {
		if !strings.Contains(s, want) {
			t.Errorf("the rendered page has no %q", want)
		}
	}
	if len(html) < 2000 {
		t.Errorf("the page is %d bytes — too small to be the app", len(html))
	}
}

// The inlined binary is gzipped before it is base64'd. If that ever silently
// became plain base64 again the pages would quadruple in size.
func TestGzipBase64Compresses(t *testing.T) {
	// Compressible input, as a wasm binary is.
	raw := bytes.Repeat([]byte("chaosrack"), 20000)
	got := gzipBase64(raw)
	if got == "" {
		t.Fatal("no output")
	}
	// base64 of the gzipped bytes must be far smaller than base64 of the raw
	// bytes, which would be 4/3 of the input.
	if plain := len(raw) * 4 / 3; len(got) > plain/10 {
		t.Errorf("gzipped+base64 is %d bytes against %d for plain base64 — it is not being compressed",
			len(got), plain)
	}
	// And it must be stable: the result is cached per binary, and a cache
	// that returned something different each call would defeat it.
	if again := gzipBase64(raw); again != got {
		t.Error("two calls returned different output")
	}
}
