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

// BuiltFrom is the stamp of the sources Wasm was built from. The page reads
// it to ask for the wasm by build and to notice a newer one (assets/update.js).
//
//go:embed built-from.txt
var BuiltFrom []byte
