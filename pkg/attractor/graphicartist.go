//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"math"
	"syscall/js"
)

// graphicArtist is the Graphic Artist's four oscillators.
type graphicArtist struct {
	// The Graphic Artist — a digital re-creation of Mitchell Waite's 1975 Popular
	// Electronics "Oscilloscope Graphic Artist" (Nov 1975), which drives a scope's
	// X/Y inputs with two harmonically-related signals to draw 3D-looking Lissajous
	// wireframes.
	//
	// Model (from the article): four relaxation oscillators A/B/C/D, each selectable
	// square or triangle. A is the fixed master; B/C/D lock to it at INTEGER
	// harmonics (this is the sync the original circuit was supposed to enforce — a
	// wiring error on a sync input let them free-run, which is why real builds drew
	// an unstable, unimpressive blur). Here we get perfect lock for free by making
	// B/C/D exact integer multiples of A, so the figure is always a stable closed
	// curve.
	//
	// 	carrier C is split ±45°; the B envelope modulates each phase:
	// 	  VERT  = levelA·A + levelB·(B · C+45)
	// 	  HORIZ = levelD·D + levelB·(B · C-45)
	//
	// The two perpendicular modulated components are what create the 3D volume
	// illusion on a real (2D) scope; we also lift that component onto a real Z so
	// the figure has genuine depth and rotates in the 3D pipeline.
	levelA float32
	levelB float32
	levelD float32
	harmB  float32 // integer harmonics of the master (A)
	harmC  float32 // carrier — higher = denser wireframe hatching
	harmD  float32 // D at the fundamental → base rectangle vs A
	waveA  float32 // 0 = triangle, 1 = square
	waveB  float32
	waveC  float32
	waveD  float32
	phase  float32 // advances each frame → the figure slowly drifts/animates
}

var ga = graphicArtist{
	levelA: 0.55,
	levelB: 0.45,
	levelD: 0.55,
	harmB:  2,
	harmC:  12,
	harmD:  1,
}

// gaWave returns a −1..1 waveform sample at phase ph (radians). kind rounds to
// triangle (0) or square (1) — matching the WAVEFORM A/B/C/D toggle switches.
func gaWave(kind float32, ph float64) float64 {
	p := ph / (2 * math.Pi)
	p -= math.Floor(p) // fractional cycle 0..1
	if kind >= 0.5 {   // square
		if p < 0.5 {
			return 1
		}
		return -1
	}
	// triangle: 0 → +1 → 0 → −1 → 0
	switch {
	case p < 0.25:
		return 4 * p
	case p < 0.75:
		return 2 - 4*p
	default:
		return 4*p - 4
	}
}

// generateGraphicArtist traces one closed pass of the Lissajous wireframe into
// the vertex buffer, animated by gaPhase. Called every frame from
// generateForMode, so it continuously renders like the attractors.
func (g *graphicArtist) generateGraphicArtist() {
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	// Slow global drift so the figure "revolves/oscillates" as the article
	// describes, scaled by the speed control.
	g.phase += 0.006 * sim.speedScale
	if g.phase > 1e6 {
		g.phase = 0
	}
	const q = math.Pi / 4     // ±45° carrier phase split
	const scale = 1.5         // fill the view
	const dQuad = math.Pi / 2 // 90° A↔D offset so the base traces a rectangle, not a diagonal line
	// Sweep the master phase over one full 2π cycle; because B/C/D are integer
	// harmonics the whole figure closes in that span.
	span := 2 * math.Pi
	for i := 0; i < sim.steps; i++ {
		t := float64(i)*invN64()*span + float64(g.phase)
		a := gaWave(g.waveA, t)
		b := gaWave(g.waveB, t*float64(g.harmB))
		d := gaWave(g.waveD, t*float64(g.harmD)+dQuad)
		cp := gaWave(g.waveC, t*float64(g.harmC)+q)
		cm := gaWave(g.waveC, t*float64(g.harmC)-q)
		env := float64(g.levelB) * b
		x := (float64(g.levelD)*d + env*cm) * scale
		y := (float64(g.levelA)*a + env*cp) * scale
		z := env * (cp - cm) * 0.7 * scale // perpendicular component → real depth
		j := i * 4
		vertices[j] = float32(x)
		vertices[j+1] = float32(y)
		vertices[j+2] = float32(z)
		vertices[j+3] = float32(i) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

// invN64 is 1/(steps-1) in float64 for the phase sweep.
func invN64() float64 {
	if sim.steps <= 1 {
		return 0
	}
	return 1 / float64(sim.steps-1)
}

// syncGAWaveSwitches shows the WAVEFORM A/B/C/D toggles (triangle ↔ square) in
// the Switches module while Graphic Artist is the active model, and removes
// them otherwise — the article's S1–S4 switches that break the figure into its
// 16 waveform "families".
func (g *graphicArtist) syncGAWaveSwitches(mode string) {
	if ex := dom.Doc.Call("getElementById", "ga-waves"); ex.Truthy() {
		ex.Get("parentNode").Call("removeChild", ex)
	}
	if mode != "graphicartist" {
		return
	}
	swrow := dom.Doc.Call("querySelector", ".swrow")
	if !swrow.Truthy() {
		return
	}
	wrap := dom.Doc.Call("createElement", "div")
	wrap.Set("id", "ga-waves")
	wrap.Set("className", "ga-waves grp")
	hdr := dom.Doc.Call("createElement", "div")
	hdr.Set("className", "ga-waves-hdr")
	hdr.Set("textContent", "WAVEFORM △/⊓")
	wrap.Call("appendChild", hdr)
	defs := []struct {
		lbl string
		ptr *float32
	}{{"A", &g.waveA}, {"B", &g.waveB}, {"C", &g.waveC}, {"D", &g.waveD}}
	for _, d := range defs {
		ptr := d.ptr
		lab := dom.Doc.Call("createElement", "label")
		lab.Set("className", "grp ga-wave")
		lab.Set("title", "Oscillator "+d.lbl+" waveform: off = triangle (smooth), on = square (breaks the figure up)")
		cb := dom.Doc.Call("createElement", "input")
		cb.Set("type", "checkbox")
		cb.Set("className", "sw")
		if *ptr >= 0.5 {
			cb.Set("checked", true)
		}
		cb.Call("addEventListener", "change", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
			if cb.Get("checked").Bool() {
				*ptr = 1
			} else {
				*ptr = 0
			}
			return nil
		}))
		txt := dom.Doc.Call("createElement", "span")
		txt.Set("textContent", " "+d.lbl)
		lab.Call("appendChild", cb)
		lab.Call("appendChild", txt)
		wrap.Call("appendChild", lab)
	}
	swrow.Call("appendChild", wrap)
}
