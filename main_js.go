//go:build js && wasm

// The page: this repo built for js/wasm is the rack, and chaosrack typed in
// its terminals is the same command the host binary is.
package main

import (
	"github.com/0magnet/chaosrack/commands"
	"github.com/0magnet/chaosrack/pkg/attractor"
)

func main() {
	attractor.Command = commands.Shell
	attractor.Run()
}
