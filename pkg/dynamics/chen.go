package dynamics

var ChenDT, ChenA, ChenB, ChenC float32 = 0.0005, 35.0, 3.0, 28.0

// chenDeriv is the vector field — single definition shared with flowreg.
func chenDeriv(x, y, z float32) (float32, float32, float32) {
	return ChenA * (y - x), (ChenC-ChenA)*x - x*z + ChenC*y, x*y - ChenB*z
}
