//go:build js && wasm

package attractor

// Sprott case A — the conservative Nosé–Hoover oscillator, completing the
// A–S set (cases B–S live in sprottdata.go / sprottcases.go). Needs the
// double-precision RK4 loop: single-precision Euler damps it into a clean
// loop instead of filling its chaotic sea.

var sprottADT float32 = 0.01

func generateSprottA() {
	integrate3D(float64(sprottADT), func(x, y, z float64) (float64, float64, float64) {
		return y, -x + y*z, 1 - y*y
	})
}

func init() {
	attractorParams["sprotta"] = []paramDef{
		{"sprotta-dt", "dt", &sprottADT, 0.0005, 0.0001, 0.02, 0.0001},
	}
	attractorInitCond["sprotta"] = [3]float32{0, 5, 0}
}
