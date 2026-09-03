//go:build !js

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
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
var (
	gzOnce  sync.Map // [*byte]string, keyed by the slice's backing array
	gzMutex sync.Mutex
)

func gzipBase64(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	key := &b[0]
	if v, ok := gzOnce.Load(key); ok {
		return v.(string)
	}
	gzMutex.Lock()
	defer gzMutex.Unlock()
	if v, ok := gzOnce.Load(key); ok {
		return v.(string)
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
	out := base64.StdEncoding.EncodeToString(buf.Bytes())
	gzOnce.Store(key, out)
	return out
}
