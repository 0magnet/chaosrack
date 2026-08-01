// Command chaosrack serves the 3D strange-attractor visualizer. It mirrors
// the repo-root entrypoint; both are thin wrappers around pkg/server.
package main

import "github.com/0magnet/chaosrack/pkg/server"

func main() { server.Execute() }
