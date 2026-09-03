package attractor

var chenDT, chenA, chenB, chenC float32 = 0.0005, 35.0, 3.0, 28.0

// chenDeriv is the vector field — single definition shared with flowreg.
func chenDeriv(x, y, z float32) (float32, float32, float32) {
	return chenA * (y - x), (chenC-chenA)*x - x*z + chenC*y, x*y - chenB*z
}
