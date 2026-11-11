// Package metersworker holds the analyzers' wasm build and the worker script
// that loads it. Kept out of assets/gowasm so a page that does not use the
// worker does not carry it.
//
// The build is not in the repository. `make metersworker` writes it here; a
// binary built without it has an empty Wasm, which pkg/wasmfetch fills from
// the build CI publishes.
package metersworker

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

// Wasm is the compiled cmd/wasmmeters build, or empty in a binary built
// without one.
var Wasm = read("meters.wasm")

// WorkerJS is the script the Worker constructor is pointed at.
var WorkerJS = read("worker.js")
