//go:build js && wasm

// Command wasmsplit-control is one of the three builds behind the renderer /
// control-plane split prototype. See pkg/splitwasm.
package main

import "github.com/0magnet/chaosrack/pkg/splitwasm"

func main() { splitwasm.RunControl() }
