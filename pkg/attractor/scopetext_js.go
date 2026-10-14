//go:build js && wasm

package attractor

// Mode wiring for Fourier Text: the banner is FourierText (fouriertext.go),
// which a native rack draws too, and the harmonic character generator lives
// in pkg/scope/text.go. The beam sweeps the reconstructed curve continuously
// like the Lissajous mode: the drawn window is exactly one period, so the
// whole banner is always on screen and the gradient head visibly retraces it.

// fourierText is the Fourier Text mode: the banner, and whether the mode is
// in residence.
type fourierText struct {
	FourierText

	// ftext.active tracks mode residency (entry setup once per entry, like
	// pong.active — panel rebuilds must not re-normalize the pose).
	active bool
}

var ftext = fourierText{FourierText: *NewFourierText()}

// generateScopeText streams every glyph's beam window into the trail buffer.
// Each glyph runs its own closed circuit (all at the same phase, like a bank
// of character generators sharing one clock); the strip's short
// glyph-to-glyph connectors are the multiplexer hand-off an unblanked scope
// would show.
func (f *fourierText) generateScopeText() {
	drawn := f.Strokes()
	if len(drawn) == 0 || sim.steps < 2 {
		return
	}
	t := f.Sweep(1, sim.speedSteps, sim.speedScale)
	if v := beamLines(drawn, t); v > 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:v*4], beamDrawMode(), v)
	}
}

// syncScopeTextExtras tracks whether Fourier Text is the
// active model, and normalizes the pose on entry — a banner reads face-on.
// (The text field itself is static markup wired in buildDemoModules.)
func (f *fourierText) syncScopeTextExtras(mode string) {
	// The module is in the rack whatever the model (panelhtml_js.go): the rack
	// is the same instrument under every model, so its panels do not come and go.
	if mode != "scopetext" {
		f.active = false
		return
	}
	if !f.active {
		f.active = true
		normalizeOrientation()
	}
}
