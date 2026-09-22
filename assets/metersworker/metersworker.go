// Package metersworker embeds the analyzers' wasm build and the worker script
// that loads it. Kept out of assets/gowasm so a page that does not use the
// worker does not carry it.
//
// Rebuild with `make metersworker`.
package metersworker

import _ "embed"

// Wasm is the compiled cmd/wasmmeters build.
//
//go:embed meters.wasm
var Wasm []byte

// WorkerJS is the script the Worker constructor is pointed at.
//
//go:embed worker.js
var WorkerJS []byte
