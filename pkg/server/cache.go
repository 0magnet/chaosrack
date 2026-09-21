//go:build !js

package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Serving the embedded assets so a browser can cache them.
//
// They were served as bare bytes: no ETag, no Last-Modified, no
// Cache-Control. A response with no validators is a response a browser is
// not allowed to cache, and for a 24 MB WebAssembly module that costs far
// more than the download — Chromium keeps a COMPILED-CODE cache for a wasm
// module fetched with instantiateStreaming, and only for a response it may
// cache. Without one the whole module is compiled again on every load, and
// a page that takes twelve seconds to come up takes twelve seconds every
// single time, including every time the URL changes.
//
// no-cache rather than a max-age: this is a development server handing out
// a binary that is rebuilt while you watch, so the browser must ask every
// time. With an ETag, asking costs a 304 and the compiled module stays put
// — never stale, and only paid for once.

// assetTag is the ETag for one embedded blob, computed once. A content hash
// rather than a build timestamp, so restarting the server does not
// invalidate a binary that has not changed.
var (
	assetTags   sync.Map // *byte (first byte address) -> string
	assetTagsMu sync.Mutex
)

// assetStart is what the assets claim as their modification time. They are
// compiled into this binary, so "when this process started" is as true as
// anything and stays stable for its lifetime.
var assetStart = time.Now()

func etagFor(blob []byte) string {
	if len(blob) == 0 {
		return `"empty"`
	}
	key := &blob[0]
	if v, ok := assetTags.Load(key); ok {
		return v.(string) //nolint:errcheck,forcetypeassert // only strings are stored
	}
	assetTagsMu.Lock()
	defer assetTagsMu.Unlock()
	if v, ok := assetTags.Load(key); ok {
		return v.(string) //nolint:errcheck,forcetypeassert // only strings are stored
	}
	sum := sha256.Sum256(blob)
	tag := `"` + hex.EncodeToString(sum[:16]) + `"`
	assetTags.Store(key, tag)
	return tag
}

// serveAsset writes an embedded blob with the validators a cache needs.
//
// http.ServeContent does the conditional request and the range handling; all
// it wants from us is the type, the ETag and a reader.
func serveAsset(c *gin.Context, contentType string, blob []byte) {
	h := c.Writer.Header()
	h.Set("Content-Type", contentType)
	h.Set("ETag", etagFor(blob))
	h.Set("Cache-Control", "no-cache")
	http.ServeContent(c.Writer, c.Request, "", assetStart, bytes.NewReader(blob))
}
