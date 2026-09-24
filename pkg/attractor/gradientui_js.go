//go:build js && wasm

package attractor

import (
	_ "embed"
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/colorspace"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"syscall/js"
)

// ── Event handlers ───────────────────────────────────────────────────────────

// refreshGradient rescans the drawn geometry for the gradient's normalizing
// extents. Called when a knob moves, which changes the shape under the scan.
func refreshGradient() {
	if !gpu.ready || len(gpu.verts) == 0 {
		return
	}
	gpu.updateGradientRange(gpu.verts)
	gradientRangePending = false
	gradientRangeSeq = gpu.uploadSeq
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
	gradientRangeSeq = gpu.uploadSeq
}

// gradientRangeDue reports whether an owed refresh can now be taken: something
// has been uploaded since the mode changed, so the buffer is this mode's.
func gradientRangeDue() bool {
	return gradientRangePending && gpu.uploadSeq != gradientRangeSeq
}

var (
	// gradientRangePending says a refresh is owed, gradientRangeSeq the upload
	// count it is waiting to see move.
	gradientRangePending bool
	gradientRangeSeq     uint64
)

// layout.standalone is true when the controls are our own fixed overlay (not
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
	dim("grp-cstart", style.gradientColors == 4 && style.gradientSource != GradientSourceOff) // no fixed colors in a hue sweep
	dim("grp-cmid", style.gradientColors != 3 || style.gradientSource == GradientSourceOff)   // mid only in 3-color
	dim("grp-cend", !(style.gradientColors == 2 || style.gradientColors == 3) ||
		style.gradientSource == GradientSourceOff) // end in 2- / 3-color
	// The period is the map WINDOW's width, and a window is something the hue
	// sweep and the colormaps both have — it stopped being the rainbow's private
	// knob when the colormaps gained a shift to slide along it. The shift itself
	// stays a colormap control: the hue sweep's offset is uGradientPhase, which
	// already exists and already animates.
	dim("grp-rainbow", style.gradientColors != 4 && style.gradientColors < colormap.First)
	dim("grp-pshift", style.gradientColors < colormap.First)
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
	usesSrc := modeUsesGradientSource(run.selectedMode)
	dim("src-cell", !usesSrc)
	dim("map-cell", usesSrc && style.gradientSource == GradientSourceOff)
	if lbl := dom.Doc.Call("getElementById", "lbl-cstart"); lbl.Truthy() {
		if style.gradientSource == GradientSourceOff {
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

func onColorChange(this js.Value, args []js.Value) any {
	baseHex := dom.Doc.Call("getElementById", "color-base").Get("value").String()
	midHex := dom.Doc.Call("getElementById", "color-mid").Get("value").String()
	topHex := dom.Doc.Call("getElementById", "color-top").Get("value").String()
	style.baseColor = colorspace.ParseHex(baseHex)
	style.midColor = colorspace.ParseHex(midHex)
	style.topColor = colorspace.ParseHex(topHex)
	glctx.GL.Call("uniform3f", gpu.u.baseColor, style.baseColor[0], style.baseColor[1], style.baseColor[2])
	glctx.GL.Call("uniform3f", gpu.u.midColor, style.midColor[0], style.midColor[1], style.midColor[2])
	glctx.GL.Call("uniform3f", gpu.u.topColor, style.topColor[0], style.topColor[1], style.topColor[2])
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
