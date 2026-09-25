package dynamics

// The integration step and parameters of the Burke–Shaw system.
var (
	BurkeDT float32 = 0.005
	BurkeS  float32 = 10.0
	BurkeV  float32 = 4.272
)

// burkeShawDeriv is the vector field — single definition shared with flowreg.
func burkeShawDeriv(x, y, z float32) (float32, float32, float32) {
	return -BurkeS * (x + y), -y - BurkeS*x*z, BurkeS*x*y + BurkeV
}
