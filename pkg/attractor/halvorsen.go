package attractor

var halvorsenDT, halvorsenA float32 = 0.003, 1.4

// halvorsenDeriv is the vector field — single definition shared with flowreg.
func halvorsenDeriv(x, y, z float32) (float32, float32, float32) {
	return -halvorsenA*x - 4*y - 4*z - y*y,
		-halvorsenA*y - 4*z - 4*x - z*z,
		-halvorsenA*z - 4*x - 4*y - x*x
}
