//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// The Newton–Leipnik system — render loop and panel registration. The vector
// field, its timestep and its initial condition live untagged in flowdata.go.

func generateNewtonLeipnik() { integrate3D(float64(dynamics.NlDT), dynamics.NlDeriv) }
