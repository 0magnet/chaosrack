//go:build js && wasm

package attractor

// Two views of the model, side by side.
//
// Step one of a pair. This one puts a second viewport on the canvas and
// draws the mode into both, sharing ONE camera — so the two show the same
// pose at the same zoom, and what they are for is comparing two settings of
// the same instrument rather than two instruments. The camera can be split
// after; the rendering has to work first, and it is the half that can be
// looked at.
//
// It follows drawSplitPasses (split_js.go), which already draws the model
// twice for the Fore knob: a pass is a viewport, a projection and a call to
// generateForMode. The differences from that one are that these passes sit
// beside each other rather than in front and behind, and that each gets its
// own projection because a half-width viewport has a different aspect.
//
// What is deliberately NOT here: the panel still drives one instance, so
// both halves currently draw the same figure. That is the honest state of
// step one — it proves the viewport split without pretending the knobs have
// been split too, which is the next commit and a different problem (the
// parameter rows bind to &stereo.tau at build time, so rebinding them to a
// focused view is a panel change, not a rendering one).

import (
	"syscall/js"

	"github.com/go-gl/mathgl/mgl32"
)

// viewSplit is the Views switch: one view or two.
var viewSplit bool

// viewGap is the gutter between the two viewports, in pixels. Without one
// the two figures touch and read as a single wide picture.
const viewGap = 2

// viewRects is where each view draws, as GL viewport rectangles measured
// from the bottom-left. One rect when the split is off.
//
// A canvas too narrow to hold two views plus the gutter gives ONE rect: GL
// rejects a zero or negative viewport width, and half of nothing is not a
// view. That is not a hypothetical — a panel dragged wide enough, or a
// phone in portrait, gets there.
func viewRects() [][4]int {
	// Clamped to a pixel: a canvas can be measured at zero while the panel
	// is being laid out, and GL rejects a zero-width viewport whether the
	// split asked for it or not.
	w, h := width, height
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	full := [][4]int{{0, 0, w, h}}
	if !viewSplit {
		return full
	}
	// Each view needs at least a pixel, and the gutter sits between them.
	if w < 2+viewGap {
		return full
	}
	half := (w - viewGap) / 2
	right := w - half - viewGap
	if half < 1 || right < 1 {
		return full
	}
	return [][4]int{
		{0, 0, half, h},
		{half + viewGap, 0, right, h},
	}
}

// setViewport points GL at one rect and gives the shader the projection for
// its aspect.
//
// The projection has to be per view: a half-width viewport is half the
// aspect ratio, and reusing the full-canvas matrix draws a figure stretched
// to twice its width inside it. projMatrix is a package variable that
// texProgram also reads, so it is restored by the caller running the full
// rect last.
func setViewport(r [4]int) {
	gl.Call("viewport", r[0], r[1], r[2], r[3])
	h := r[3]
	if h < 1 {
		h = 1
	}
	projMatrix = mgl32.Perspective(mgl32.DegToRad(45.0), float32(r[2])/float32(h), 1, 1500.0)
	gl.Call("useProgram", shaderProgram)
	gl.Call("uniformMatrix4fv",
		gl.Call("getUniformLocation", shaderProgram, "Pmatrix"), false, mat4ToTyped(&projMatrix))
}

// drawViewPasses draws the mode once per view.
//
// The scissor is what keeps a pass inside its rect: a viewport clips the
// geometry but not a clear or a full-screen effect, and without it the
// second pass's background wipes the first pass's figure.
func drawViewPasses(mode string) {
	rects := viewRects()
	if len(rects) == 1 {
		generateForMode(mode)
		return
	}
	gl.Call("enable", gl.Get("SCISSOR_TEST"))
	for _, r := range rects {
		gl.Call("scissor", r[0], r[1], r[2], r[3])
		setViewport(r)
		generateForMode(mode)
	}
	gl.Call("disable", gl.Get("SCISSOR_TEST"))
	// Back to the whole canvas, so everything drawn after these passes —
	// the Poincaré overlay, the lens, the next frame's clear — sees the
	// state it has always seen.
	setViewport([4]int{0, 0, width, height})
}

// wireViewSplitSwitch hooks up the Views checkbox.
func wireViewSplitSwitch() {
	sw := doc.Call("getElementById", "views-sw")
	if !sw.Truthy() {
		return
	}
	sw.Call("addEventListener", "change", trackedFuncOf(func(this js.Value, args []js.Value) interface{} {
		viewSplit = sw.Get("checked").Bool()
		// The camera was fitted to a full-width viewport; half of one wants
		// a different distance, and the fit is what knows how to pick it.
		autoFitCamera()
		return nil
	}))
}
