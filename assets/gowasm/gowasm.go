// Package gowasm holds the standard Go toolchain WebAssembly build (cmd/wasm)
// and its matching wasm_exec.js runtime shim, so an importer that only needs
// the full Go build doesn't pull in the TinyGo artifacts.
//
// The build is not in the repository. `make wasm` writes it here, and a
// binary built after that carries it; one built without it has an empty Wasm,
// which pkg/wasmfetch fills from the build CI publishes.
package gowasm

import "embed"

// Everything here, because an embed pattern that matches no file does not
// compile and the build is not always here to be matched.
//
//go:embed *
var files embed.FS

func read(name string) []byte {
	b, _ := files.ReadFile(name) //nolint:errcheck // absent is empty, which is what callers test for
	return b
}

// Wasm is the standard Go toolchain WebAssembly build (cmd/wasm), or empty in
// a binary built without one.
var Wasm = read("chaosrack.wasm")

// WasmExec is the wasm_exec.js runtime shim for the standard Go build.
var WasmExec = read("wasm_exec.js")

// BuiltFrom is the stamp of the sources Wasm was built from. The page reads
// it to ask for the wasm by build and to notice a newer one (assets/update.js).
var BuiltFrom = read("built-from.txt")

// Has reports whether a Go build is in this binary.
func Has() bool { return len(Wasm) > 0 && len(WasmExec) > 0 }
