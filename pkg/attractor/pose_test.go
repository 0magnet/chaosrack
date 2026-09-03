package attractor

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// decomposeXYZ round-trip: recomposing the extracted angles in the
// RotX·RotY·RotZ order (rebuildModelMatrix's convention) must reproduce the
// matrix. This is the property that catches gimbal-branch mistakes — the
// class of bug behind the rotation-inversion issue this project hit.
func TestDecomposeXYZRoundTrip(t *testing.T) {
	angles := []float32{0, 0.3, -0.7, 1.2, -1.5, 2.9, -3.0}
	for _, ax := range angles {
		for _, ay := range angles {
			for _, az := range angles {
				m := mgl32.HomogRotate3DX(ax).
					Mul4(mgl32.HomogRotate3DY(ay)).
					Mul4(mgl32.HomogRotate3DZ(az))
				gx, gy, gz := decomposeXYZ(m)
				r := mgl32.HomogRotate3DX(gx).
					Mul4(mgl32.HomogRotate3DY(gy)).
					Mul4(mgl32.HomogRotate3DZ(gz))
				for i := 0; i < 16; i++ {
					if d := math.Abs(float64(m[i] - r[i])); d > 2e-3 {
						t.Fatalf("angles (%v,%v,%v): recomposed matrix differs at [%d] by %v (got angles %v,%v,%v)",
							ax, ay, az, i, d, gx, gy, gz)
					}
				}
			}
		}
	}
}

// Gimbal poles (pitch ±90°) must not produce NaNs and must still recompose.
func TestDecomposeXYZGimbal(t *testing.T) {
	for _, ay := range []float32{float32(math.Pi / 2), float32(-math.Pi / 2)} {
		m := mgl32.HomogRotate3DX(0.2).
			Mul4(mgl32.HomogRotate3DY(ay)).
			Mul4(mgl32.HomogRotate3DZ(0.4))
		gx, gy, gz := decomposeXYZ(m)
		for _, v := range []float32{gx, gy, gz} {
			if math.IsNaN(float64(v)) {
				t.Fatalf("gimbal decompose produced NaN: %v %v %v", gx, gy, gz)
			}
		}
		r := mgl32.HomogRotate3DX(gx).
			Mul4(mgl32.HomogRotate3DY(gy)).
			Mul4(mgl32.HomogRotate3DZ(gz))
		for i := 0; i < 16; i++ {
			if d := math.Abs(float64(m[i] - r[i])); d > 5e-3 {
				t.Fatalf("gimbal recompose differs at [%d] by %v", i, d)
			}
		}
	}
}

func TestWrapTwoPi(t *testing.T) {
	for _, a := range []float32{0, 1, -1, 7, -7, 100, -100} {
		w := wrapTwoPi(a)
		if w < 0 || w >= twoPi {
			t.Errorf("wrapTwoPi(%v) = %v, outside [0, 2π)", a, w)
		}
		if math.Abs(math.Mod(float64(a-w), 2*math.Pi)) > 1e-3 &&
			math.Abs(math.Mod(float64(a-w), 2*math.Pi)-2*math.Pi) > 1e-3 {
			t.Errorf("wrapTwoPi(%v) = %v not congruent mod 2π", a, w)
		}
	}
}
