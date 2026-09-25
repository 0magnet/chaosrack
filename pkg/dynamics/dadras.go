package dynamics

// The integration step and parameters of the Dadras system.
var (
	DadrasDT float32 = 0.005
	DadrasP  float32 = 3.0
	DadrasQ  float32 = 2.7
	DadrasR  float32 = 1.7
	DadrasS  float32 = 2.0
	DadrasE  float32 = 9.0
)

// dadrasDeriv is the vector field — single definition shared with flowreg.
func dadrasDeriv(x, y, z float32) (float32, float32, float32) {
	return y - DadrasP*x + DadrasQ*y*z, DadrasR*y - x*z + z, DadrasS*x*y - DadrasE*z
}
