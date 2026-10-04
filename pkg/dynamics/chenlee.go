package dynamics

// The integration step and parameters of the Chen–Lee system (Chen and Lee,
// 2004), derived from the rigid body's Euler equations with feedback.
var (
	ChenLeeDT float32 = 0.002
	ChenLeeA  float32 = 5.0
	ChenLeeB  float32 = -10.0
	ChenLeeC  float32 = -0.38
	ChenLeeD  float32 = 3.0
)

// chenLeeDeriv is the vector field.
func chenLeeDeriv(x, y, z float32) (float32, float32, float32) {
	return ChenLeeA*x - y*z, ChenLeeB*y + x*z, ChenLeeC*z + x*y/ChenLeeD
}
