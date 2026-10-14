package dynamics

// Sprott's case A, the Nosé–Hoover oscillator: conservative, so it fills a
// torus-like volume rather than settling onto an attractor.

// SprottADT is its integration step.
var SprottADT float32 = 0.01

// SprottADeriv is its vector field.
func SprottADeriv(x, y, z float64) (float64, float64, float64) {
	return y, -x + y*z, 1 - y*y
}

func init() {
	InitCond["sprotta"] = [3]float32{0, 5, 0}
	registerFlow64("sprotta", &SprottADT, SprottADeriv)
}
