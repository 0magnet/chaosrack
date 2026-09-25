package dynamics

// The integration step and parameters of the Chen system.
var (
	ChenDT float32 = 0.0005
	ChenA  float32 = 35.0
	ChenB  float32 = 3.0
	ChenC  float32 = 28.0
)

// chenDeriv is the vector field — single definition shared with flowreg.
func chenDeriv(x, y, z float32) (float32, float32, float32) {
	return ChenA * (y - x), (ChenC-ChenA)*x - x*z + ChenC*y, x*y - ChenB*z
}
