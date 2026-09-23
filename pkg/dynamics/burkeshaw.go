package dynamics

var BurkeDT, BurkeS, BurkeV float32 = 0.005, 10.0, 4.272

// burkeShawDeriv is the vector field — single definition shared with flowreg.
func burkeShawDeriv(x, y, z float32) (float32, float32, float32) {
	return -BurkeS * (x + y), -y - BurkeS*x*z, BurkeS*x*y + BurkeV
}
