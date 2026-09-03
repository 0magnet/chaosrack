package attractor

var chuaDT, chuaAlpha, chuaBeta, chuaM0, chuaM1 float32 = 0.005, 15.6, 28.0, -1.143, -0.714

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
	hx := chuaM1*x + 0.5*(chuaM0-chuaM1)*(abxp1-abxm1)
	return chuaAlpha * (y - x - hx), x - y + z, -chuaBeta * y
}
