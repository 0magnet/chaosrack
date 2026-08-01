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
