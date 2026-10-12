//go:build !js

// Command chaosrack serves the 3D strange-attractor visualizer. It mirrors
// the repo-root entrypoint; both are thin wrappers around package commands.
package main

import "github.com/0magnet/chaosrack/commands"

func main() { commands.Execute() }
