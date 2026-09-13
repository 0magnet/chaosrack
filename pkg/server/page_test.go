//go:build !js

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"regexp"
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

// The payload has to survive the template, and the way it fails does not look
// like a failure.
//
// It is base64 in an element the template escapes as HTML text, so a '+' comes
// out as "&#43;". The page still renders, the test above still passes, the
// bytes are still all there — and atob rejects them in the browser, which is
// the only place anyone finds out. The same thing in the other direction cost
// about 1.4 MB a page: in a JavaScript string literal '+' became a six-byte
// unicode escape and '/' became an escaped slash, both of which JavaScript
// decodes back, so it was merely expensive rather than broken and went
// unnoticed for that reason.
//
// So: decode what the template actually wrote, and require a wasm out of it.
func TestTheRenderedPayloadStillDecodes(t *testing.T) {
	// A one-byte "wasm" is enough — this is about the encoding surviving, not
	// about the module being loadable.
	want := []byte("\x00asm\x01\x00\x00\x00")
	html, err := RenderPage(PageOptions{
		Wasm:       want,
		WasmExecJs: "/* wasm_exec */",
		Title:      "Go",
	})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	re := regexp.MustCompile(`(?s)<script type="text/plain" id="wasmgz">(.*?)</script>`)
	m := re.FindSubmatch(html)
	if m == nil {
		t.Fatal("no #wasmgz element in the rendered page: the payload is not where the boot script looks for it")
	}
	payload := strings.TrimSpace(string(m[1]))

	if i := strings.IndexAny(payload, "&\\<"); i >= 0 {
		t.Fatalf("the payload was escaped by the template at offset %d (%q): "+
			"it is no longer valid base64 and the page will fail in the browser with an atob error",
			i, payload[max(0, i-8):min(len(payload), i+8)])
	}

	packed, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("the payload is not decodable base64: %v", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatalf("the payload is not gzip: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("inflating the payload: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the payload inflated to %q, want %q", got, want)
	}
}

// The crawlable description has to come before the payload.
//
// Everything that reads this page without running it reads a bounded prefix
// and discards the rest — Google documents 2 MB, measured on the uncompressed
// bytes. With the payload in <head> the description sat at about byte
// 13,500,000 and no crawler ever reached it. The ordering is the whole fix, so
// assert it rather than trusting that nobody moves a <script> back.
func TestThePageComesBeforeItsPayload(t *testing.T) {
	html, err := RenderPage(PageOptions{
		Wasm:       bytes.Repeat([]byte("\x00asm\x01\x00\x00\x00"), 128),
		WasmExecJs: "/* wasm_exec */",
		Title:      "Go",
	})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	s := string(html)
	desc := strings.Index(s, "<h1>")
	payload := strings.Index(s, `id="wasmgz"`)
	switch {
	case desc < 0:
		t.Fatal("the page has no <h1>: there is nothing for a crawler to read")
	case payload < 0:
		t.Fatal("no #wasmgz element in the rendered page")
	case desc > payload:
		t.Errorf("the payload starts at byte %d and the description at %d — "+
			"the description is behind the payload again, which is what made it unreachable",
			payload, desc)
	}
}

// The fetched page must not ALSO carry the payload.
//
// The two forms are one switch, and the way to get it wrong is to set both: the
// page then fetches the binary, runs perfectly, and is still twelve megabytes,
// with the megabytes it ignores sitting in the document. Nothing fails, so
// nothing says so — which is how the deployed page got big in the first place.
func TestTheFetchedPageCarriesNoPayload(t *testing.T) {
	html, err := RenderPage(PageOptions{
		Wasm:       bytes.Repeat([]byte("\x00asm\x01\x00\x00\x00"), 128),
		WasmExecJs: "/* wasm_exec */",
		Title:      "Go",
		WasmURL:    "/assets/gowasm/chaosrack.wasm",
	})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	s := string(html)
	if strings.Contains(s, `id="wasmgz"`) {
		t.Error("the page names a URL to fetch and still carries the inlined payload")
	}
	if !strings.Contains(s, "instantiateStreaming(fetch(") {
		t.Error("the page does not stream-compile: instantiateStreaming(fetch(…)) is the point of fetching it")
	}
	if !strings.Contains(s, "/assets/gowasm/chaosrack.wasm") {
		t.Error("the page does not name the URL it was given")
	}
	// It is a page, not a payload: whatever a crawler is willing to read, this
	// fits inside it many times over.
	if len(html) > 200<<10 {
		t.Errorf("the fetched page is %d bytes — it should be tens of kilobytes", len(html))
	}
}
