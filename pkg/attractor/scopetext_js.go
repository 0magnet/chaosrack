//go:build js && wasm

package attractor

// Mode wiring for Fourier Text (the harmonic character generator lives in
// scopetext.go). The beam sweeps the reconstructed curve continuously like
// the Lissajous mode: the drawn window is exactly one period, so the whole
// banner is always on screen and the gradient head visibly retraces it.

var (
	scopeTextStr          = "CHAOSRACK"
	scopeTextHarm float32 = 24 // harmonics kept per glyph (the knob)
	scopeTextKeyS string       // cache keys: text…
	scopeTextKeyH int          // …and harmonic count
	scopeTextT    float64      // beam phase, 0..1 of the banner sweep
	scopeTextRes  = 1024       // reconstruction samples per glyph
)

// generateScopeText rebuilds the per-glyph harmonic reconstructions when
// the text or the harmonics knob changed, then streams every glyph's beam
// window into the trail buffer. Each glyph runs its own closed circuit
// (all at the same phase, like a bank of character generators sharing one
// clock); the strip's short glyph-to-glyph connectors are the multiplexer
// hand-off an unblanked scope would show.
func generateScopeText() {
	h := int(scopeTextHarm + 0.5)
	if h < 1 {
		h = 1
	}
	if scopeTextDrawn == nil || scopeTextKeyS != scopeTextStr || scopeTextKeyH != h {
		glyphs := scopeTextGlyphStrokes(scopeTextStr)
		scopeTextDrawn = scopeTextDrawn[:0]
		for _, g := range glyphs {
			c := scopeTextSynth(g, h, scopeTextRes)
			if c == nil {
				continue
			}
			// Blank the reconstruction across the tour's retrace spans — the
			// z-axis keying a hardware character generator would apply. The
			// glyph-to-glyph hand-off is likewise never drawn (separate
			// strokes), so no beam appears anywhere it shouldn't.
			scopeTextDrawn = append(scopeTextDrawn, scopeTextSplitCurve(c, scopeTextJumpFractions(g))...)
		}
		scopeTextKeyS, scopeTextKeyH = scopeTextStr, h
	}
	if len(scopeTextDrawn) == 0 || steps < 2 {
		return
	}
	// One full period ≈ 3 s at speed 1, scaled like the integrators; the
	// phase sweeps the gradient along the banner.
	scopeTextT += float64(speedScale) * float64(speedSteps) / 180
	for scopeTextT >= 1 {
		scopeTextT--
	}
	if v := beamLines(scopeTextDrawn, scopeTextT); v > 0 {
		uploadVerticesOnly(vertBuf[:v*4], beamDrawMode(), v)
	}
}

// scopeTextActive tracks mode residency (entry setup once per entry, like
// pongActive — panel rebuilds must not re-normalize the pose).
var (
	scopeTextActive bool
	scopeTextDrawn  [][]float64 // cached drawable strokes (blanked circuits)
)

// syncScopeTextExtras shows the Banner module while Fourier Text is the
// active model, and normalizes the pose on entry — a banner reads face-on.
// (The text field itself is static markup wired in buildDemoModules.)
func syncScopeTextExtras(mode string) {
	if sect := doc.Call("getElementById", "stext-module"); sect.Truthy() {
		if mode == "scopetext" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if mode != "scopetext" {
		scopeTextActive = false
		return
	}
	if !scopeTextActive {
		scopeTextActive = true
		normalizeOrientation()
	}
}
