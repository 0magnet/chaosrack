package attractor

var dadrasDT, dadrasP, dadrasQ, dadrasR, dadrasS, dadrasE float32 = 0.005, 3.0, 2.7, 1.7, 2.0, 9.0

// dadrasDeriv is the vector field — single definition shared with flowreg.
func dadrasDeriv(x, y, z float32) (float32, float32, float32) {
	return y - dadrasP*x + dadrasQ*y*z, dadrasR*y - x*z + z, dadrasS*x*y - dadrasE*z
}
