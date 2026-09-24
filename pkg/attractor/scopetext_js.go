//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/scope"
)

// Mode wiring for Fourier Text (the harmonic character generator lives in
// pkg/scope/text.go). The beam sweeps the reconstructed curve continuously like
// the Lissajous mode: the drawn window is exactly one period, so the whole
// banner is always on screen and the gradient head visibly retraces it.

// fourierText is the Fourier Text mode: the string, the harmonics that draw
// it and its clock.
type fourierText struct {
	str  string
	harm float32 // harmonics kept per glyph (the knob)
	keyS string  // cache keys: text…
	keyH int     // …and harmonic count
	t    float64 // beam phase, 0..1 of the banner sweep

	// ftext.active tracks mode residency (entry setup once per entry, like
	// pong.active — panel rebuilds must not re-normalize the pose).
	active bool
	drawn  [][]float64 // cached drawable strokes (blanked circuits)
}

var ftext = fourierText{
	str:  "CHAOSRACK",
	harm: 24,
}

var (
	scopeTextRes = 1024 // reconstruction samples per glyph
)

// generateScopeText rebuilds the per-glyph harmonic reconstructions when
// the text or the harmonics knob changed, then streams every glyph's beam
// window into the trail buffer. Each glyph runs its own closed circuit
// (all at the same phase, like a bank of character generators sharing one
// clock); the strip's short glyph-to-glyph connectors are the multiplexer
// hand-off an unblanked scope would show.
func (f *fourierText) generateScopeText() {
	h := int(f.harm + 0.5)
	if h < 1 {
		h = 1
	}
	if f.drawn == nil || f.keyS != f.str || f.keyH != h {
		glyphs := scope.TextGlyphStrokes(f.str)
		f.drawn = f.drawn[:0]
		for _, g := range glyphs {
			c := scope.TextSynth(g, h, scopeTextRes)
			if c == nil {
				continue
			}
			// Blank the reconstruction across the tour's retrace spans — the
			// z-axis keying a hardware character generator would apply. The
			// glyph-to-glyph hand-off is likewise never drawn (separate
			// strokes), so no beam appears anywhere it shouldn't.
			f.drawn = append(f.drawn, scope.TextSplitCurve(c, scope.TextJumpFractions(g))...)
		}
		f.keyS, f.keyH = f.str, h
	}
	if len(f.drawn) == 0 || sim.steps < 2 {
		return
	}
	// One full period ≈ 3 s at speed 1, scaled like the integrators; the
	// phase sweeps the gradient along the banner.
	f.t += float64(sim.speedScale) * float64(sim.speedSteps) / 180
	for f.t >= 1 {
		f.t--
	}
	if v := beamLines(f.drawn, f.t); v > 0 {
		gpu.uploadVerticesOnly(sim.vertBuf[:v*4], beamDrawMode(), v)
	}
}

// syncScopeTextExtras shows the Banner module while Fourier Text is the
// active model, and normalizes the pose on entry — a banner reads face-on.
// (The text field itself is static markup wired in buildDemoModules.)
func (f *fourierText) syncScopeTextExtras(mode string) {
	if sect := dom.Doc.Call("getElementById", "stext-module"); sect.Truthy() {
		if mode == "scopetext" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if mode != "scopetext" {
		f.active = false
		return
	}
	if !f.active {
		f.active = true
		normalizeOrientation()
	}
}
