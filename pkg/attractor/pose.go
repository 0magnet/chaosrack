package attractor

// Orientation math — pure, untagged for native testing (pose_test.go): the
// euler decomposition has gimbal branches that only a round-trip property
// test exercises reliably.

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

const twoPi = 2 * math.Pi

// wrapTwoPi folds an angle into [0, 2π) so the degree readout and knob
// pointer stay in 0..359 no matter how far a spin has wound.
func wrapTwoPi(a float32) float32 {
	for a >= twoPi {
		a -= twoPi
	}
	for a < 0 {
		a += twoPi
	}
	return a
}

// decomposeXYZ extracts the X→Y→Z euler angles from a rotation matrix built as
// RotX·RotY·RotZ (the order rebuildModelMatrix composes), so a camera-relative
// drag can be folded back into the knob angles. Handles the ±90° Y gimbal by
// folding the roll into X (Z is then indeterminate → 0), which still reproduces
// the same matrix.
func decomposeXYZ(m mgl32.Mat4) (float32, float32, float32) {
	sy := m.At(0, 2)
	if sy > 1 {
		sy = 1
	} else if sy < -1 {
		sy = -1
	}
	y := float32(math.Asin(float64(sy)))
	if math.Abs(float64(m.At(2, 2))) > 1e-4 || math.Abs(float64(m.At(1, 2))) > 1e-4 {
		return float32(math.Atan2(float64(-m.At(1, 2)), float64(m.At(2, 2)))),
			y,
			float32(math.Atan2(float64(-m.At(0, 1)), float64(m.At(0, 0))))
	}
	// Gimbal (cos Y ≈ 0): fold the remaining rotation into X, set Z = 0. At
	// the −90° pole the composite collapses to x−z (not x+z), which flips
	// the sign of the sine term — caught by TestDecomposeXYZGimbal.
	if sy < 0 {
		return float32(math.Atan2(float64(-m.At(1, 0)), float64(m.At(1, 1)))), y, 0
	}
	return float32(math.Atan2(float64(m.At(1, 0)), float64(m.At(1, 1)))), y, 0
}
