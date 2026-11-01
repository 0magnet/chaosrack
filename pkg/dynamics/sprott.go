package dynamics

// The integration step and parameters of the Sprott system.
//
// a=1.60, b=1.85, chosen by a Lyapunov-exponent parameter sweep (λ≈0.17)
// with every sampled neighbor also chaotic. That sweep stepped forward Euler,
// as the page did then, and under it the old 1.9/2.0 defaults and the
// literature's 2.07/1.79 sat in periodic windows; those two have not been
// measured under RK4. Measured again under RK4 (analysis.MeasureLongRun),
// 1.60/1.85 holds: λ≈0.1–0.2 to t=9300, at this step and a quarter of it,
// every neighbor 0.05 away too.
//
// dt=0.04 rather than 0.005, which is how much of the attractor a trail shows
// rather than which attractor it is: RK4 here agrees with a quarter of the
// step. One circuit of this system takes tens of time units, and the page's
// 20,000-point trail at 0.005 was 100 of them — two or three loops, a thin
// figure that read as a collapsed one. At 0.04 the trail is 800, and the
// swirl and the sheets folding through it are all there at once.
var (
	SprottDT float32 = 0.04
	SprottA  float32 = 1.6
	SprottB  float32 = 1.85
)

// sprottDeriv is the vector field — single definition shared with flowreg.
func sprottDeriv(x, y, z float32) (float32, float32, float32) {
	return y + SprottA*x*y + x*z, 1 - SprottB*x*x + y*z, x - x*x - y*y
}
