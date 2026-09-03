// Package gowasm embeds the standard Go toolchain WebAssembly build (cmd/wasm)
// and its matching wasm_exec.js runtime shim, so an importer that only needs
// the full Go build doesn't pull in the TinyGo artifacts. Rebuild with
// `make wasms`.
package gowasm

import _ "embed"

// Wasm is the standard Go toolchain WebAssembly build (cmd/wasm).
//
//go:embed chaosrack.wasm
var Wasm []byte

// WasmExec is the wasm_exec.js runtime shim for the standard Go build.
//
//go:embed wasm_exec.js
var WasmExec []byte
