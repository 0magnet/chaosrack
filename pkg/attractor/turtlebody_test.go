package attractor

import (
	"math"
	"testing"
)

// The Turtle's body stays a body: whatever the walk and however long it
// extrudes, its position and spin are numbers. Friction and damping applied
// as stiff terms at a frame's step wound the spin up through the lever of a
// contact at the end of a long figure until it was not one.
func TestTheTurtlesBodyStaysFinite(t *testing.T) {
	for _, mod := range []float32{7, 10, 25, 40} {
		for _, bounce := range []float32{0, 0.2, 0.8} {
			var r TurtleRun
			r.Step([9]float32{mod, 0, 1, 0, 3, 0, 0, 0, 0}, 4000, 1, 1, 1, true, nil, func(_, _, _, _ float32) {})
			w := turtleWorld{halfW: 1.88, halfH: 1.176, camDist: 1e4, grav: 3, fric: 0.6, bounce: bounce, spin: 1, dt: 0.016}
			for f := range 900 {
				r.walk.advance(2)
				r.walk.aim(len(r.walk.pts))
				r.body.step(r.walk, r.walk.pts, w)
				b := r.body
				for _, v := range []float32{b.x, b.y, b.vx, b.vy, b.ang, b.spin} {
					if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
						t.Fatalf("mod %.0f bounce %.1f: the body is not a number at frame %d (%+v)", mod, bounce, f, b)
					}
				}
			}
		}
	}
}

// TestAGrowingWalkStaysSmallerThanItsRoom: the scale is held once the trail
// has filled, and a walk that keeps growing outgrew the room at it — longer
// than the room was tall, wedged between floor and ceiling, it could not fall
// or be dragged. With weight, every point stays inside the room.
func TestAGrowingWalkStaysSmallerThanItsRoom(t *testing.T) {
	var r TurtleRun
	r.Step([9]float32{25, 0, 1, 0, 3, 0, 0, 0, 0}, 4000, 1, 1, 1, true, nil, func(_, _, _, _ float32) {})
	w := turtleWorld{halfW: 1.88, halfH: 1.176, camDist: 1e4, grav: 3, fric: 0.6, bounce: 0.2, spin: 1, dt: 0.016}
	for range 3000 {
		r.walk.advance(8) // a long, drifting walk, all of it kept
		r.walk.aim(len(r.walk.pts))
		r.body.step(r.walk, r.walk.pts, w)
	}
	var lo, hi [2]float32
	for i, p := range r.walk.pts {
		x, y, _ := r.body.place(r.walk, p)
		if i == 0 {
			lo, hi = [2]float32{x, y}, [2]float32{x, y}
		}
		lo = [2]float32{min(lo[0], x), min(lo[1], y)}
		hi = [2]float32{max(hi[0], x), max(hi[1], y)}
	}
	if h := hi[1] - lo[1]; h > 2*w.halfH {
		t.Errorf("the figure is %.2f tall in a room %.2f tall", h, 2*w.halfH)
	}
	if wd := hi[0] - lo[0]; wd > 2*w.halfW {
		t.Errorf("the figure is %.2f wide in a room %.2f wide", wd, 2*w.halfW)
	}
}
