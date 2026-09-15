//go:build js && wasm

package attractor

import (
	_ "embed"
	"syscall/js"
)

// ── Event handlers ───────────────────────────────────────────────────────────

// refreshGradient rescans the drawn geometry for the gradient's normalizing
// extents. Called when a knob moves, which changes the shape under the scan.
func refreshGradient() {
	if !shadersReady || len(attractorVertices) == 0 {
		return
	}
	updateGradientRange(attractorVertices)
	gradientRangePending = false
	gradientRangeSeq = vertexUploadSeq
}

// armGradientRange says the extents are owed, and records how many uploads had
// happened when the mode changed — so the refresh can wait for one MORE.
//
// ── WHY WAITING FOR AN UPLOAD IS THE POINT ───────────────────────────────
//
// The mode switch runs one generate before refreshing, precisely so the scan
// sees the new model. That is not enough for an audio mode, and the failure is
// silent in both of its forms. The waterfall draws nothing at all until it has
// measured, so the buffer is empty and the scan is skipped. The embeddings are
// worse: generateTakens returns early until its ring has filled, WITHOUT
// uploading, so the buffer still holds the previous model — the scan runs, it
// succeeds, and it sets the extents of a figure that is no longer on screen.
//
// Nothing asked again in either case, so the mode was drawn for as long as it
// was on screen with somebody else's bounding box. A Takens embedding entered
// from the waterfall was normalized to the waterfall's ±4.5, which is a band
// narrower than the figure: everything above it clamped to the top of the
// colormap and everything below to the bottom, and six colormaps all came out
// as two flat colors with a hard line between them.
//
// Counting uploads is what tells the two apart, because "the buffer changed" is
// exactly the question and neither emptiness nor a mode name answers it.
func armGradientRange() {
	gradientRangePending = true
	gradientRangeSeq = vertexUploadSeq
}

// gradientRangeDue reports whether an owed refresh can now be taken: something
// has been uploaded since the mode changed, so the buffer is this mode's.
func gradientRangeDue() bool {
	return gradientRangePending && vertexUploadSeq != gradientRangeSeq
}

var (
	// gradientRangePending says a refresh is owed, gradientRangeSeq the upload
	// count it is waiting to see move.
	gradientRangePending bool
	gradientRangeSeq     uint64
)

// standalonePanel is true when the controls are our own fixed overlay (not
// appended into a host page's <footer>); only then can we dock/move them.

func updateGradientUI() {
	// All palette knobs stay visible; the ones that don't apply to the current
	// color count are just dimmed (no populate/depopulate reflow when the count
	// changes). bg always applies.
	dim := func(id string, inactive bool) {
		if el := doc.Call("getElementById", id); el.Truthy() {
			el.Get("style").Set("display", "")
			el.Get("classList").Call("toggle", "pal-dim", inactive)
		}
	}
	dim("grp-cstart", gradientColors == 4)                         // no fixed colors in rainbow
	dim("grp-cmid", gradientColors != 3)                           // mid only in 3-color
	dim("grp-cend", !(gradientColors == 2 || gradientColors == 3)) // end in 2- / 3-color
	// The period is the palette WINDOW's width, and a window is something the
	// rainbow and the colormaps both have — it stopped being the rainbow's
	// private knob when the colormaps gained a shift to slide along it. The
	// shift itself stays a colormap control: the rainbow's offset is
	// uGradientPhase, which already exists and already animates.
	dim("grp-rainbow", gradientColors != 4 && gradientColors < paletteFirst)
	dim("grp-pshift", gradientColors < paletteFirst)
	// The OUTER ring — what the colour follows — when the inner one is on mono.
	//
	// One colour is one colour whatever value it follows, so on mono the source
	// ring reaches nothing. That is the same rule that already dims the mid
	// swatch outside three-colour and the end swatch outside two; this ring was
	// left out of it, and "I turn it and nothing happens" is the result.
	//
	// Only this case. A selected phosphor overrides BOTH rings — it forces the
	// palette to monochrome in applyPhosphorColor, after the gradient uniforms
	// are set — and that is already covered: src-cell is in crtOverriddenIDs, so
	// the whole cell takes crt-dim. Dimming it a second time here would be two
	// mechanisms for one rule.
	dim("grad-src-ring", gradientColors == 1)
	if lbl := doc.Call("getElementById", "lbl-cstart"); lbl.Truthy() {
		if gradientColors == 1 {
			lbl.Set("textContent", "color")
		} else {
			lbl.Set("textContent", "start")
		}
	}
	updateViewModRows()
	quantizeModuleWidths()
}

// autoRotYDelta is the Y spin-rate the Auto-rotate switch contributes, so
// the auto-spin shows up on the Y rate knob rather than being a hidden term.

func onColorChange(this js.Value, args []js.Value) interface{} {
	baseHex := doc.Call("getElementById", "color-base").Get("value").String()
	midHex := doc.Call("getElementById", "color-mid").Get("value").String()
	topHex := doc.Call("getElementById", "color-top").Get("value").String()
	baseColor[0], baseColor[1], baseColor[2] = hexToRGB(baseHex)
	midColor[0], midColor[1], midColor[2] = hexToRGB(midHex)
	topColor[0], topColor[1], topColor[2] = hexToRGB(topHex)
	gl.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	gl.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	gl.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
	return nil
}

// installErrorNet is the global backstop for the whole CLASS of uncaught async
// errors — promise rejections from browser APIs (fullscreen, permissions,
// media, AudioContext) that reject when the browser refuses, on mobile or in
// embedded/occluded contexts. Rather than chasing a missing .catch at every
// call site one at a time, this contains the class in one place: it logs the
// rejection and prevents the default so a stray one never degrades the session
// or shows the browser's uncaught-error UI. Call-site handling that also updates
// UI (e.g. re-syncing the Fullscreen switch) still lives where it matters; this
// is the safety net beneath it.
