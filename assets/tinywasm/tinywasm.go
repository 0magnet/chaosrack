// Package tinywasm holds the TinyGo WebAssembly build (cmd/wasm) and its
// matching wasm_exec.js runtime shim. It is a separate library from gowasm so
// an importer that only wants the smaller TinyGo build doesn't compile in the
// larger standard Go artifacts.
//
// The build is not in the repository. `make tinywasm` writes it here; a
// binary built without it has an empty Wasm, which pkg/wasmfetch fills from
// the build CI publishes.
package tinywasm

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

// Wasm is the TinyGo WebAssembly build (cmd/wasm), smaller than the standard
// Go build, or empty in a binary built without one.
var Wasm = read("chaosrack-tiny.wasm")

// WasmExec is the wasm_exec.js runtime shim for the TinyGo build.
var WasmExec = read("tinygo_wasm_exec.js")

// Has reports whether a TinyGo build is in this binary.
func Has() bool { return len(Wasm) > 0 && len(WasmExec) > 0 }

// BuiltFrom is the stamp of the sources Wasm was built from (see
// gowasm.BuiltFrom).
var BuiltFrom = read("built-from.txt")
