//go:build js && wasm

package attractor

// Aiming the camera down the figure's own axis.
//
// These paths propagate along a line: one pass through the period moves the
// whole thing by a rotation about that line together with a slide along it, a
// screw. Which means there is a right way to look at one, and it is not a
// random pose — it is end-on, the way you look at a screw or a length of pipe
// to see its section, or side-on to watch it advance.
//
// The default stays as it was. This is a view to choose, not a correction: a
// figure tumbled at some angle is often the prettier picture, and nothing here
// fires unless asked.
//
// The axis is exact. pisano's AxisDir reads it off the classification as an
// integer vector — no fitting to the points on screen, nothing to converge, and
// no dependence on how much of the trail has been drawn.

import (
	"github.com/go-gl/mathgl/mgl32"
)

// The views, and where each sends the axis in eye space. The camera looks down
// -Z, so +Z is out of the screen toward the viewer.
var turtleViews = []struct {
	name string
	to   mgl32.Vec3
}{
	{"free", mgl32.Vec3{}},          // whatever pose the model already has
	{"end", mgl32.Vec3{0, 0, 1}},    // axis toward the viewer: the section
	{"back", mgl32.Vec3{0, 0, -1}},  // axis away: the same section, far end
	{"across", mgl32.Vec3{1, 0, 0}}, // axis left to right, advancing across
	{"up", mgl32.Vec3{0, 1, 0}},     // axis up the screen
}

const viewFree = 0

func turtleViewNames() []string {
	return turtleNames(len(turtleViews), func(i int) string { return turtleViews[i].name })
}

// turtleViewApplied remembers what the pose was last set for, so that the view
// is imposed when it changes and not on every frame. Overwriting the pose every
// frame would take the drag away: the figure would snap back the instant it was
// let go, and the mode would read as the model being stuck.
var turtleViewApplied struct {
	idx int
	key turtleKey
	set bool

	// Auto-rotate is put away while a view is held and given back on the way
	// out. A spin and an aimed pose are the same setting arguing with itself:
	// the pose is set once, the spin turns away from it on the next frame, and
	// the view reads as having done nothing at all.
	spinSaved bool
	spinWas   bool
}

// applyTurtleView points the model so the figure's axis lands where the chosen
// view wants it. Called each frame; does nothing unless the view or the figure
// has changed since it last acted.
func applyTurtleView(t *turtleWalk) {
	idx := clampIndex(int(turtleViewF), len(turtleViews))
	if idx == viewFree {
		// Leaving the mode does not restore the old pose. It was replaced, not
		// hidden, and putting back a pose from before a different figure was on
		// screen would be a surprise rather than a courtesy.
		if turtleViewApplied.spinSaved {
			setAutoRotate(turtleViewApplied.spinWas)
			turtleViewApplied.spinSaved = false
		}
		turtleViewApplied.set = false
		return
	}
	if !t.axisSet {
		return // a figure with no single direction: nothing to aim down
	}
	if turtleViewApplied.set && turtleViewApplied.idx == idx && turtleViewApplied.key == t.key {
		return
	}

	// Y is negated, because the geometry is. The trail is uploaded as
	// (x, -y, z) — screen Y runs down where the lattice Y runs up — so an axis
	// taken from the lattice has to be flipped the same way or it describes a
	// mirrored figure. Getting this wrong does not look like a small error: it
	// sends the figure to a different view entirely, and end-on came out as a
	// side view.
	from := mgl32.Vec3{t.axisDir[0], -t.axisDir[1], t.axisDir[2]}
	if from.Len() == 0 {
		return
	}
	from = from.Normalize()

	// The whole orientation goes in the drag matrix, and the knob angles are
	// zeroed, because the model matrix is dragMatrix·Rx·Ry·Rz — putting it in
	// the outermost term means the answer does not have to be decomposed into
	// three Euler angles that then have to compose back to it.
	// The pose has to survive the frame after it is set.
	if !turtleViewApplied.spinSaved {
		turtleViewApplied.spinWas = autoRotate
		turtleViewApplied.spinSaved = true
	}
	clearAutoRotateFlag()

	dragMatrix = mgl32.QuatBetweenVectors(from, turtleViews[idx].to).Mat4()
	angleX, angleY, angleZ = 0, 0, 0
	rebuildModelMatrix()
	zeroRotationSliders()
	updateRotKnobs()

	turtleViewApplied.idx = idx
	turtleViewApplied.key = t.key
	turtleViewApplied.set = true
}
