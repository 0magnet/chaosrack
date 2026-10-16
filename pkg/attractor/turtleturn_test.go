package attractor

import (
	"math"
	"testing"
)

// A rounded walk keeps its ends, turns each square corner on a quarter circle
// of half a step about the corner's far side, and stays in the room it has.
func TestTurtleTurnsOnArcs(t *testing.T) {
	// Right, up, left: two square corners of unit steps.
	pts := [][4]float32{{0, 0, 0, 0}, {1, 0, 0, 0.1}, {1, 1, 0, 0.2}, {0, 1, 0, 0.3}}
	var out [][4]float32
	n := roundCorners(pts, 100, func(v [4]float32) { out = append(out, v) })
	if n != len(out) || n != 2+2*turtleArcMost {
		t.Fatalf("%d points, %d handed", len(out), n)
	}
	if out[0] != pts[0] || out[n-1] != pts[3] {
		t.Errorf("the ends moved: %v … %v", out[0], out[n-1])
	}
	// The first corner's arc runs about (0.5, 0.5) at radius 0.5.
	for _, p := range out[1 : 1+turtleArcMost] {
		if r := math.Hypot(float64(p[0]-0.5), float64(p[1]-0.5)); math.Abs(r-0.5) > 1e-5 {
			t.Errorf("%v is %.4f from the turn's middle", p, r)
		}
	}
	// A trail that fills the room still turns on arcs, its oldest left off.
	long := make([][4]float32, 200)
	for i := range long {
		long[i] = [4]float32{float32((i + 1) / 2), float32(i / 2), 0, 0} // a staircase
	}
	out = out[:0]
	n = roundCorners(long, 200, func(v [4]float32) { out = append(out, v) })
	if n > 200 || n != len(out) || out[n-1] != long[199] {
		t.Errorf("%d points in a room of 200, ending at %v", n, out[n-1])
	}
	if per := (n - 2) / ((n - 2) / turtleArcLeast); per < turtleArcLeast {
		t.Errorf("arcs of %d points", per)
	}
}

// Square, the trail is handed as it stands.
func TestTurtleSquareIsTheTrail(t *testing.T) {
	pts := [][4]float32{{0, 0, 0, 0}, {1, 0, 0, 0}, {1, 1, 0, 0}}
	var out [][4]float32
	if n := drawTurtle(pts, 0, 10, func(v [4]float32) { out = append(out, v) }); n != 3 || out[1] != pts[1] {
		t.Errorf("square drew %v", out)
	}
}
