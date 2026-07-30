//go:build js && wasm

// Package main cmd/wasm/main.go — thin entry-point wrapper for the
// b.wasm / b-tiny.wasm binaries. All the orchestration moved to
// pkg/attractor so it can be imported as a library by other wasm
// consumers (e.g. m2/wasm/stl2's home-page renderer). This file stays
// to keep the existing Makefile target (`go build ./cmd/wasm` →
// `b.wasm`) working unchanged.
package main

import "github.com/0magnet/wasm-stuff/pkg/attractor"

func main() {
	attractor.Run()
}
