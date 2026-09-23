//go:build js && wasm

package attractor

import (
	_ "embed"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
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
	// All the color knobs stay visible; the ones that do not apply to the
	// current SRC and MAP setting are just dimmed, with no populate/depopulate
	// reflow when either moves. bg always applies.
	dim := func(id string, inactive bool) {
		if el := dom.Doc.Call("getElementById", id); el.Truthy() {
			el.Get("style").Set("display", "")
			el.Get("classList").Call("toggle", "pal-dim", inactive)
		}
	}
	dim("grp-cstart", gradientColors == 4 && gradientSource != GradientSourceOff) // no fixed colors in a hue sweep
	dim("grp-cmid", gradientColors != 3 || gradientSource == GradientSourceOff)   // mid only in 3-color
	dim("grp-cend", !(gradientColors == 2 || gradientColors == 3) ||
		gradientSource == GradientSourceOff) // end in 2- / 3-color
	// The period is the map WINDOW's width, and a window is something the hue
	// sweep and the colormaps both have — it stopped being the rainbow's private
	// knob when the colormaps gained a shift to slide along it. The shift itself
	// stays a colormap control: the hue sweep's offset is uGradientPhase, which
	// already exists and already animates.
	dim("grp-rainbow", gradientColors != 4 && gradientColors < paletteFirst)
	dim("grp-pshift", gradientColors < paletteFirst)
	// The two rings, dimmed when the model on screen cannot use them.
	//
	// Both rules now fall out of what the knobs MEAN rather than being special
	// cases bolted on. SRC is dimmed where there is no source to choose: the
	// spectrogram, the RTA and the transfer function are each built from one
	// quantity, so nothing about them is a choice of what the color follows,
	// and the knob sat there looking live in those modes. MAP is dimmed where
	// the source is OFF and the model reads the source at all — a trace that
	// follows nothing has no value to map — but NOT in those same three modes,
	// which keep mapping their own quantity whatever the src ring says.
	//
	// A selected phosphor overrides both, and that is covered where it belongs:
	// src-cell and map-cell are in crtOverriddenIDs, so the whole cell takes
	// crt-dim without this function knowing about phosphors at all.
	usesSrc := modeUsesGradientSource(selectedMode)
	dim("src-cell", !usesSrc)
	dim("map-cell", usesSrc && gradientSource == GradientSourceOff)
	if lbl := dom.Doc.Call("getElementById", "lbl-cstart"); lbl.Truthy() {
		if gradientSource == GradientSourceOff {
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
	baseHex := dom.Doc.Call("getElementById", "color-base").Get("value").String()
	midHex := dom.Doc.Call("getElementById", "color-mid").Get("value").String()
	topHex := dom.Doc.Call("getElementById", "color-top").Get("value").String()
	baseColor[0], baseColor[1], baseColor[2] = hexToRGB(baseHex)
	midColor[0], midColor[1], midColor[2] = hexToRGB(midHex)
	topColor[0], topColor[1], topColor[2] = hexToRGB(topHex)
	glctx.GL.Call("uniform3f", uBaseColorLoc, baseColor[0], baseColor[1], baseColor[2])
	glctx.GL.Call("uniform3f", uMidColorLoc, midColor[0], midColor[1], midColor[2])
	glctx.GL.Call("uniform3f", uTopColorLoc, topColor[0], topColor[1], topColor[2])
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
