package attractor

import (
	"math"
	"testing"
)

// Catmull-Rom must interpolate (pass through p1 at t=0, reach p2 at t→1) and
// reproduce a straight line exactly — the properties the xy-scope beam
// smoothing depends on.
func TestCatmullRomInterpolates(t *testing.T) {
	if v := catmullRom(0, 1, 2, 3, 0); v != 1 {
		t.Errorf("t=0 should hit p1: got %v", v)
	}
	// straight line: all collinear points → exact linear interpolation
	for tt := float32(0); tt < 1; tt += 0.125 {
		got := catmullRom(0, 1, 2, 3, tt)
		want := 1 + tt
		if math.Abs(float64(got-want)) > 1e-5 {
			t.Errorf("collinear catmullRom(t=%v) = %v, want %v", tt, got, want)
		}
	}
}
