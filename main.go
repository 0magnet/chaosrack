//go:build !js

// Command chaosrack serves the 3D strange-attractor visualizer as a
// self-contained WebAssembly page. This repo-root entrypoint and
// cmd/chaosrack are identical thin wrappers around pkg/server, so both
// `go run github.com/0magnet/chaosrack@master` and
// `go run github.com/0magnet/chaosrack/cmd/chaosrack@master` work.
package main

import "github.com/0magnet/chaosrack/pkg/server"

func main() { server.Execute() }
