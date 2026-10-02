//go:build js && wasm

package attractor

import (
	"syscall/js"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/glctx"
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
	defer func() {
		if manualPowerHook != nil && !modelWin.moving {
			manualPowerHook(on) // on the manual page, the model's window
		}
		if sw := dom.Doc.Call("getElementById", "power-sw"); sw.Truthy() && sw.Get("checked").Bool() != on {
			sw.Set("checked", on)
		}
	}()
	if on {
		if run.stopped {
			run.stopped = false
			js.Global().Call("requestAnimationFrame", renderFrame)
		}
		updateInfoOverlay() // Info is the model again, not the manual
		return
	}
	run.stopped = true
	glctx.GL.Call("clearColor", 0, 0, 0, 0)
	glctx.GL.Call("clear", glctx.Types.ColorBufferBit)
	glctx.GL.Call("clear", glctx.Types.DepthBufferBit)
	updateInfoOverlay() // and the manual, now there is no model to describe
}

// attachSelMarquee puts a character display over a <select>, reading its
// current option: the model name under a bay's MODEL knob. The select stays
// under it, click-through, so a click still opens the list. A name longer than
// the display scrolls through it a character at a time, the way a character
// module scrolls, after a pause on its start.
//
// Counted in characters, not measured: the overlay it replaces read its text's
// width on every change, which was a forced layout in the middle of a model
// change.
//
// picker is the list the click opens, if it is not sel itself: a merged
// bay's MODEL knob turns a select of one category's models, and its display
// opens one of every model in the bay (bayPicker).
func attachSelMarquee(sel, picker js.Value) {
	parent := picker.Get("parentNode")
	if !parent.Truthy() {
		return
	}
	wrap := dom.Doc.Call("createElement", "span")
	wrap.Set("className", "selwrap")
	parent.Call("insertBefore", wrap, picker)
	wrap.Call("appendChild", picker)
	win := dotDisplayN("", false, dispFullChars)
	win.Get("classList").Call("add", "selmarq")
	wrap.Call("appendChild", win)
	var loop []rune
	pos, hold := 0, 0
	show := func() {
		if len(loop) == 0 {
			return
		}
		out := make([]rune, dispFullChars)
		for i := range out {
			out[i] = loop[(pos+i)%len(loop)]
		}
		setDotText(win, string(out))
	}
	upd := func() {
		txt := ""
		if idx := sel.Get("selectedIndex").Int(); idx >= 0 {
			txt = displayText(sel.Get("options").Index(idx).Get("text").String())
		} else {
			txt = categoryOffLabel // on no position: the bay is off
		}
		loop, pos, hold = []rune(txt), 0, marqueeHold
		if len(loop) > dispFullChars {
			loop = append(loop, []rune("   ")...)
		} else {
			loop = nil
			setDotText(win, txt)
		}
		show()
	}
	sel.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) any { upd(); return nil }))
	js.Global().Call("setInterval", js.FuncOf(func(js.Value, []js.Value) any {
		if len(loop) == 0 {
			return nil
		}
		if hold > 0 {
			hold--
			return nil
		}
		pos = (pos + 1) % len(loop)
		if pos == 0 {
			hold = marqueeHold
		}
		show()
		return nil
	}), marqueeStepMs)
	upd()
}

// A scrolling name moves a character every marqueeStepMs, and rests
// marqueeHold steps at its start.
const (
	marqueeStepMs = 300
	marqueeHold   = 6
)

func updateInfoOverlay() {
	overlay := dom.Doc.Call("getElementById", "info-overlay")
	if overlay.IsNull() || overlay.IsUndefined() {
		return
	}
	showInfo := dom.Doc.Call("getElementById", "show-info")
	if showInfo.IsNull() || showInfo.IsUndefined() || !showInfo.Get("checked").Bool() {
		return
	}
	text, ok := attractorDescriptions[run.selectedMode]
	if !ok {
		text = run.selectedMode
	}
	// The turtle path knows something specific about the figure currently on
	// screen — whether it closes, drifts or screws away — which the static
	// description cannot say.
	if run.selectedMode == "turtle" {
		if label := turtle.shapeLabel(); label != "" {
			text += "\n\n" + label
		}
	}
	writeInfo(overlay, text) // and the manual for the bay being worked at (infomanual_js.go)
	info.updateInfoTitle()   // a window left open while the model changes says which one it describes
}

// setCellBlank keeps a control on the panel whether or not it does anything
// at the moment. A cell with nothing to do for this model, or at these
// settings, stays bright and in reach: its readouts go blank and its knobs
// still turn, so the setting is there waiting when it matters again. It is
// not taken away, and not darkened: a panel whose controls come and go with
// what it is showing moves everything after them, and the rack is one
// instrument whatever the model.
func setCellBlank(el js.Value, blank bool) {
	if !el.Truthy() {
		return
	}
	el.Get("style").Set("display", "")
	el.Get("classList").Call("toggle", "cell-blank", blank)
}

// updatePhysVisibility makes the Physics switch live only where there is
// something to weigh (turtle), and keeps the panel's ph-on/ph-off class in
// step with it so the Physics module appears and disappears with the switch.
func updatePhysVisibility() {
	setCellBlank(dom.Doc.Call("getElementById", "phys-sw-wrap"), run.selectedMode != "turtle")
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

// updateTrailVisibility blanks the Trail readout on the models that draw no
// trail; the knob stays bright and turnable (setCellBlank).
func updateTrailVisibility() {
	setCellBlank(dom.Doc.Call("getElementById", "trail-controls"), !isAttractorMode(run.selectedMode))
}

// normalizeOrientation resets the current model to the default identity
// pose and zeroes the per-axis spin rates, so it faces the camera head-on.
// Auto-rotate (if on) still applies afterward.

func onModeChange(this js.Value, args []js.Value) any {
	sel := dom.Doc.Call("getElementById", "mode-select")
	// While the panel is on the backdrop, choosing a model chooses what is
	// behind (backlayer_js.go); the running one is left as it is.
	if back.editing && sel.Truthy() {
		chooseBackdrop(sel.Get("value").String())
		return nil
	}
	if sel.Truthy() {
		run.selectedMode = sel.Get("value").String()
	}
	// The whole change as one layout (withDeferredLayout, below): the rack is
	// not read between any two of these, and several of them ask for it to be
	// re-measured. Following the mode's colormap alone (spect.followMode) asked
	// before the batch began, and paid a full pass on every change to the
	// spectrogram.
	owed.modelChange = true
	owed.withDeferredLayout(run.selectedMode, func() {
		// Which row is the instrument, before anything rebuilds: the readouts
		// that belong to the running model are filed into its category's row,
		// and buildParamPanel below re-measures and re-packs the rack. Set after
		// that, they are packed into the row the PREVIOUS model was in.
		setActiveCategory(run.selectedMode)
		// Before the panel rebuild, so the MAP ring is drawn at the map the mode
		// comes up on.
		spect.followMode(run.selectedMode)
		// Whether the model is drawn in two halves depends on the mode as well as
		// the knob, so the canvases have to be reconsidered here — not only when
		// the knob moves.
		syncSplitCanvas()
		// A modulation loop's memory is of the system that is no longer
		// running; carried across, it would drive the new model from the old
		// one's last position. See modelmod.go.
		resetModelMod()
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
		buildParamPanel(run.selectedMode)
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
	generateForMode(run.selectedMode)
	if isTexturePlane(run.selectedMode) {
		spect.setSpectrogramCamera()
	} else {
		spect.restoreAutoRotateAfterSpectrogram()
		view.autoFitCamera()
	}
	// FVF audio-out follows the mode: resume if re-entering FVF with Listen
	// on; stop when leaving so no stray audio plays under other models.
	if run.selectedMode == "fvf" {
		if fvf.listen {
			fvf.startFVFAudio()
		}
	} else {
		fvf.stopFVFAudio()
	}
	// Model Out likewise: suspend in modes it can't sonify (geometry,
	// spectrogram…) instead of streaming zeros ~23×/s, resume in trail modes.
	son.sync()
	// The model's own row shows it and every other row shows off, whatever
	// moved the model — this knob, a permalink, a preset, the jam performer.
	syncCategoryRotaries()
	// No refreshGradient here. Armed above and taken by the first frame that
	// actually uploads: scanning now would scan the previous model whenever this
	// mode's generate has not drawn yet, which is every audio mode.
	perma.syncPermalinkNow() // reflect the new mode in the URL immediately
	return nil
}
