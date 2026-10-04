package dynamics

// The integration step and parameters of the Arneodo system (Arneodo,
// Coullet and Tresser, 1981): a third-order jerk equation, x‴ the cubic
// feedback of x, written as three first-order equations.
var (
	ArneodoDT float32 = 0.005
	ArneodoA  float32 = -5.5
	ArneodoB  float32 = 3.5
	ArneodoD  float32 = -1.0
)

// arneodoDeriv is the vector field.
func arneodoDeriv(x, y, z float32) (float32, float32, float32) {
	return y, z, -ArneodoA*x - ArneodoB*y - z + ArneodoD*x*x*x
}
