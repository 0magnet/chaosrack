//go:build js && wasm

// Command wasmmeters is the analyzers' own wasm build, the one the Web Worker
// loads. It is the rack's DSP and nothing else — see pkg/metersworker.
package main

import "github.com/0magnet/chaosrack/pkg/metersworker"

func main() { metersworker.Run() }
