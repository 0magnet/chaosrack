//go:build js && wasm

// Package viewer displays a file from the shared filesystem in a window.
//
// It renders through an <img> with a blob URL rather than putting the markup
// in the page. That matters for SVG: an SVG carries its own stylesheet, and
// inlined into the document it would inherit the page's CSS and resolve its own
// media queries against the page rather than against itself. Loaded as an image
// it is a separate document, which is what it was written as.
package viewer

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall/js"

	"github.com/0magnet/afero"
)

// Pane shows one file.
type Pane struct {
	fs   afero.Fs
	path string

	el  js.Value
	url string
}

// New reads path from fs when the pane is mounted.
func New(fs afero.Fs, path string) *Pane { return &Pane{fs: fs, path: path} }

// mimeOf guesses from the extension. Only the handful this is ever pointed at
// need to be right; anything else is offered as plain text.
func mimeOf(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml", true
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	}
	return "text/plain", false
}

// Mount renders the file.
func (p *Pane) Mount(el js.Value) error {
	data, err := afero.ReadFile(p.fs, p.path)
	if err != nil {
		return fmt.Errorf("viewer: %w", err)
	}
	p.el = el
	mime, isImage := mimeOf(p.path)

	style := el.Get("style")
	style.Set("width", "100%")
	style.Set("height", "100%")
	style.Set("margin", "0")
	style.Set("background", "#1b1f27")

	if !isImage {
		pre := js.Global().Get("document").Call("createElement", "pre")
		pre.Set("textContent", string(data))
		ps := pre.Get("style")
		ps.Set("margin", "0")
		ps.Set("padding", "10px")
		ps.Set("color", "#d3d7cf")
		ps.Set("font", "12px/1.45 ui-monospace, SFMono-Regular, Menlo, monospace")
		ps.Set("overflow", "auto")
		ps.Set("height", "100%")
		ps.Set("boxSizing", "border-box")
		el.Call("appendChild", pre)
		return nil
	}

	// Copy into a JS Uint8Array, wrap in a Blob, and hand the image a URL for
	// it. The blob has to be revoked on close or the bytes stay alive for as
	// long as the page does.
	buf := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buf, data)
	parts := js.Global().Get("Array").New(1)
	parts.SetIndex(0, buf)
	blob := js.Global().Get("Blob").New(parts, map[string]any{"type": mime})
	p.url = js.Global().Get("URL").Call("createObjectURL", blob).String()

	img := js.Global().Get("document").Call("createElement", "img")
	img.Set("src", p.url)
	img.Set("alt", p.path)
	is := img.Get("style")
	is.Set("display", "block")
	is.Set("width", "100%")
	is.Set("height", "100%")
	// contain, not cover: a design cropped to fill the window is a different
	// design.
	is.Set("objectFit", "contain")
	is.Set("padding", "8px")
	is.Set("boxSizing", "border-box")
	el.Call("appendChild", img)
	return nil
}

// Close revokes the blob URL.
func (p *Pane) Close() {
	if p.url != "" {
		js.Global().Get("URL").Call("revokeObjectURL", p.url)
		p.url = ""
	}
}
