package dynamics

import "math"

// The integration step and parameters of the Simone system: a point relaxing
// toward s·(sin(a·y) + cos(b·z), …), each coordinate driven by the next two —
// the trigonometric map of the Simone attractor written as a flow, as in
// Shashank Tomar's strange attractors (blog.shashanktomar.com).
//
// a=5.40, b=2.40, measured under RK4 (analysis.MeasureLongRun): λ≈1.0 in
// every window to t=9300, at this step and a quarter of it, and at every
// neighbor a step of 0.1 away. The 5.51/2.49 it had before is chaotic only for
// a while: λ≈1.1 to t≈2000, then 0.14, a single loop on screen — which forward
// Euler at this step hid, being chaotic there itself.
var (
	SimoneDT float32 = 0.005
	SimoneA  float32 = 5.40
	SimoneB  float32 = 2.40
	SimoneS  float32 = 2.0
)

// simoneDeriv is the vector field.
func simoneDeriv(x, y, z float32) (float32, float32, float32) {
	a, b, s := float64(SimoneA), float64(SimoneB), float64(SimoneS)
	fx, fy, fz := float64(x), float64(y), float64(z)
	return float32(s*(math.Sin(a*fy)+math.Cos(b*fz)) - fx),
		float32(s*(math.Sin(a*fz)+math.Cos(b*fx)) - fy),
		float32(s*(math.Sin(a*fx)+math.Cos(b*fy)) - fz)
}
