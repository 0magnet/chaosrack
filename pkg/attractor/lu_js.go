//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// The Lü attractor — render loop and panel registration. The vector field,
// its timestep and its initial condition live untagged in flowdata.go, so the
// flow registry and the native tools have them without this file running.

func generateLu() { integrate3D(float64(dynamics.LuDT), dynamics.LuDeriv) }
