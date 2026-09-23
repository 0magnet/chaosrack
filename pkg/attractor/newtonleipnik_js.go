//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// The Newton–Leipnik system — render loop and panel registration. The vector
// field, its timestep and its initial condition live untagged in flowdata.go.

func generateNewtonLeipnik() { integrate3D(float64(dynamics.NlDT), dynamics.NlDeriv) }

func init() {
	attractorParams["newtonleipnik"] = []paramDef{
		{"nl-dt", "dt", &dynamics.NlDT, 0.005, 0.001, 0.02, 0.001},
		{"nl-a", "a", &dynamics.NlA, 0.4, 0.1, 1, 0.01},
		{"nl-b", "b", &dynamics.NlB, 0.175, 0.01, 0.5, 0.01},
	}
}
