//go:build js && wasm

package attractor

import "testing"

// The plane has to be OUTSIDE the model at each end of the knob, or the ends
// stop meaning "all of it behind" and "all of it in front" — one stray fragment
// on the wrong canvas is a piece of the model floating through the panel.
func TestThePlaneClearsTheModelAtBothEnds(t *testing.T) {
	oldFrac, oldExt, oldDist := splitFrac, modelFitExtent, defaultCameraDist
	t.Cleanup(func() { splitFrac, modelFitExtent, defaultCameraDist = oldFrac, oldExt, oldDist })

	modelFitExtent, defaultCameraDist = 4, 20
	near, far := float32(-20+4), float32(-20-4) // the model spans these in view space

	splitFrac = -1
	if z := splitPlaneZ(); z < near {
		t.Errorf("at the far end the plane is at %v, which is not in front of the model's nearest point %v", z, near)
	}
	splitFrac = 1
	if z := splitPlaneZ(); z > far {
		t.Errorf("at the near end the plane is at %v, which is not behind the model's farthest point %v", z, far)
	}
	splitFrac = 0
	if z := splitPlaneZ(); z != -20 {
		t.Errorf("in the middle the plane is at %v, want the model's center at -20", z)
	}
}

// Both ends are single-pass: the whole point of the epsilon is that a knob
// resting at 0.999 does not pay for a second pass and a frame copy to draw a
// half that nobody can see.
func TestTheEndsOfTheKnobDoNotSplit(t *testing.T) {
	old := splitFrac
	t.Cleanup(func() { splitFrac = old })

	for _, f := range []float32{-1, -0.995, 0.995, 1} {
		splitFrac = f
		if splitActive() {
			t.Errorf("frac %v asks for two passes", f)
		}
	}
	for _, f := range []float32{-0.9, 0, 0.5, 0.9} {
		splitFrac = f
		if !splitActive() {
			t.Errorf("frac %v does not split, so nothing would be in front", f)
		}
	}
	splitFrac = 1
	if !splitAllInFront() {
		t.Error("the near end is not reported as all-in-front, so the canvas would not be raised")
	}
	splitFrac = -1
	if splitAllInFront() {
		t.Error("the far end claims to be all-in-front")
	}
}

// With no camera fit yet the plane still has to land somewhere sane rather than
// at zero, which would be the camera's own position and would put the entire
// model on the far side no matter where the knob is.
func TestThePlaneCopesBeforeTheCameraHasBeenFitted(t *testing.T) {
	oldExt, oldDist, oldFrac := modelFitExtent, defaultCameraDist, splitFrac
	t.Cleanup(func() { modelFitExtent, defaultCameraDist, splitFrac = oldExt, oldDist, oldFrac })

	modelFitExtent, defaultCameraDist, splitFrac = 0, 30, 0
	if z := splitPlaneZ(); z != -30 {
		t.Errorf("plane at %v with no fit, want the camera distance -30", z)
	}
	splitFrac = 1
	if z := splitPlaneZ(); z >= -30 {
		t.Errorf("plane at %v did not move behind the center when the knob went forward", z)
	}
}

// frontCanvasPx is what a browser test reads to prove the near canvas tracks
// the main one; with no canvas built it must answer rather than panic.
func TestFrontCanvasSizeIsReadableBeforeItExists(t *testing.T) {
	if frontCanvas.Truthy() {
		t.Skip("a canvas already exists in this run")
	}
	if got := frontCanvasPx(); got != "" {
		t.Errorf("frontCanvasPx = %q with no canvas, want empty", got)
	}
}

// The knob being mid-range is not enough to put anything on the near canvas:
// the MODE has to be one that draws twice. They were the same question for
// about ten minutes and the gap was a bug — an attractor's near half stayed
// frozen over the panel after switching to a model that never repaints it.
func TestOnlyModesThatRedrawAreSplit(t *testing.T) {
	oldFrac, oldMode := splitFrac, selectedMode
	t.Cleanup(func() { splitFrac, selectedMode = oldFrac, oldMode })

	splitFrac = 0 // hard in the middle: the knob is asking for a split

	// Attractors: one integration, and the far pass re-issues the draw.
	selectedMode = "lorenz"
	if !splitDrawing() {
		t.Error("an attractor mid-knob is not being split, so nothing reaches the near canvas")
	}
	if splitRegenerates(selectedMode) {
		t.Error("an attractor would be generated twice, which advances it twice and runs it at double speed")
	}

	// Static geometry: it splits too, by generating a second time.
	//
	// This list used to be on the other side of this test, on the stated
	// grounds that these modes rebuild their geometry as they draw. They do
	// not: uploadBuffersIndexed re-uploads only while staticGeomDirty is set,
	// and re-issues drawElements against the cached buffers otherwise. What the
	// old rule actually cost was the feature — the knob did nothing whatever on
	// a torus until it hit the very end of its travel.
	for _, m := range []string{"torus", "dodecahedron", "globe", "magnetosphere"} {
		selectedMode = m
		if !splitDrawing() {
			t.Errorf("%s does not split, so the knob does nothing until it reaches an end", m)
		}
		if !splitRegenerates(m) {
			t.Errorf("%s should draw its far half by generating again", m)
		}
	}

	// The texture planes stay whole. A picture framed face on has no depth for
	// the plane to cut, and generating one twice would upload its texture twice
	// a frame for a half that cannot exist.
	for _, m := range []string{"terminal", "desk", "spectrogram", "recurrence"} {
		selectedMode = m
		if splitDrawing() {
			t.Errorf("%s is a flat picture and has no near half to draw", m)
		}
	}

	// And the knob still governs: an attractor at the far end draws once.
	selectedMode, splitFrac = "lorenz", -1
	if splitDrawing() {
		t.Error("the far end of the knob still asks for two passes")
	}
}
