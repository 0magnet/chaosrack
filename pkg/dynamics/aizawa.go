package dynamics

var AizawaDT, AizawaA, AizawaB, AizawaC, AizawaD, AizawaE, AizawaF float32 = 0.0052, 0.95, 0.7, 0.6, 3.5, 0.25, 0.1

// aizawaDeriv is the vector field — single definition shared with flowreg.
func aizawaDeriv(x, y, z float32) (float32, float32, float32) {
	return (z-AizawaB)*x - AizawaD*y,
		AizawaD*x + (z-AizawaB)*y,
		AizawaC + AizawaA*z - (z*z*z)/3 - (x*x+y*y)*(1+AizawaE*z) + AizawaF*z*x*x*x
}
