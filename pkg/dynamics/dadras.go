package dynamics

var DadrasDT, DadrasP, DadrasQ, DadrasR, DadrasS, DadrasE float32 = 0.005, 3.0, 2.7, 1.7, 2.0, 9.0

// dadrasDeriv is the vector field — single definition shared with flowreg.
func dadrasDeriv(x, y, z float32) (float32, float32, float32) {
	return y - DadrasP*x + DadrasQ*y*z, DadrasR*y - x*z + z, DadrasS*x*y - DadrasE*z
}
