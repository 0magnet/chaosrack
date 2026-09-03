// Package assets embeds the shared single-page-app template. The compiled
// WebAssembly builds and their wasm_exec.js runtimes live in separate
// sub-packages so importers pull in only the runtime they serve:
//
//	assets/gowasm    — standard Go build   (chaosrack.wasm      + wasm_exec.js)
//	assets/tinywasm  — TinyGo build        (chaosrack-tiny.wasm + tinygo_wasm_exec.js)
//
// Rebuild the embedded artifacts with `make wasms`.
package assets

import _ "embed"

// IndexTemplate is the html/template source for the single-page app; it
// inlines a wasm build (base64) and its wasm_exec.js so the served page is a
// standalone file. See the server's PageData for the fields it expects.
//
//go:embed index.tmpl.html
var IndexTemplate string
