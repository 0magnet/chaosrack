//go:build js && wasm

package attractor

import "github.com/0magnet/chaosrack/pkg/dynamics"

// Sprott case A — the conservative Nosé–Hoover oscillator, completing the
// A–S set (cases B–S live in sprottdata.go / sprottcases.go). Needs the
// double-precision RK4 loop: single-precision Euler damps it into a clean
// loop instead of filling its chaotic sea. Its field, step and knob are in
// pkg/dynamics (sprotta.go), where a rack without a page integrates it too.

func generateSprottA() {
	integrate3D(float64(dynamics.SprottADT), dynamics.SprottADeriv)
}
