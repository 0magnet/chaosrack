package dynamics

// The integration step and parameters of the Dequan Li system (Li, 2008), a
// three-scroll attractor. Stiff: Euler holds it at this step and not at three
// times it.
var (
	DequanDT float32 = 0.0001
	DequanA  float32 = 40.0
	DequanB  float32 = 1.833
	DequanC  float32 = 0.16
	DequanD  float32 = 0.65
	DequanE  float32 = 55.0
	DequanF  float32 = 20.0
)

// dequanDeriv is the vector field.
func dequanDeriv(x, y, z float32) (float32, float32, float32) {
	return DequanA*(y-x) + DequanC*x*z, DequanE*x + DequanF*y - x*z, DequanB*z + x*y - DequanD*x*x
}
