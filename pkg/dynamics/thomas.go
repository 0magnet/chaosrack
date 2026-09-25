package dynamics

import "math"

// The integration step and parameters of Thomas' cyclically symmetric attractor.
var (
	ThomasDT float32 = 0.05
	ThomasB  float32 = 0.185
)

// thomasDeriv is the vector field — single definition shared with flowreg.
func thomasDeriv(x, y, z float32) (float32, float32, float32) {
	return -ThomasB*x + float32(math.Sin(float64(y))),
		-ThomasB*y + float32(math.Sin(float64(z))),
		-ThomasB*z + float32(math.Sin(float64(x)))
}
