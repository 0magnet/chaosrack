//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// The Lü attractor — render loop and panel registration. The vector field,
// its timestep and its initial condition live untagged in flowdata.go, so the
// flow registry and the native tools have them without this file running.

func generateLu() { integrate3D(float64(dynamics.LuDT), dynamics.LuDeriv) }

func init() {
	attractorParams["lu"] = []paramDef{
		{"lu-dt", "dt", &dynamics.LuDT, 0.005, 0.001, 0.02, 0.001},
		{"lu-a", "a", &dynamics.LuA, 36, 1, 50, 0.1},
		{"lu-b", "b", &dynamics.LuB, 3, 0.1, 10, 0.1},
		{"lu-c", "c", &dynamics.LuC, 20, 1, 40, 0.1},
	}
}
