package subcmds

import (
	"math"
	"testing"
)

func near(a, b mat3) bool {
	for i := range 3 {
		for j := range 3 {
			if math.Abs(a[i][j]-b[i][j]) > 1e-9 {
				return false
			}
		}
	}
	return true
}

// The angles a frame is given are the rotation it was built as.
func TestPoseAnglesInvertPoseMatrix(t *testing.T) {
	for _, a := range [][3]float64{{0.349, 0, 0}, {0.6, 0.9, 0}, {-1.2, 0.4, 2.5}, {0.3, math.Pi / 2, 0.2}} {
		if m := poseMatrix(a); !near(poseMatrix(poseAngles(m)), m) {
			t.Errorf("%v does not come back as itself", a)
		}
	}
}

// Six quarter turns, right and down by turns, come back to the start; four
// do not. And no step between frames, the wrap included, is a jump.
func TestSixTumblesCloseTheLoop(t *testing.T) {
	m := mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for k := range 6 {
		m = quarter(k, math.Pi/2).mul(m)
		if k == 3 && near(m, mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}) {
			t.Error("four turns closed the loop")
		}
	}
	if !near(m, mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}) {
		t.Fatalf("six turns end at %v", m)
	}
	base := [3]float64{0.349, 0, 0}
	v := tumbleViews(base, 6, 96)
	worst := 0.0
	for i := range v {
		a, b := poseMatrix(v[i]), poseMatrix(v[(i+1)%len(v)])
		// The angle of the rotation between consecutive frames.
		var tr float64
		for k := range 3 {
			for j := range 3 {
				tr += a[j][k] * b[j][k]
			}
		}
		worst = max(worst, math.Acos(math.Max(-1, math.Min(1, (tr-1)/2))))
	}
	// A quarter turn over 16 frames, eased, peaks under 0.2 rad a frame.
	if worst > 0.2 {
		t.Errorf("a step of %.3f rad between frames", worst)
	}
}

// The first turn is to the right as seen: the point nearest the viewer
// moves right on the screen.
func TestTheFirstTumbleTurnsRight(t *testing.T) {
	v := tumbleViews([3]float64{}, 6, 96)
	m := poseMatrix(v[8]) // halfway into the first turn
	if x := m[0][2]; x <= 0 {
		t.Errorf("the front point moved to x %.3f", x)
	}
}
