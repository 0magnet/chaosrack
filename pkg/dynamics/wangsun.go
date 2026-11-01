package dynamics

// The integration step and parameters of the Wang–Sun system, a four-wing
// attractor, at the values of Wang et al., "A 3-D four-wing attractor and its
// analysis" (Braz. J. Phys. 39, 2009). Those measure chaotic
// under RK4 (λ≈0.06–0.09 to t=9300, every neighbor too); the b=−0.03, c=0.3,
// e=f=−1.5 it had before settle onto a cycle under RK4 at every step, and were
// chaotic only under forward Euler at this one.
var (
	WangSunDT float32 = 0.01
	WangSunA  float32 = 0.2
	WangSunB  float32 = -0.01
	WangSunC  float32 = 1
	WangSunD  float32 = -0.4
	WangSunE  float32 = -1
	WangSunF  float32 = -1
)

// wangSunDeriv is the vector field.
func wangSunDeriv(x, y, z float32) (float32, float32, float32) {
	return WangSunA*x + WangSunC*y*z, WangSunB*x + WangSunD*y - x*z, WangSunE*z + WangSunF*x*y
}
