package attractor

// The Turtle's VIEW, shared by the page (turtleview_js.go, which says why a
// screw is looked at down its axis) and a native rack.

// The views, and where each sends the axis in eye space. The camera looks down
// -Z, so +Z is out of the screen toward the viewer.
var turtleViews = []struct {
	name string
	to   [3]float32
}{
	{"free", [3]float32{}},          // whatever pose the model already has
	{"end", [3]float32{0, 0, 1}},    // axis toward the viewer: the section
	{"back", [3]float32{0, 0, -1}},  // axis away: the same section, far end
	{"across", [3]float32{1, 0, 0}}, // axis left to right, advancing across
	{"up", [3]float32{0, 1, 0}},     // axis up the screen
}

const viewFree = 0

// TurtleViewTarget is where VIEW position idx sends the walk's axis in eye
// space, +Z toward the viewer; false for free, which sends it nowhere.
func TurtleViewTarget(idx int) ([3]float32, bool) {
	idx = clampIndex(idx, len(turtleViews))
	return turtleViews[idx].to, idx != viewFree
}

// Axis is the direction the walk advances along, repeated forever, as Step
// puts its points (up is up); false for a walk with no single direction.
func (r *TurtleRun) Axis() ([3]float64, bool) {
	if r.walk == nil || !r.walk.axisSet {
		return [3]float64{}, false
	}
	d := r.walk.axisDir
	return [3]float64{float64(d[0]), -float64(d[1]), float64(d[2])}, true
}
