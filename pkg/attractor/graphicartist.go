package attractor

import "math"

// GraphicArtist is the Graphic Artist's four oscillators, shared by the page
// (graphicartist_js.go) and a native rack.
type GraphicArtist struct {
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

// NewGraphicArtist is the Graphic Artist at the page's defaults.
func NewGraphicArtist() *GraphicArtist {
	return &GraphicArtist{levelA: 0.55, levelB: 0.45, levelD: 0.55, harmB: 2, harmC: 12, harmD: 1}
}

// GraphicArtistKnobs are its knobs, by the page's ids.
var GraphicArtistKnobs = []string{"ga-la", "ga-lb", "ga-ld", "ga-hb", "ga-hc", "ga-hd", "ga-wa", "ga-wb", "ga-wc", "ga-wd"}

// Set turns knob id to v, reporting whether the Graphic Artist has it.
func (g *GraphicArtist) Set(id string, v float32) bool {
	p := map[string]*float32{
		"ga-la": &g.levelA, "ga-lb": &g.levelB, "ga-ld": &g.levelD,
		"ga-hb": &g.harmB, "ga-hc": &g.harmC, "ga-hd": &g.harmD,
		"ga-wa": &g.waveA, "ga-wb": &g.waveB, "ga-wc": &g.waveC, "ga-wd": &g.waveD,
	}[id]
	if p == nil {
		return false
	}
	*p = v
	return true
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

// Drift moves the figure on by frames of the page's, at speed scale: the
// slow global drift that makes it "revolve/oscillate" as the article
// describes.
func (g *GraphicArtist) Drift(frames float64, scale float32) {
	g.phase += float32(0.006 * frames * float64(scale))
	if g.phase > 1e6 {
		g.phase = 0
	}
}

// Trace hands put one closed pass of the Lissajous wireframe, steps points,
// at the phase it has drifted to.
func (g *GraphicArtist) Trace(steps int, put func(i int, x, y, z float64)) {
	invN := 0.0
	if steps > 1 {
		invN = 1 / float64(steps-1)
	}
	const q = math.Pi / 4     // ±45° carrier phase split
	const scale = 1.5         // fill the view
	const dQuad = math.Pi / 2 // 90° A↔D offset so the base traces a rectangle, not a diagonal line
	// Sweep the master phase over one full 2π cycle; because B/C/D are integer
	// harmonics the whole figure closes in that span.
	span := 2 * math.Pi
	for i := range steps {
		t := float64(i)*invN*span + float64(g.phase)
		a := gaWave(g.waveA, t)
		b := gaWave(g.waveB, t*float64(g.harmB))
		d := gaWave(g.waveD, t*float64(g.harmD)+dQuad)
		cp := gaWave(g.waveC, t*float64(g.harmC)+q)
		cm := gaWave(g.waveC, t*float64(g.harmC)-q)
		env := float64(g.levelB) * b
		x := (float64(g.levelD)*d + env*cm) * scale
		y := (float64(g.levelA)*a + env*cp) * scale
		z := env * (cp - cm) * 0.7 * scale // perpendicular component → real depth
		put(i, x, y, z)
	}
}
