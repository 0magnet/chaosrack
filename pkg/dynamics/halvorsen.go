package dynamics

// The integration step and parameters of the Halvorsen attractor.
var (
	HalvorsenDT float32 = 0.003
	HalvorsenA  float32 = 1.4
)

// halvorsenDeriv is the vector field — single definition shared with flowreg.
func halvorsenDeriv(x, y, z float32) (float32, float32, float32) {
	return -HalvorsenA*x - 4*y - 4*z - y*y,
		-HalvorsenA*y - 4*z - 4*x - z*z,
		-HalvorsenA*z - 4*x - 4*y - x*x
}
