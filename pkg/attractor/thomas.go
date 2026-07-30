package attractor

import "math"

var thomasDT, thomasB float32 = 0.05, 0.19

// thomasDeriv is the vector field — single definition shared with flowreg.
func thomasDeriv(x, y, z float32) (float32, float32, float32) {
	return -thomasB*x + float32(math.Sin(float64(y))),
		-thomasB*y + float32(math.Sin(float64(z))),
		-thomasB*z + float32(math.Sin(float64(x)))
}
