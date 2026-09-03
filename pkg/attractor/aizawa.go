package attractor

var aizawaDT, aizawaA, aizawaB, aizawaC, aizawaD, aizawaE, aizawaF float32 = 0.0052, 0.95, 0.7, 0.6, 3.5, 0.25, 0.1

// aizawaDeriv is the vector field — single definition shared with flowreg.
func aizawaDeriv(x, y, z float32) (float32, float32, float32) {
	return (z-aizawaB)*x - aizawaD*y,
		aizawaD*x + (z-aizawaB)*y,
		aizawaC + aizawaA*z - (z*z*z)/3 - (x*x+y*y)*(1+aizawaE*z) + aizawaF*z*x*x*x
}
