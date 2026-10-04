package dynamics

// The integration step and parameters of the Wang–Sun system (Wang and Sun,
// 2010), a four-wing attractor.
var (
	WangSunDT float32 = 0.01
	WangSunA  float32 = 0.2
	WangSunB  float32 = -0.03
	WangSunC  float32 = 0.3
	WangSunD  float32 = -0.4
	WangSunE  float32 = -1.5
	WangSunF  float32 = -1.5
)

// wangSunDeriv is the vector field.
func wangSunDeriv(x, y, z float32) (float32, float32, float32) {
	return WangSunA*x + WangSunC*y*z, WangSunB*x + WangSunD*y - x*z, WangSunE*z + WangSunF*x*y
}
