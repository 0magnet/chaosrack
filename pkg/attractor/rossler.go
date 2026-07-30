package attractor

var rosslerDT, rosslerA, rosslerB, rosslerC float32 = 0.005, 0.2, 0.2, 5.7

// rosslerDeriv is the vector field — single definition shared with flowreg.
func rosslerDeriv(x, y, z float32) (float32, float32, float32) {
	return -y - z, x + rosslerA*y, rosslerB + z*(x-rosslerC)
}
