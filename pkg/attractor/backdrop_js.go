//go:build js && wasm

package attractor

// Background visualizer: render a live audio display (scrolling spectrogram or
// xy/Lissajous scope) BEHIND the current attractor / geometry model, so the
// attractor floats over a reactive backdrop. It is a layer, not a mode — the
// spectrogram and xy MODES still exist (they replace the model); this draws the
// same visuals as a full-screen background under a normal model instead.
//
// Driven from renderLoop: after the frame is cleared and before the attractor
// is generated, renderBackgroundVisual() paints the backdrop with depth-testing
// off (a flat 2D layer); the attractor then draws on top with depth on.

// bgVisual selects the background layer: "" (off) | "spectrogram" | "xy".
var bgVisual string

// bgVisualActive reports whether a background visualizer is enabled AND the
// current model is a normal one (not itself a visualizer mode, which would
// already fill the screen).
func bgVisualActive() bool {
	if bgVisual == "" {
		return false
	}
	if isSpectroSurface(selectedMode) || isAudioMode(selectedMode) {
		return false
	}
	return true
}

// setBackgroundVisual switches the background layer. Turning one on ensures the
// shared audio source is live so samples start flowing immediately.
func setBackgroundVisual(kind string) {
	bgVisual = kind
	if bgVisual != "" {
		ensureAudioSource()
	}
}

// renderBackgroundVisual paints the selected backdrop onto the (already
// cleared) color buffer with depth off. Leaves depth test disabled; the caller
// re-enables it before drawing the attractor.
func renderBackgroundVisual(nowMs float64) {
	switch bgVisual {
	case "xy":
		drawXYScope(false) // no self-clear — layer onto the current buffer
	case "spectrogram":
		drawSpectrogramBackground(nowMs)
	case "desk":
		drawDeskBackground()
	case "terminal":
		drawTerminalBackground()
	case "termanim":
		drawTermAnimBackground()
	}
}

// drawSpectrogramBackground keeps the scrolling spectrogram texture current and
// draws it face-on filling the canvas (regardless of the Fill switch), with no
// clear, so it sits behind the model.
func drawSpectrogramBackground(nowMs float64) {
	if !spectReady {
		initSpectrogram()
	}
	ensureAudioSource()
	updateSpectrogramTexture(nowMs)
	offset := float32(spectTexCol) / float32(spectTexW)
	// Force the full-screen face-on placement for the duration of this draw
	// (the background always fills; the Fill switch only governs the MODE).
	savedFill := spectFill
	spectFill = true
	gl.Call("disable", glTypes.DepthTest)
	drawTexturedPlane(spectTexture, offset)
	spectFill = savedFill
}

// syncLayersModule dims the two layer controls the current model cannot take.
//
// The Layers module answers one question three ways — where a second picture
// goes relative to the model: BEHIND it (the backdrop), ON it (the spectrogram
// skin), or AS it (Fill, which turns the spectrogram/FVF plane into the whole
// screen). Backdrop applies to any ordinary model; the other two do not.
//
// Skin needs a surface to paint on, which is what ModeInfo.Skin records, and
// Fill only means anything when the spectrogram or FVF IS the model. Left
// undimmed they were two switches that did nothing, with no way to tell that
// apart from a feature that was broken — the same complaint the Persist switch
// had against the Fore knob, answered the same way and with the same class.
func syncLayersModule(mode string) {
	// The switch dims by its label; the skin selector is a cell with a knob in
	// it, so it dims by the cell. Same class, same sentence, different wrapper.
	dimIn := func(id, wrapper string, applies bool) {
		el := doc.Call("getElementById", id)
		if !el.Truthy() {
			return
		}
		w := el.Call("closest", wrapper)
		if !w.Truthy() {
			return
		}
		w.Get("classList").Call("toggle", "layer-dim", !applies)
	}
	dimIn("skin-visual", ".pcell", isSkinnable(mode))
	dimIn("spect-fill", "label", isSpectroSurface(mode))
}

// syncSpectroModule builds the spectrogram's own controls when it is a LAYER.
//
// A model's parameters live in the Parameters module — one rule, every model.
// The spectrogram breaks that rule by being three things: it can BE the model,
// it can be painted on one as the skin, and it can fill the canvas behind one
// as the backdrop. In the second and third, Parameters is showing whatever the
// model actually is, so the spectrogram's controls had nowhere to be — measured
// before this, spect-col, spect-dft and spect-scale were all absent from the
// document while a spectrogram backdrop was on screen. The backdrop could be
// turned on and then not adjusted at all: no color map, no DFT size, no
// window, no scale.
//
// Built here rather than duplicated in markup, from the same spectParams the
// Parameters module uses and through the same buildParamUnit, so there is one
// definition of what a spectrogram's controls are. It is only ever built when
// the spectrogram is NOT the model, so the element ids stay unique.
func syncSpectroModule(mode string) {
	sect := doc.Call("getElementById", "spectro-module")
	host := doc.Call("getElementById", "spectro-params")
	if !sect.Truthy() || !host.Truthy() {
		return
	}
	host.Set("innerHTML", "")
	asLayer := (skinSource == "spectrogram" && isSkinnable(mode)) || bgVisual == "spectrogram"
	if !asLayer || isSpectroSurface(mode) {
		sect.Get("style").Set("display", "none")
		return
	}
	grid := doc.Call("createElement", "div")
	grid.Set("className", "punit-grid two-col")
	for _, p := range spectParams {
		grid.Call("appendChild", buildParamUnit(p))
	}
	host.Call("appendChild", grid)
	sect.Get("style").Set("display", "")
}
