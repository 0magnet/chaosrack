//go:build !js

// Command chaosrack serves the 3D strange-attractor visualizer as a
// self-contained WebAssembly page. This repo-root entrypoint and
// cmd/chaosrack are identical thin wrappers around package commands, so both
// `go run github.com/0magnet/chaosrack@master` and
// `go run github.com/0magnet/chaosrack/cmd/chaosrack@master` work.
package main

import "github.com/0magnet/chaosrack/commands"

func main() { commands.Execute() }
