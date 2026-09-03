package attractor

var burkeDT, burkeS, burkeV float32 = 0.005, 10.0, 4.272

// burkeShawDeriv is the vector field — single definition shared with flowreg.
func burkeShawDeriv(x, y, z float32) (float32, float32, float32) {
	return -burkeS * (x + y), -y - burkeS*x*z, burkeS*x*y + burkeV
}
