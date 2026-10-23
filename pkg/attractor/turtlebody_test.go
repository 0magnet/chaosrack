package attractor

import (
	"math"
	"testing"

	"github.com/0magnet/pisano/pkg/pisano"
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

// TestAHeldPointGoesToTheHand: picked up and carried, the point held arrives
// where the hand stops and stays there, at the frame rates displays run at and
// at a throttled tab's long frames. Pulled with the whole body's mass, a long
// figure held off center spun up and flew off at 60 Hz; with a step a frame,
// a slow frame overshot the hand.
func TestAHeldPointGoesToTheHand(t *testing.T) {
	for _, dt := range []float32{1.0 / 60, 1.0 / 144, 0.05} {
		var r TurtleRun
		r.Step([9]float32{25, 0, 1, 0, 3, 0, 0, 0, 0}, 4000, 1, 1, 1, true, nil, func(_, _, _, _ float32) {})
		w := turtleWorld{halfW: 1.88, halfH: 1.176, camDist: 1e4, grav: 3, fric: 0.6, bounce: 0.2, spin: 1, dt: dt}
		for range 600 {
			r.walk.advance(1)
			r.walk.aim(len(r.walk.pts))
			r.body.step(r.walk, r.walk.pts, w)
		}
		held := r.walk.dropped + len(r.walk.pts)/2
		x0, y0, _ := r.body.place(r.walk, r.walk.pts[held-r.walk.dropped])
		const tx1, ty1 = -0.5, 0.6
		var tx, ty float32
		w.grab = func(b *turtleBody, tw *turtleWalk, _ []pisano.Pt3, mass, inertia float32) (fx, fy, torque float32) {
			x, y, _ := b.place(tw, tw.pts[held-tw.dropped])
			return b.handForce(x, y, tx, ty, mass, inertia)
		}
		move := int(0.5 / dt)
		for s := range int(2.5 / dt) {
			f := min(float32(s)/float32(move), 1)
			tx, ty = x0+(tx1-x0)*f, y0+(ty1-y0)*f
			r.body.step(r.walk, r.walk.pts, w)
		}
		x, y, _ := r.body.place(r.walk, r.walk.pts[held-r.walk.dropped])
		if d := math.Hypot(float64(x-tx1), float64(y-ty1)); !(d < 0.1) {
			t.Errorf("at a %.4f s frame the point held is %.3f from the hand, two seconds after it stopped", dt, d)
		}
	}
}
