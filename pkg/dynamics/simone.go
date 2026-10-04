package dynamics

import "math"

// The integration step and parameters of the Simone system: a point relaxing
// toward s·(sin(a·y) + cos(b·z), …), each coordinate driven by the next two —
// the trigonometric map of the Simone attractor written as a flow, as in
// Shashank Tomar's strange attractors (blog.shashanktomar.com). b is 2.49
// rather than the 4.84 shown there, which settles to a cycle.
var (
	SimoneDT float32 = 0.005
	SimoneA  float32 = 5.51
	SimoneB  float32 = 2.49
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
