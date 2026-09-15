//go:build !js

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	htmpl "html/template"
	"log"
	"sync"
)

// The inlined WebAssembly is gzipped before it is base64'd.
//
// A wasm binary is about 6 MB and base64 inflates it by a third, so the
// self-contained page was 10.5 MB of which 8.4 MB was one string — a string
// the browser has to receive, parse as JavaScript source, and then atob into
// a byte array before anything can start. Gzipped first it is 1.6 MB, or
// 2.2 MB base64'd: the same binary, a quarter of the page.
//
// Every browser that can decompress it has DecompressionStream, which is what
// the page uses; see assets/index.tmpl.html for what happens on one that
// cannot.
//
// The result is cached because it is the same bytes every request and gzip at
// maximum compression is not free — the first page load would otherwise pay
// for it, and so would every one after that.
//
// It is typed htmpl.HTML so the template writes it out as it is. That is a
// deliberate escape hatch and it needs a reason, so: the value is the standard
// base64 alphabet and nothing else — [A-Za-z0-9+/=] — produced here from a
// binary compiled into this program. It is not request data and it cannot
// contain '<', so it cannot close the element it sits in. Left as a plain
// string it is escaped for whatever context the template finds it in, and both
// escapings are expensive at this size: in a JavaScript string literal every
// '+' becomes + and every '/' becomes \/, which added about 1.4 MB to the
// page, and in the element where the payload now lives every '+' becomes
// &#43;, which silently corrupts it — base64 that no longer decodes.
var (
	gzOnce  sync.Map // [*byte]htmpl.HTML, keyed by the slice's backing array
	gzMutex sync.Mutex
)

func gzipBase64(b []byte) htmpl.HTML {
	if len(b) == 0 {
		return ""
	}
	key := &b[0]
	if v, ok := gzOnce.Load(key); ok {
		return v.(htmpl.HTML)
	}
	gzMutex.Lock()
	defer gzMutex.Unlock()
	if v, ok := gzOnce.Load(key); ok {
		return v.(htmpl.HTML)
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		log.Println("server: gzip:", err)
		return ""
	}
	if _, err := zw.Write(b); err != nil {
		log.Println("server: gzip:", err)
		return ""
	}
	if err := zw.Close(); err != nil {
		log.Println("server: gzip:", err)
		return ""
	}
	// Base64 of our own gzipped asset. The alphabet is A-Za-z0-9+/= — it
	// cannot contain a single HTML metacharacter, so there is nothing here for
	// escaping to do.
	out := htmpl.HTML(base64.StdEncoding.EncodeToString(buf.Bytes())) //nolint:gosec // base64 of our own asset; the alphabet has no HTML metacharacters
	gzOnce.Store(key, out)
	return out
}
