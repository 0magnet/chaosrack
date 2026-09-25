package dynamics

// The integration step and parameters of Chua's circuit.
var (
	ChuaDT    float32 = 0.005
	ChuaAlpha float32 = 15.6
	ChuaBeta  float32 = 28.0
	ChuaM0    float32 = -1.143
	ChuaM1    float32 = -0.714
)

// chuaDeriv is the vector field — single definition shared with flowreg.
// h(x) = m1*x + 0.5*(m0-m1)*(|x+1| - |x-1|) is the diode's piecewise slope.
func chuaDeriv(x, y, z float32) (float32, float32, float32) {
	abxp1 := x + 1
	if abxp1 < 0 {
		abxp1 = -abxp1
	}
	abxm1 := x - 1
	if abxm1 < 0 {
		abxm1 = -abxm1
	}
	hx := ChuaM1*x + 0.5*(ChuaM0-ChuaM1)*(abxp1-abxm1)
	return ChuaAlpha * (y - x - hx), x - y + z, -ChuaBeta * y
}
