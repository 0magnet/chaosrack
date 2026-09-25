package dynamics

// a=1.60, b=1.85: chosen by a Lyapunov-exponent parameter sweep UNDER THIS
// APP'S OWN INTEGRATOR (forward Euler, dt=0.005, long transient): λ≈+0.17
// with every sampled neighbor also chaotic. The old 1.9/2.0 defaults — and
// the literature's 2.07/1.79 — both sit in periodic windows here: chaotic
// for thousands of time units, then collapsing onto a closed orbit (the
// gallery GIF caught it; chaos_test.go now guards it).
// The integration step and parameters of the Sprott system.
var (
	SprottDT float32 = 0.005
	SprottA  float32 = 1.6
	SprottB  float32 = 1.85
)

// sprottDeriv is the vector field — single definition shared with flowreg.
func sprottDeriv(x, y, z float32) (float32, float32, float32) {
	return y + SprottA*x*y + x*z, 1 - SprottB*x*x + y*z, x - x*x - y*y
}
