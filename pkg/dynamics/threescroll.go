package dynamics

// The integration step and parameters of the three-scroll unified chaotic
// system (TSUCS), the Dequan Li system without its x term in dy/dt.
var (
	ThreeScrollDT float32 = 0.0008
	ThreeScrollA  float32 = 40.0
	ThreeScrollB  float32 = 0.833
	ThreeScrollC  float32 = 0.5
	ThreeScrollD  float32 = 0.65
	ThreeScrollE  float32 = 20.0
)

// threeScrollDeriv is the vector field.
func threeScrollDeriv(x, y, z float32) (float32, float32, float32) {
	return ThreeScrollA*(y-x) + ThreeScrollC*x*z, ThreeScrollE*y - x*z, ThreeScrollB*z + x*y - ThreeScrollD*x*x
}
