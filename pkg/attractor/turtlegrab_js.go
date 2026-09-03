//go:build js && wasm

package attractor

// Picking the figure up.
//
// Two things want the same gesture: turning the model, which every mode has
// always done with a drag, and moving a figure that has weight, which only this
// one can. Neither should need a switch thrown on the rack first, and the way
// out is the one physics sandboxes and 3-D editors settled on long ago — let
// what is UNDER THE CURSOR decide. Press on the figure and you have hold of the
// figure; press on the space around it and you are turning the view, exactly as
// before. Nothing is moded, nothing is remembered between gestures, and the
// cursor says which one you are about to get.
//
// It is only ever offered when the figure has weight. With gravity at zero
// there is nothing to pick up — the figure is wherever the camera decided — so
// a drag is a drag and the question does not arise.
//
// What the hand holds is a piece of path, not the figure: the grab is a spring
// from the point you pressed on to where the cursor is now, so the rest of the
// figure hangs off it and swings, and letting go at speed throws it. That falls
// out of applying the force at a point rather than at the center of mass, which
// is the same reason the floor can tip it over.

import (
	"syscall/js"

	"github.com/0magnet/pisano/pkg/pisano"
	"github.com/go-gl/mathgl/mgl32"
)

// turtleGrabState is the hand: which piece of path is held, and where it is
// being pulled to.
var turtleGrabState struct {
	held   bool
	point  int     // which step of the WALK, not which slot of the trail
	tx, ty float32 // where it is being pulled to, in model space
	depth  float32 // the clip depth it was picked at, so it stays in its own plane
}

// turtleGrabbable reports whether a press should take hold of the figure rather
// than turn the view.
func turtleGrabbable() bool { return selectedMode == "turtle" && physOn() }

// mvpNow is the matrix the shader is drawing with, which is the one a cursor
// has to be compared against.
func mvpNow() mgl32.Mat4 { return projMatrix.Mul4(viewMatrix).Mul4(movMatrix) }

// canvasPoint turns a client position into a position on the canvas, in the
// canvas's own pixels.
func canvasPoint(clientX, clientY float64) (x, y, w, h float32, ok bool) {
	if !canvasEl.Truthy() {
		return 0, 0, 0, 0, false
	}
	r := canvasEl.Call("getBoundingClientRect")
	rw, rh := float32(r.Get("width").Float()), float32(r.Get("height").Float())
	if rw <= 0 || rh <= 0 {
		return 0, 0, 0, 0, false
	}
	return float32(clientX) - float32(r.Get("left").Float()),
		float32(clientY) - float32(r.Get("top").Float()), rw, rh, true
}

// turtleHit finds the piece of path nearest the cursor, if any is near enough
// to have been aimed at. It returns the step of the walk, so the answer stays
// meaningful as the trail scrolls out from under it.
//
// stride samples the path rather than reading every point of it: for deciding
// what the cursor is over, one point in eight of a line that is drawn a pixel
// wide is the same answer for an eighth of the work.
func turtleHit(clientX, clientY float64, stride int) (step int, depth float32, ok bool) {
	t := turtleState
	if t == nil || len(t.pts) < 2 {
		return 0, 0, false
	}
	cx, cy, w, h, ok := canvasPoint(clientX, clientY)
	if !ok {
		return 0, 0, false
	}
	mvp := mvpNow()
	n := min(len(t.pts), min(steps, t.trailLen()))
	base := len(t.pts) - n

	// What is on screen is the LINE, not the points it is bent at. A figure of
	// fifty-odd lattice points fills the canvas, so its vertices are a couple of
	// hundred pixels apart and pressing on the middle of a straight run is
	// nowhere near any of them. Distance to the segment is the only measure that
	// matches what the cursor is actually pointing at.
	const reach = 26 // how far off the line still counts as pressing on it, in px
	best := float32(reach * reach)
	found := -1
	var bestDepth float32
	var px, py, pz float32
	have := false
	for i := 0; i < n; i += stride {
		x, y, z := t.body.place(t, t.pts[base+i])
		c := mvp.Mul4x1(mgl32.Vec4{x, y, z, 1})
		if c.W() <= 0 { // behind the eye
			have = false
			continue
		}
		sx := (c.X()/c.W()*0.5 + 0.5) * w
		sy := (1 - (c.Y()/c.W()*0.5 + 0.5)) * h
		if have {
			if d, at := segDist(px, py, sx, sy, cx, cy); d < best {
				best, bestDepth = d, pz+(c.Z()/c.W()-pz)*at
				found = base + i
				if at < 0.5 {
					found = base + i - stride
				}
			}
		}
		px, py, pz, have = sx, sy, c.Z()/c.W(), true
	}
	if found < 0 {
		return 0, 0, false
	}
	return t.dropped + found, bestDepth, true
}

// turtleGrabBegin takes hold, if the press was on the figure. It reports
// whether it did, so the caller knows not to start turning the view.
func turtleGrabBegin(clientX, clientY float64) bool {
	if !turtleGrabbable() {
		return false
	}
	step, depth, ok := turtleHit(clientX, clientY, 2)
	if !ok {
		return false
	}
	turtleGrabState.held = true
	turtleGrabState.point = step
	turtleGrabState.depth = depth
	turtleGrabMove(clientX, clientY)
	setCanvasCursor("grabbing")
	return true
}

// turtleGrabMove aims the hand at wherever the cursor has got to, on the plane
// the figure was picked at — so dragging moves it across the screen rather than
// pushing it away from or toward the eye.
func turtleGrabMove(clientX, clientY float64) {
	if !turtleGrabState.held {
		return
	}
	cx, cy, w, h, ok := canvasPoint(clientX, clientY)
	if !ok {
		return
	}
	inv := mvpNow().Inv()
	ndc := mgl32.Vec4{cx/w*2 - 1, 1 - cy/h*2, turtleGrabState.depth, 1}
	p := inv.Mul4x1(ndc)
	if p.W() == 0 {
		return
	}
	turtleGrabState.tx, turtleGrabState.ty = p.X()/p.W(), p.Y()/p.W()
}

func turtleGrabEnd() {
	if !turtleGrabState.held {
		return
	}
	turtleGrabState.held = false
	setCanvasCursor("")
}

// turtleGrabHover is the affordance: over the figure the cursor becomes a hand,
// so the two things a drag can do are visible before committing to one.
func turtleGrabHover(clientX, clientY float64) {
	if turtleGrabState.held || dragging {
		return
	}
	if !turtleGrabbable() {
		setCanvasCursor("")
		return
	}
	if _, _, ok := turtleHit(clientX, clientY, 8); ok {
		setCanvasCursor("grab")
	} else {
		setCanvasCursor("")
	}
}

// segDist is the squared distance from a point to a segment, and how far along
// the segment the nearest place is.
func segDist(ax, ay, bx, by, px, py float32) (d2, at float32) {
	vx, vy := bx-ax, by-ay
	wx, wy := px-ax, py-ay
	den := vx*vx + vy*vy
	if den <= 0 {
		return wx*wx + wy*wy, 0
	}
	at = (wx*vx + wy*vy) / den
	at = clamp01(at)
	dx, dy := wx-at*vx, wy-at*vy
	return dx*dx + dy*dy, at
}

func setCanvasCursor(name string) {
	if canvasEl.Truthy() {
		canvasEl.Get("style").Set("cursor", name)
	}
}

// grabForce is what the hand does to the body: a spring to the cursor, damped,
// applied at the point being held.
//
// Scaled by the body's mass so a long figure is no harder to drag than a short
// one — an arm that got weaker as the walk got longer would be a worse
// interface, and there is no claim here that a lattice point weighs anything in
// particular.
func (b *turtleBody) grabForce(t *turtleWalk, pts []pisano.Pt3, mass float32) (fx, fy, torque float32) {
	if !turtleGrabState.held {
		return 0, 0, 0
	}
	// Which slot of the trail that step is in now. The walk scrolls out from
	// under the hand, so a piece of path held long enough ages off the tail;
	// holding the oldest that is left keeps hold of the end of the figure
	// rather than dropping it without warning.
	i := turtleGrabState.point - t.dropped
	base := len(t.pts) - len(pts)
	if i < base {
		i = base
	}
	if i >= len(t.pts) {
		i = len(t.pts) - 1
	}
	if i < 0 {
		return 0, 0, 0
	}
	x, y, _ := b.place(t, t.pts[i])
	rx, ry := x-b.x, y-b.y // the arm, from the center of mass to the hand
	pvx := b.vx - b.spin*ry
	pvy := b.vy + b.spin*rx

	const pull = 90 // how hard the hand pulls, per unit out of place
	const ease = 14 // and how much of the swinging it takes back out
	fx = mass * (pull*(turtleGrabState.tx-x) - ease*pvx)
	fy = mass * (pull*(turtleGrabState.ty-y) - ease*pvy)
	return fx, fy, rx*fy - ry*fx
}

// turtleSpinDrag is a drag on the RIM while the figure has weight: it spins the
// figure itself rather than rolling the camera.
//
// Without weight a drag is a trackball — near the middle it tilts the model
// about the screen axes, near the rim it rolls it about the screen normal — and
// both of those should survive the physics switch. Tilt cannot: the body is a
// rigid body in the PLANE of the screen, and tipping that plane out of the
// screen is what made gravity pull sideways in the first place. So off the
// figure, near the middle, a drag still turns the view as it always did, and
// physics is only meaningful while that view is square on.
//
// Roll is different. Rolling keeps the figure in the screen plane, so it stays
// compatible — and there is a better thing to point it at than the camera. With
// weight on, the figure is an object you can already pick up and throw, so a
// twist at the rim turns THE OBJECT: it spins under your hand, keeps the spin
// when you let go, and the floor takes it back through friction. The gesture is
// the one that was already there, aimed at the thing that now has mass.
var turtleSpinDrag bool

// turtleTiltDrag is a drag near the MIDDLE while the figure has weight: it
// turns the figure in three dimensions rather than turning the camera.
//
// This is the part that makes it a three-dimensional object trapped in a
// two-dimensional world rather than a flat shape that happens to be drawn from
// a 3-D walk. The physics reads every point through the figure's own
// orientation, so what falls, what the floor holds up and what the walls stop
// is the SILHOUETTE of however it is turned. Tilt a long walk edge-on and it
// lands as a short one, because from there it is a short one.
//
// Turning the CAMERA instead would tip the plane the physics lives in out of
// the screen, which is what made gravity appear to pull sideways. The camera
// stays square on to the room; the object turns inside it.
var turtleTiltDrag bool

// turtleTiltBegin claims a middle drag for the figure.
func turtleTiltBegin() bool {
	if !turtleGrabbable() || turtleState == nil {
		return false
	}
	turtleTiltDrag = true
	setCanvasCursor("grabbing")
	return true
}

// turtleTiltMove turns the figure about the screen axes.
func turtleTiltMove(dax, day float32) {
	if !turtleTiltDrag || turtleState == nil {
		return
	}
	turtleState.body.turtleTiltBy(dax, day)
}

func turtleTiltEnd() {
	if !turtleTiltDrag {
		return
	}
	turtleTiltDrag = false
	setCanvasCursor("")
}

// turtleSpinBegin claims a rim drag for the figure, if there is a figure with
// weight to claim it. Reports whether it did.
func turtleSpinBegin() bool {
	if !turtleGrabbable() || turtleState == nil {
		return false
	}
	turtleSpinDrag = true
	turtleSpinMs = 0
	setCanvasCursor("grabbing")
	return true
}

// turtleSpinBy turns the held figure by d radians and gives it the matching
// angular velocity, so releasing mid-twist lets go of a spinning object rather
// than a stopped one.
//
// The velocity is d over the REAL time since the last move, not over a nominal
// frame. Pointer moves do not arrive on the frame clock — they arrive when the
// hand moves — so dividing by a fixed 16 ms turned a leisurely twist into a
// hard flick and the figure carried on spinning long after it was let go.
func turtleSpinBy(d float32) {
	if !turtleSpinDrag || turtleState == nil {
		return
	}
	b := &turtleState.body
	b.ang += d
	now := js.Global().Get("performance").Call("now").Float()
	if turtleSpinMs > 0 {
		if dt := float32(now-turtleSpinMs) / 1000; dt > 0.001 {
			b.spin = d / dt
		}
	}
	turtleSpinMs = now
}

// turtleSpinMs is when the last twist was measured.
var turtleSpinMs float64

func turtleSpinEnd() {
	if !turtleSpinDrag {
		return
	}
	turtleSpinDrag = false
	setCanvasCursor("")
}
