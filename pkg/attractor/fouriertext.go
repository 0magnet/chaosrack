package attractor

import "github.com/0magnet/chaosrack/pkg/scope"

// Fourier Text's banner, shared by the page (scopetext_js.go) and a native
// rack: the harmonic character generator is in pkg/scope (text.go); this is
// its string, its harmonics knob, the strokes they make and the beam's phase.

// FourierText is the banner: the string, the harmonics that draw it and its
// clock.
type FourierText struct {
	str   string
	harm  float32     // harmonics kept per glyph (the knob)
	keyS  string      // cache keys: text…
	keyH  int         // …and harmonic count
	t     float64     // beam phase, 0..1 of the banner sweep
	drawn [][]float64 // cached drawable strokes (blanked circuits)
}

// NewFourierText is the banner at the page's defaults.
func NewFourierText() *FourierText { return &FourierText{str: "CHAOSRACK", harm: 24} }

// scopeTextRes is the reconstruction samples per glyph.
var scopeTextRes = 1024

// Set turns the harmonics knob, stext-harm, reporting whether it is that.
func (f *FourierText) Set(id string, v float32) bool {
	if id != "stext-harm" {
		return false
	}
	f.harm = v
	return true
}

// SetText is the banner's text.
func (f *FourierText) SetText(s string) { f.str = s }

// Strokes are the banner's glyphs as drawable strokes, each x,y pairs,
// rebuilt when the text or the harmonics knob changed.
func (f *FourierText) Strokes() [][]float64 {
	h := max(int(f.harm+0.5), 1)
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
	return f.drawn
}

// Sweep moves the beam's phase on by frames of the page's at its speed
// (steps a frame, each scale): one full period ≈ 3 s at speed 1, scaled like
// the integrators. It reports the phase, which sweeps the gradient along the
// banner.
func (f *FourierText) Sweep(frames float64, steps int, scale float32) float64 {
	f.t += frames * float64(scale) * float64(steps) / 180
	for f.t >= 1 {
		f.t--
	}
	return f.t
}
