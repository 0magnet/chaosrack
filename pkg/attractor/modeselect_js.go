//go:build js && wasm

package attractor

import (
	_ "embed"
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"strconv"
	"syscall/js"
)

// setPowerState stops or resumes the render loop. Off blanks the canvas but
// keeps the panel; on restarts the loop only if it was actually stopped (so
// flipping the switch twice does not spawn a second RAF loop).
//
// Driven by the Console's Power switch. It used to be the first detent of
// the model category knob — powering the rack down looked like choosing a
// model called OFF — and when that knob went to the rows, the rack's own
// power came back to the Rack group where the frame's other controls are.
func setPowerState(on bool) {
	if on {
		if stopped {
			stopped = false
			js.Global().Call("requestAnimationFrame", renderFrame)
		}
		return
	}
	stopped = true
	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
	glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
}

// attachSelMarquee caps a Console <select> to one unit and overlays a marquee
// readout of the current option: the native text is hidden (see CSS), a
// click-through overlay shows the name, and long names that overflow scroll
// back and forth so they stay readable.
func attachSelMarquee(sel js.Value, colorHex string) {
	parent := sel.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "selwrap")
	parent.Call("insertBefore", wrap, sel)
	wrap.Call("appendChild", sel)
	marq := dom.Doc.Call("createElement", "span")
	marq.Set("className", "selmarq")
	if colorHex != "" {
		marq.Get("style").Set("color", colorHex)
	}
	inner := dom.Doc.Call("createElement", "span")
	marq.Call("appendChild", inner)
	wrap.Call("appendChild", marq)
	upd := func() {
		idx := sel.Get("selectedIndex").Int()
		txt := ""
		if idx >= 0 {
			txt = sel.Get("options").Index(idx).Get("text").String()
		}
		inner.Set("textContent", txt)
		over := inner.Get("offsetWidth").Int() - marq.Get("clientWidth").Int()
		if over > 4 {
			marq.Get("style").Call("setProperty", "--marq-shift", "-"+strconv.Itoa(over+6)+"px")
			marq.Get("classList").Call("add", "scroll")
		} else {
			marq.Get("classList").Call("remove", "scroll")
		}
	}
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} { upd(); return nil }))
	upd()
}

func updateInfoOverlay() {
	overlay := dom.Doc.Call("getElementById", "info-overlay")
	if overlay.IsNull() || overlay.IsUndefined() {
		return
	}
	showInfo := dom.Doc.Call("getElementById", "show-info")
	if showInfo.IsNull() || showInfo.IsUndefined() || !showInfo.Get("checked").Bool() {
		return
	}
	text, ok := attractorDescriptions[selectedMode]
	if !ok {
		text = selectedMode
	}
	// The turtle path knows something specific about the figure currently on
	// screen — whether it closes, drifts or screws away — which the static
	// description cannot say.
	if selectedMode == "turtle" {
		if label := turtle.shapeLabel(); label != "" {
			text += "\n\n" + label
		}
	}
	overlay.Set("textContent", text)
	info.updateInfoTitle() // a window left open while the model changes says which one it describes
}

// updatePhysVisibility shows the Physics switch only where there is something
// to weigh, and keeps the panel's ph-on/ph-off class in step with it so the
// Physics module appears and disappears with the switch.
func updatePhysVisibility() {
	if w := dom.Doc.Call("getElementById", "phys-sw-wrap"); w.Truthy() {
		if selectedMode == "turtle" {
			w.Get("style").Set("display", "")
		} else {
			w.Get("style").Set("display", "none")
		}
	}
	if panel := dom.Doc.Call("getElementById", "controls-panel"); panel.Truthy() {
		cl := panel.Get("classList")
		if physOn() {
			cl.Call("add", "ph-on")
			cl.Call("remove", "ph-off")
		} else {
			cl.Call("add", "ph-off")
			cl.Call("remove", "ph-on")
		}
	}
}

// updateTrailVisibility shows/hides the Trail slider + Persist
// checkbox depending on whether the current mode renders a trail.
func updateTrailVisibility() {
	el := dom.Doc.Call("getElementById", "trail-controls")
	if !el.Truthy() {
		return
	}
	if isAttractorMode(selectedMode) {
		el.Get("style").Set("display", "")
	} else {
		el.Get("style").Set("display", "none")
	}
}

// normalizeOrientation resets the current model to the default identity
// pose and zeroes the per-axis spin rates, so it faces the camera head-on.
// Auto-rotate (if on) still applies afterward.

func onModeChange(this js.Value, args []js.Value) interface{} {
	sel := dom.Doc.Call("getElementById", "mode-select")
	if sel.Truthy() {
		selectedMode = sel.Get("value").String()
	}
	// Which row is the instrument, before anything rebuilds: the readouts
	// that belong to the running model are filed into its category's row,
	// and buildParamPanel below re-measures and re-packs the rack. Set after
	// that, they are packed into the row the PREVIOUS model was in.
	setActiveCategory(selectedMode)
	// Whether the model is drawn in two halves depends on the mode as well as
	// the knob, so the canvases have to be reconsidered here — not only when
	// the knob moves.
	syncSplitCanvas()
	// A modulation loop's memory is of the system that is no longer
	// running; carried across, it would drive the new model from the old
	// one's last position. See modelmod.go.
	resetModelMod()
	// Keep the "Edit eqn" switch in sync with whether we're in Custom mode.
	if sw := dom.Doc.Call("getElementById", "edit-eq-sw"); sw.Truthy() {
		sw.Set("checked", selectedMode == "custom")
	}
	// New mode means fresh geometry — force an upload on the next
	// uploadBuffersIndexed for static modes, and a skin-mesh rebuild.
	gpu.staticDirty = true
	skin.dirty = true
	// The Takens mode measures τ once when it first has audio to measure, and
	// entering the mode is what "first" means. The source may also have been
	// swapped while the mode was away, leaving a measurement of a signal that
	// is no longer playing.
	emb.armAutoMeasure()
	wfall.armFit()
	resetAttractorState()
	// The panel rebuild and the four mode-dependent visibility passes, as
	// one layout.
	//
	// Every one of these asks the rack to re-measure itself, and each ask
	// was answered in full: five passes stretching all seventy-odd modules
	// out, reading them back, re-packing every bay and re-sizing every
	// skirt. Measured, that was 2435ms of a 2903ms model change — against
	// 10ms to integrate the attractor. The rack cannot be read between two
	// of these calls, so it does not need to be settled between them; it
	// needs to be settled once, here, before the camera is fitted to it.
	//
	// updateGradientUI belongs in this group and used to sit forty lines
	// down among the audio calls. It is the same kind of thing as the three
	// above it — which controls this model has any use for — and depends on
	// nothing that happens in between.
	withDeferredLayout(selectedMode, func() {
		buildParamPanel(selectedMode)
		updateInfoOverlay()
		updateTrailVisibility()
		updatePhysVisibility()
		// Which rings apply depends on the model: a display built from one
		// quantity has no source to choose, so the src knob dims in those
		// modes.
		updateGradientUI()
	})
	// Run one frame to populate vertices, then update gradient and fit camera.
	// Armed BEFORE that generate: an audio mode may not upload anything on it,
	// and the refresh has to wait for the upload rather than for the call.
	armGradientRange()
	generateForMode(selectedMode)
	if isTexturePlane(selectedMode) {
		spect.setSpectrogramCamera()
	} else {
		spect.restoreAutoRotateAfterSpectrogram()
		view.autoFitCamera()
	}
	// FVF audio-out follows the mode: resume if re-entering FVF with Listen
	// on; stop when leaving so no stray audio plays under other models.
	if selectedMode == "fvf" {
		if fvf.listen {
			fvf.startFVFAudio()
		}
	} else {
		fvf.stopFVFAudio()
	}
	// Model Out likewise: suspend in modes it can't sonify (geometry,
	// spectrogram…) instead of streaming zeros ~23×/s, resume in trail modes.
	son.modeSync()
	// The model's own row shows it and every other row shows off, whatever
	// moved the model — this knob, a permalink, a preset, the jam performer.
	syncCategoryRotaries()
	// No refreshGradient here. Armed above and taken by the first frame that
	// actually uploads: scanning now would scan the previous model whenever this
	// mode's generate has not drawn yet, which is every audio mode.
	perma.syncPermalinkNow() // reflect the new mode in the URL immediately
	return nil
}
