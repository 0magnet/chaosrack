//go:build js && wasm

package attractor

// The depth partition: which side of a plane in front of the camera a pass is
// allowed to draw.
//
// The rack panel is DOM and the model is WebGL, so "part of the model in front
// of the controls and part behind" cannot be one canvas — there is no way to
// put DOM between two halves of a single drawing. It takes a canvas on each
// side of the panel in the stacking order, and the same scene drawn into both
// with a plane deciding which fragments belong to which.
//
// This file is the plane. Nothing here draws.

// Which side of the plane a pass keeps. The values are what the shader reads.
const (
	splitFar  = -1 // behind the plane: the half that goes on the back canvas
	splitNone = 0  // no partition, and the only state the app had before this
	splitNear = 1  // in front of the plane: the half that goes on the front one
)

// setSplitPlane tells the shader where the plane is and which side to keep.
//
// z is in VIEW space, where the camera sits at the origin looking down -Z, so
// the values are negative and a smaller number is farther away. Callers work in
// that space rather than in world coordinates because the plane is a property
// of the viewer — it stays parallel to the screen however the model is turned,
// which is what makes a knob that slides the model "through" the panel behave
// the way a hand would expect.
func setSplitPlane(side int, z float32) {
	if !gl.Truthy() || uSplitSideLoc.IsUndefined() || uSplitSideLoc.IsNull() {
		return
	}
	gl.Call("uniform1i", uSplitSideLoc, side)
	gl.Call("uniform1f", uSplitZLoc, float64(z))
}

// modelFitExtent is how far the model reaches from its center, as measured the
// last time the camera was fitted to it. Zero until then.
var modelFitExtent float32

// splitFrac is the Fore knob: -1 puts the whole model behind the rack, +1 puts
// all of it in front, and anything between is a plane cutting through it.
//
// It is a fraction rather than a distance so the control means the same thing
// whatever is being shown. A Lorenz attractor and a dodecahedron are different
// sizes in world units, and a knob calibrated in those would need re-learning
// per model; in fractions, the middle is always the middle of the model.
var splitFrac float32 = -1

// splitEpsilon is how close to an end counts as being at that end. A knob is an
// analog thing and lands on 0.999 as often as on 1, and a two-pass render for
// a half nobody can see is pure cost.
const splitEpsilon = 0.02

// splitActive reports whether the model has to be drawn on both sides.
func splitActive() bool {
	return splitFrac > -1+splitEpsilon && splitFrac < 1-splitEpsilon
}

// splitDrawing reports whether THIS frame is drawn in two halves.
//
// splitActive is about the knob; this is about the knob AND the model. Only the
// attractors can be drawn twice cheaply, because their second pass re-issues a
// draw against a buffer that is already on the card. The geometry models build
// their vertices as they draw, so a second pass would rebuild them, and until
// that is worth doing they stay on one side.
//
// The two questions were the same question for about ten minutes, and the gap
// between them was a bug: the near canvas was shown whenever the KNOB was
// mid-range, while only attractors ever painted it. Switching from an attractor
// to a torus left the attractor's near half hanging over the panel, frozen,
// while the torus turned behind it.
func splitDrawing() bool {
	return splitActive() && (isAttractorMode(selectedMode) || splitRegenerates(selectedMode))
}

// splitRegenerates names the modes whose second pass RUNS THE GENERATOR AGAIN
// instead of re-issuing one draw call, and it is the whole reason the knob
// works on a torus.
//
// The two halves are drawn differently because the two kinds of model are
// different. An attractor INTEGRATES as it draws, so generating twice would
// advance it twice and run it at double speed — its second pass re-issues the
// draw against the buffer already on the card. Static geometry advances
// nothing: running its generator a second time produces the same vertices in
// the same place, and uploadBuffersIndexed skips the upload entirely once
// staticGeomDirty is clear, so the cost is the vertex build and not a round
// trip to the card.
//
// This used to be excluded, on the stated grounds that "a second pass would
// rebuild them". That was wrong, and measurably so: the buffers are cached and
// the second pass re-issues drawElements against them. What the exclusion
// actually cost was the feature — the knob did nothing at all on a torus or a
// polyhedron until it hit the very end of its travel, where the whole canvas
// jumps in front of the panel. Measured before this: front-canvas pixels were
// zero at every knob position for both.
//
// Generating twice, rather than re-issuing one draw, is what makes it correct
// for a model that issues SEVERAL draw calls per frame — re-issuing only the
// last one would put a fraction of the model on the far side.
// The texture planes are excluded even though they are filed as geometry. Two
// reasons, and the first is enough on its own: generating one twice would
// upload its texture twice a frame, and for the terminal that is a 900x560
// canvas going to the card for a second time to draw a half nobody asked for.
// The second is that there is no half — a texture plane is framed face on, so
// the whole quad sits at one depth and the plane can only ever put all of it on
// one side. Nothing is lost by leaving them to the ends of the knob.
func splitRegenerates(mode string) bool {
	return modeInfo[mode].Class == ClassGeometry && !isTexturePlane(mode)
}

// splitAllInFront reports the other end of the knob, where the whole model
// belongs on the near canvas and the old Front behavior is exactly right.
func splitAllInFront() bool { return splitFrac >= 1-splitEpsilon }

// splitPlaneZ is where the partition sits in view space.
//
// The camera is at the origin looking down -Z, so the model's center is at
// -defaultCameraDist and its extent reaches modelFitExtent either side of that.
// At frac -1 the plane is put in FRONT of everything, which leaves the whole
// model on the far side; at +1 it goes behind everything and the whole model is
// near. The sign is what makes the knob read the way a hand expects: turning it
// up brings the model forward through the panel.
func splitPlaneZ() float32 {
	ext := modelFitExtent
	if ext <= 0 {
		// Nothing has fitted the camera yet, so guess from the distance. Half
		// of it is generous for anything the fitter would have chosen.
		ext = defaultCameraDist * 0.5
	}
	return -defaultCameraDist - splitFrac*ext
}

// drawSplitPasses draws the model twice, once for each side of the plane.
//
// THE SIMULATION IS STEPPED ONCE. generateForMode both integrates and draws, so
// calling it twice would advance the attractor twice per frame and run it at
// double speed — the near pass generates, and the far pass re-issues the draw
// against the buffer that is already on the card, which is exactly what the
// paused path does to redraw a frozen frame.
//
// Static geometry has no simulation to step, so it takes the other branch and
// generates twice; splitRegenerates says why that is the right pass for it.
//
// Order is forced: the near half has to be drawn first because the only way off
// the GL canvas is to copy it, and the copy takes whatever is there. So the
// near half is drawn, lifted onto the front canvas, and then overwritten by the
// far half, which is what stays on screen behind the panel.
//
// Persist and phosphor cannot survive this. Both work by NOT clearing, and the
// far pass has to clear to get rid of the near half it is replacing. Split wins
// while it is on; the trail comes back when the knob returns to an end. That is
// a trade rather than an oversight, but it used to be recorded only in this
// comment, where nobody turning the knob could read it — markPersistSuspended
// now says it on the panel too.
func drawSplitPasses(mode string) {
	z := splitPlaneZ()

	setSplitPlane(splitNear, z)
	generateForMode(mode)
	drawn := lastDrawnCount
	copyNearPassToFront()

	setSplitPlane(splitFar, z)
	gl.Call("clear", glTypes.ColorBufferBit)
	gl.Call("clear", glTypes.DepthBufferBit)
	switch {
	case splitRegenerates(mode):
		// Nothing to advance, so the far half is the generator run again with
		// the plane flipped — see splitRegenerates for why this is the correct
		// second pass for geometry and the wrong one for an attractor.
		generateForMode(mode)
	case drawn > 0:
		gl.Call("drawArrays", mapDrawMode(mode), 0, drawn)
	}

	setSplitPlane(splitNone, 0)
}

// syncSplitCanvas puts the near canvas in the right state for the knob.
//
// At the far end there is nothing in front and the canvas is hidden outright,
// which is the common case and saves compositing a transparent full-screen
// layer every frame. At the near end the whole model is in front, and the old
// Front behavior — raise the main canvas above the panel — is both cheaper and
// exactly right, so the near canvas stays out of it.
func syncSplitCanvas() {
	markPersistSuspended(splitDrawing())
	if splitDrawing() {
		if ensureFrontCanvas() {
			showFrontCanvas(true)
		}
		raiseMainCanvas(false)
		return
	}
	showFrontCanvas(false)
	raiseMainCanvas(splitAllInFront())
}

// persistTitle is the Persist switch's own tooltip, kept so the suspended one
// can be swapped back for it. Read from the DOM rather than repeated here,
// because a copy would drift the first time the markup was reworded.
var (
	persistTitle     string
	persistTitleRead bool // guards the capture, so an empty title isn't re-read
)

// markPersistSuspended says on the panel that Persist is not doing anything.
//
// drawSplitPasses clears between the halves and so cannot accumulate, which is
// a deliberate trade and documented above — but only in this file. On screen it
// read as a switch that was on and had stopped working: turn Fore off either
// end and the trail you had asked for silently stops building, with the switch
// still checked and nothing saying why. Measured mid-knob, a persisted trail
// held steady around 18k lit pixels where the same run at the far end grew from
// 192k to 269k.
//
// Dimmed rather than disabled, and the checkbox is left checked: the preference
// is still yours and the trail comes back the moment the knob reaches an end.
// Unchecking it here would make the knob quietly rewrite a switch the person
// set, which is worse than a switch that says it is waiting.
//
// The class is the one the phosphor overrides already use for the same
// sentence — this control does not apply right now — so it looks like the rest
// of the panel rather than like a new idea.
func markPersistSuspended(on bool) {
	sw := doc.Call("getElementById", "persist-trail")
	if !sw.Truthy() {
		return
	}
	lbl := sw.Call("closest", "label")
	if !lbl.Truthy() {
		return
	}
	if !persistTitleRead {
		persistTitle = lbl.Call("getAttribute", "title").String()
		persistTitleRead = true
	}
	lbl.Get("classList").Call("toggle", "split-dim", on)
	if on {
		lbl.Call("setAttribute", "title",
			"Suspended while Fore is mid-range: the model is drawn twice, once for "+
				"each side of the panel, and the second pass has to clear what the "+
				"first left. The trail returns when Fore reaches either end.")
		return
	}
	lbl.Call("setAttribute", "title", persistTitle)
}

// raiseMainCanvas moves the whole GL canvas across the panel in the stacking
// order — the original Front switch, kept because at the near end it does the
// whole job on its own.
func raiseMainCanvas(front bool) {
	cont := doc.Call("getElementById", "gocanvas-container")
	if !cont.Truthy() {
		return
	}
	st := cont.Get("style")
	if front {
		st.Set("zIndex", "var(--z-canvas-front)")
		st.Set("pointerEvents", "none")
		return
	}
	st.Set("zIndex", "var(--z-canvas)")
	st.Set("pointerEvents", "auto")
}
