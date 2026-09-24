//go:build js && wasm

package attractor

import "testing"

// The gradient's extents have to come from the mode ON SCREEN, and the only
// thing that says the buffer is that mode's is an upload having happened since
// the mode changed.

func withGradientRangeState(t *testing.T) {
	t.Helper()
	sr, av, pend, seq, up := gpu.ready, gpu.verts, gradientRangePending, gradientRangeSeq, gpu.uploadSeq
	t.Cleanup(func() {
		gpu.ready, gpu.verts = sr, av
		gradientRangePending, gradientRangeSeq, gpu.uploadSeq = pend, seq, up
	})
}

// TestGradientRangeWaitsForTheModeToDraw is the regression for six colormaps
// that came out as two flat colors with a hard line between them.
//
// A Takens embedding entered from the waterfall was normalized to the
// waterfall's bounds, because generateTakens returns early until its ring has
// filled WITHOUT uploading — so the buffer still held the waterfall, the scan
// ran, it succeeded, and it set the extents of a figure no longer on screen.
// An empty-buffer check does not catch that: the buffer is not empty, it is
// somebody else's.
func TestGradientRangeWaitsForTheModeToDraw(t *testing.T) {
	withGradientRangeState(t)
	gpu.ready = true
	gpu.verts = make([]float32, 8) // the PREVIOUS mode's, still there
	gpu.uploadSeq = 7

	armGradientRange()
	if !gradientRangePending {
		t.Fatal("a mode change did not leave the extents owed")
	}
	if gradientRangeDue() {
		t.Error("the refresh was taken while the buffer was still the previous mode's")
	}
	// Frames go by with the mode drawing nothing of its own.
	for i := range 5 {
		if gradientRangeDue() {
			t.Fatalf("frame %d took the refresh before anything was uploaded", i)
		}
	}
	// The mode finally draws.
	gpu.uploadSeq++
	if !gradientRangeDue() {
		t.Error("the refresh was still refused after the mode uploaded its own geometry")
	}
}

// TestGradientRangeIsOwedOnceOnly checks the refresh does not re-arm itself:
// the scan is an O(n) pass over the whole trail and its own doc comment says it
// is not per-frame work.
func TestGradientRangeIsOwedOnceOnly(t *testing.T) {
	withGradientRangeState(t)
	gpu.ready = false // so refreshGradient returns before touching WebGL
	gpu.verts = make([]float32, 8)
	gpu.uploadSeq = 1
	armGradientRange()
	gpu.uploadSeq++
	if !gradientRangeDue() {
		t.Fatal("not due after an upload")
	}
	// With the shaders down it cannot be taken, and it stays owed rather than
	// being quietly marked done — the mode switch happens either side of the
	// shaders coming up.
	refreshGradient()
	if !gradientRangePending {
		t.Error("an unusable refresh was marked done")
	}
}
