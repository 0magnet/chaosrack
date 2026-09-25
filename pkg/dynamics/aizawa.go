package dynamics

// The integration step and parameters of the Aizawa attractor.
var (
	AizawaDT float32 = 0.0052
	AizawaA  float32 = 0.95
	AizawaB  float32 = 0.7
	AizawaC  float32 = 0.6
	AizawaD  float32 = 3.5
	AizawaE  float32 = 0.25
	AizawaF  float32 = 0.1
)

// aizawaDeriv is the vector field — single definition shared with flowreg.
func aizawaDeriv(x, y, z float32) (float32, float32, float32) {
	return (z-AizawaB)*x - AizawaD*y,
		AizawaD*x + (z-AizawaB)*y,
		AizawaC + AizawaA*z - (z*z*z)/3 - (x*x+y*y)*(1+AizawaE*z) + AizawaF*z*x*x*x
}
