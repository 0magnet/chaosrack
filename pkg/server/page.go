//go:build !js

package server

import (
	"bytes"
	"fmt"
	htmpl "html/template"

	"github.com/0magnet/chaosrack/assets"
)

// The single-page render, exported so that every server in this repo builds
// the page the same way.
//
// It exists because there were two: pkg/server had its own struct and
// cmd/audiows had a copy of it, both feeding assets.IndexTemplate. When the
// inlined wasm changed from plain base64 to gzipped base64 the fields were
// renamed here and the copy was left behind, so audiows started answering
// every request with
//
//	template error: can't evaluate field WasmGzB64 in type interface {}
//
// A template shared by two callers needs one definition of what feeds it.

// PageOptions is what a caller supplies to render the app page. The wasm is
// passed as BYTES: this package owns how it is encoded, so no caller can get
// the encoding wrong or fall out of step with the template.
type PageOptions struct {
	Wasm       []byte   // the WebAssembly binary to inline
	WasmExecJs htmpl.JS // its matching wasm_exec.js
	Title      string   // shown in the page's runtime switch
	OtherLink  string   // href of the other runtime's page
	OtherLabel string   // its label

	// WasmURL makes the page FETCH the binary from that URL instead of carrying
	// it. Empty inlines it, which is the single-file build. Wasm is then unused
	// and need not be supplied.
	WasmURL string

	// CanonicalPath is appended to the site root to form the page's canonical
	// URL. It is empty for every page here on purpose: /, /go/ and /tinygo/
	// differ only in which wasm runtime they boot, and three URLs serving one
	// application compete with each other in a search index rather than adding
	// anything. They all point at the root, which is the page to land on.
	CanonicalPath string
	Debug         bool

	// HostConfig is the token and endpoints the page needs to reach the host
	// agent, as a JSON object — or empty when no agent is being served, which
	// is the default and what every static host will produce.
	//
	// It is a template field rather than a string replacement on the rendered
	// bytes because the template is the one place that knows the page's shape,
	// and html/template escapes it into a script context correctly. A replace
	// would have to find </head> in a file that also mentions it in a comment.
	HostConfig htmpl.JS

	// AudioFeed names the audio transport this server wants the page to PREFER:
	// "wt" when it is capturing and has a WebTransport listener up, "ws" when it
	// is capturing without one, empty when it is not capturing. The page reads it
	// to decide where audio comes from WITHOUT being told in the URL: a server
	// that captures gets used, a server that does not is never dialed, so neither
	// silence-by-default nor an error overlay for a socket that was never going
	// to answer. "wt" is a preference and not a promise -- the page falls back to
	// the WebSocket by itself when WebTransport cannot be had.
	AudioFeed string

	// WobbulateCtl says the page may offer the FVF routing switch: this server
	// captures, it was not told to refuse the control, and the machine has the
	// pactl the routing is made of. A static host produces false, which is right
	// -- there is nothing there to switch.
	WobbulateCtl bool
}

// RenderPage returns the finished HTML for a single-runtime page.
func RenderPage(o PageOptions) ([]byte, error) {
	return renderTemplate(htmlTemplateData{
		WasmExecJs: o.WasmExecJs,
		// Not gzipped at all when the page is going to fetch the binary: the
		// template would skip the payload either way, but compressing 23 MB at
		// maximum level to then throw it away is the expensive way to do that.
		WasmGzB64: func() htmpl.HTML {
			if o.WasmURL != "" {
				return ""
			}
			return gzipBase64(o.Wasm)
		}(),
		WasmURL:       o.WasmURL,
		Title:         o.Title,
		OtherLink:     o.OtherLink,
		OtherLabel:    o.OtherLabel,
		CanonicalPath: o.CanonicalPath,
		Debug:         o.Debug,
		HostConfig:    o.HostConfig,
		AudioFeed:     o.AudioFeed,
		WobbulateCtl:  o.WobbulateCtl,
	})
}

// renderTemplate is the one place assets.IndexTemplate is executed.
func renderTemplate(d htmlTemplateData) ([]byte, error) {
	tmpl, err := htmpl.New("index").Parse(assets.IndexTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing the page template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, map[string]interface{}{"Page": d}); err != nil {
		return nil, fmt.Errorf("executing the page template: %w", err)
	}
	return out.Bytes(), nil
}
