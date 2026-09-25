package dynamics

// The integration step and parameters of the Lorenz system.
var (
	LorenzDT float32 = 0.005
	LorenzS  float32 = 10.0
	LorenzR  float32 = 28.0
	LorenzB  float32 = 2.7
)

// lorenzDeriv is the vector field — defined once, used by the render loop
// AND the flow registry (Model Out), so the equations can't drift apart.
func lorenzDeriv(x, y, z float32) (float32, float32, float32) {
	return LorenzS * (y - x), x*(LorenzR-z) - y, x*y - LorenzB*z
}
