//go:build js && wasm

package attractor

import (
	"strings"
	"syscall/js"
)

// Mode wiring for Fourier Text (the harmonic character generator lives in
// scopetext.go). The beam sweeps the reconstructed curve continuously like
// the Lissajous mode: the drawn window is exactly one period, so the whole
// banner is always on screen and the gradient head visibly retraces it.

var (
	scopeTextStr                = "CHAOSRACK"
	scopeTextHarm   float32     = 24 // harmonics kept per glyph (the knob)
	scopeTextCurves [][]float64      // cached per-glyph reconstructions
	scopeTextKeyS   string           // cache keys: text…
	scopeTextKeyH   int              // …and harmonic count
	scopeTextT      float64          // beam phase, 0..1 of a glyph period
	scopeTextRes    = 1024           // reconstruction samples per glyph
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
	if scopeTextCurves == nil || scopeTextKeyS != scopeTextStr || scopeTextKeyH != h {
		glyphs := scopeTextGlyphStrokes(scopeTextStr)
		scopeTextCurves = scopeTextCurves[:0]
		for _, g := range glyphs {
			if c := scopeTextSynth(g, h, scopeTextRes); c != nil {
				scopeTextCurves = append(scopeTextCurves, c)
			}
		}
		scopeTextKeyS, scopeTextKeyH = scopeTextStr, h
	}
	ng := len(scopeTextCurves)
	if ng == 0 || steps < 2 {
		return
	}
	// One full period ≈ 3 s at speed 1, scaled like the integrators.
	scopeTextT += float64(speedScale) * float64(speedSteps) / 180
	for scopeTextT >= 1 {
		scopeTextT--
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	per := steps / ng // trail points per glyph
	for i := 0; i < steps; i++ {
		g := i / per
		if g >= ng {
			g = ng - 1
		}
		curve := scopeTextCurves[g]
		res := len(curve) / 2
		gi := i - g*per
		gn := per
		if g == ng-1 {
			gn = steps - g*per // last glyph absorbs the remainder
		}
		// This glyph's window is exactly one period, head at the shared phase.
		s := (scopeTextT + float64(gi)/float64(gn)) * float64(res)
		k := int(s)
		f := s - float64(k)
		if k >= res {
			k -= res
		}
		k2 := (k + 1) % res
		j := i * 4
		vertices[j] = float32(curve[k*2] + (curve[k2*2]-curve[k*2])*f)
		vertices[j+1] = float32(curve[k*2+1] + (curve[k2*2+1]-curve[k*2+1])*f)
		vertices[j+2] = 0
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

// scopeTextActive tracks mode residency (entry setup once per entry, like
// pongActive — panel rebuilds must not re-normalize the pose).
var scopeTextActive bool

// syncScopeTextExtras shows a TEXT field in the Console's switch row while
// Fourier Text is the active model (the Graphic Artist waveform-switch
// pattern), and normalizes the pose on entry — a banner reads face-on.
func syncScopeTextExtras(mode string) {
	if ex := doc.Call("getElementById", "stext-ui"); ex.Truthy() {
		ex.Get("parentNode").Call("removeChild", ex)
	}
	if mode != "scopetext" {
		scopeTextActive = false
		return
	}
	if !scopeTextActive {
		scopeTextActive = true
		normalizeOrientation()
	}
	swrow := doc.Call("querySelector", ".swrow")
	if !swrow.Truthy() {
		return
	}
	wrap := doc.Call("createElement", "div")
	wrap.Set("id", "stext-ui")
	wrap.Set("className", "ga-waves grp")
	hdr := doc.Call("createElement", "div")
	hdr.Set("className", "ga-waves-hdr")
	hdr.Set("textContent", "TEXT")
	wrap.Call("appendChild", hdr)
	in := doc.Call("createElement", "input")
	in.Set("type", "text")
	in.Set("id", "stext-in")
	in.Set("className", "stext-in")
	in.Set("value", scopeTextStr)
	in.Set("maxLength", 24)
	in.Set("title", "Banner text — A–Z, 0–9, dash and space; drawn from the kept harmonics (turn the harm knob down to melt it)")
	in.Call("setAttribute", "data-no-drag", "")
	in.Call("addEventListener", "input", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		scopeTextStr = strings.ToUpper(in.Get("value").String())
		return nil
	}))
	wrap.Call("appendChild", in)
	swrow.Call("appendChild", wrap)
}
