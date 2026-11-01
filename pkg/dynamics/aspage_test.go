package dynamics

import (
	"math"
	"testing"
)

// reach is the largest side of the points' bounding box.
func reach(pts [][3]float64) float64 {
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		for i := range p {
			lo[i], hi[i] = min(lo[i], p[i]), max(hi[i], p[i])
		}
	}
	return max(hi[0]-lo[0], hi[1]-lo[1], hi[2]-lo[2])
}

// The gallery GIF of the Simone attractor was chaotic for a moment and then a
// single closed loop, because it was drawn by RK4 and the page steps it by
// float32 Euler: under RK4 it settles onto a cycle a few thousand time units
// in, under the page's Euler it never does. Drawn as the page steps it, the
// last stretch of a long run still fills the figure.
func TestAsThePageKeepsSimoneChaotic(t *testing.T) {
	o := TrajectoryOptions{Transient: 500, Duration: 9600, MaxPoints: 1 << 20, AsThePage: true}
	pts := Trajectory("simone", o)
	if len(pts) < 1000 {
		t.Fatalf("simone: %d points", len(pts))
	}
	dt, _, _ := FlowFor("simone")
	tail := pts[len(pts)-int(100/dt):] // the page's trail: 20 000 steps
	whole, last := reach(pts), reach(tail)
	t.Logf("simone: reach %.3f over the run, %.3f in its last 100 time units", whole, last)
	if last < 0.8*whole {
		t.Errorf("the last stretch reaches %.3f of a %.3f figure — it has settled onto a cycle", last, whole)
	}
}
