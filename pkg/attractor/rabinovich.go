//go:build js && wasm

package attractor

// Rabinovich–Fabrikant, chaotic at α=1.1, γ=0.87 (the canonical set). The
// system is stiff and its chaotic attractor has a small basin, so it reuses
// the shared double-precision RK4 loop (integrate3D) with state kept across
// frames — in single precision the trajectory escapes and renders blank.
var rabDT, rabAlpha, rabGamma float32 = 0.001, 1.1, 0.87

func generateRabinovich() {
	al, ga := float64(rabAlpha), float64(rabGamma)
	integrate3D(float64(rabDT), func(x, y, z float64) (float64, float64, float64) {
		return y*(z-1+x*x) + ga*x, x*(3*z+1-x*x) + ga*y, -2 * z * (al + x*y)
	})
}
