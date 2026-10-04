package dynamics

// The integration step and parameters of the second modified Lorenz system,
// a variant whose two wings are drawn as one rotating body.
var (
	LorenzMod2DT float32 = 0.002
	LorenzMod2A  float32 = 0.9
	LorenzMod2B  float32 = 5.0
	LorenzMod2C  float32 = 9.9
	LorenzMod2D  float32 = 1.0
)

// lorenzMod2Deriv is the vector field.
func lorenzMod2Deriv(x, y, z float32) (float32, float32, float32) {
	return -LorenzMod2A*x + y*y - z*z + LorenzMod2A*LorenzMod2C,
		x*(y-LorenzMod2B*z) + LorenzMod2D,
		-z + x*(LorenzMod2B*y+z)
}
