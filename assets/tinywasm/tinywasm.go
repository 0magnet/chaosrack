// Package tinywasm embeds the TinyGo WebAssembly build (cmd/wasm) and its
// matching wasm_exec.js runtime shim. It is a separate library from gowasm so
// an importer that only wants the smaller TinyGo build doesn't compile in the
// larger standard Go artifacts. Rebuild with `make wasms`.
package tinywasm

import _ "embed"

// Wasm is the TinyGo WebAssembly build (cmd/wasm), smaller than the standard
// Go build.
//
//go:embed b-tiny.wasm
var Wasm []byte

// WasmExec is the wasm_exec.js runtime shim for the TinyGo build.
//
//go:embed tinygo_wasm_exec.js
var WasmExec []byte

// Has reports whether a TinyGo build is embedded.
func Has() bool { return len(Wasm) > 0 && len(WasmExec) > 0 }
