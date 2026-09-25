package dynamics

// The integration step and parameters of the Rössler system.
var (
	RosslerDT float32 = 0.005
	RosslerA  float32 = 0.2
	RosslerB  float32 = 0.2
	RosslerC  float32 = 5.7
)

// rosslerDeriv is the vector field — single definition shared with flowreg.
func rosslerDeriv(x, y, z float32) (float32, float32, float32) {
	return -y - z, x + RosslerA*y, RosslerB + z*(x-RosslerC)
}
