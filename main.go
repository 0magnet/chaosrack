// Command wasm-stuff serves the 3D strange-attractor visualizer as a
// self-contained WebAssembly page. This repo-root entrypoint and
// cmd/wasm-stuff are identical thin wrappers around pkg/server, so both
// `go run github.com/0magnet/wasm-stuff@master` and
// `go run github.com/0magnet/wasm-stuff/cmd/wasm-stuff@master` work.
package main

import "github.com/0magnet/wasm-stuff/pkg/server"

func main() { server.Execute() }
