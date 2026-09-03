package attractor

var lorenzDT, lorenzS, lorenzR, lorenzB float32 = 0.005, 10.0, 28.0, 2.7

// lorenzDeriv is the vector field — defined once, used by the render loop
// AND the flow registry (Model Out), so the equations can't drift apart.
func lorenzDeriv(x, y, z float32) (float32, float32, float32) {
	return lorenzS * (y - x), x*(lorenzR-z) - y, x*y - lorenzB*z
}
