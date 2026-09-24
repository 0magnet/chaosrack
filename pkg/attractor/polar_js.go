//go:build js && wasm

package attractor

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/takens"
)

// Polar embedding — the Takens delay vector drawn in a SPHERE instead of a cube.
//
// The Takens mode plots (s(t), s(t−τ), s(t−2τ)) coordinate by coordinate: each
// axis is one sample, bounded to ±1 and multiplied by GAIN. The reachable set
// of that is a cube, and the cube is visible in the picture — a loud passage
// piles up against the faces, and against the corners hardest of all, because
// the three coordinates of a signal with strong low-frequency content peak
// together (that is what takensCubeDiag exists for). The bounding box is an
// artifact of writing the vector down in coordinates. Nothing in the signal
// knows about the axes.
//
// So this mode keeps the vector and replaces the bound. A delay vector v is
// split into a DIRECTION v/|v| and a LENGTH |v|; the direction is kept exactly
// as it is, and the length is passed through a map that cannot exceed 1. The
// figure then lives in the ball of radius GAIN, and a loud passage saturates
// toward a sphere — which is round in every direction, so no rotation of the
// model finds an edge that is really the arithmetic showing through.
//
// WHAT THE MAP CHANGES, AND WHAT IT MUST NOT. Only the length. The angles
// between successive delay vectors are what the embedding is FOR — they are the
// geometry Takens' theorem says is diffeomorphic to the source system's
// attractor — and a map that touched them would be drawing a different manifold
// and calling it a reconstruction. Every map here multiplies all three
// coordinates by the same non-negative factor, so the direction survives
// exactly and the compression is visible only as radius.
//
// The MAP knob picks which one:
//
//   - tanh: r ↦ tanh(drive·r). Smooth, odd, asymptotic to 1, and near-linear
//     for small r — so quiet material is drawn the size the Takens mode would
//     draw it and only the loud end is bent. It is the soft clipper, which is
//     the same reason it is the standard nonlinearity for this shape of job.
//   - algebraic: r ↦ drive·r/(1 + drive·r). The same asymptote reached more
//     slowly: at the same drive it sits below tanh everywhere past the origin,
//     so the mid-range keeps more of its dynamics and the approach to the
//     surface takes longer. Cheaper too, though that is not why it is here.
//   - direction only: r ↦ 1. Loudness is removed ENTIRELY and every point
//     lands on the sphere's surface. What is left is the angular part of the
//     trajectory, which is the part the amplitude was hiding: a figure that
//     merely got bigger and smaller before now moves, and one that was
//     genuinely wandering still wanders. DRIVE does nothing on this position,
//     and descriptions.go says so — a knob left plainly inert with nothing
//     explaining it reads as a knob that has stopped working. This is the
//     Stereo Embedding's τ on its two time positions, and the same answer:
//     one knob with two meanings that swap under another knob would be worse.
//
// DRIVE is how hard the signal is pushed into the map. It defaults to 2 rather
// than 1 because at 1 the map is nearly the identity for ordinary program
// material — an RMS of maybe 0.1 gives tanh(0.1) = 0.0997 — so the default view
// would be the Takens mode with the corners very slightly rounded, i.e. a mode
// that appears to do nothing. At 2 the compression is visible at the top and
// quiet passages are drawn about twice the size, which is the mode showing what
// it is for on the first look.
//
// Everything else is deliberately the Takens mode's, for the reasons argued
// over there and unchanged by the bounding: takensWindow's window / stride /
// budget arithmetic, takensSmooth's Catmull-Rom beam, takensVerts, the fixed
// scale with nothing auto-ranging, and the one-shot camera fit.

// polarMapNames are the dial's positions by name — the tooltip on each detent.
var polarMapNames = []string{"tanh", "algebraic", "logarithmic (dB)", "direction only"}

// polarMapRing is what fits AROUND the dial: five runes at most, and for a
// named setting the ring IS the readout (the LED is hidden), so these have to
// be told apart at a glance. "unit" for the direction-only map because that is
// what it draws — the unit sphere — and "sphr" would describe all three.
var polarMapRing = []string{"tanh", "soft", "dB", "unit"}

// polarMode is the polar embedding mode: the window of audio it embeds and
// its knobs.
type polarMode struct {
	mapF    float32 // map knob: an index into the constants above
	drive   float32 // how hard the length is pushed into the map
	win     float32 // display window, milliseconds
	gain    float32 // world units the sphere's surface sits at
	ring    []float32
	w       int // monotonic write cursor into polar.ring
	scratch []float32
	cursor  int // read position in the shared audio tap

	// chanF is the SRC knob, the Takens mode's and for its reason: the
	// delay vector is built from ONE observable, and which one is a choice the
	// mode could not offer while the tap carried only the mix.
	chanF float32

	// fitGain is the GAIN the camera was last fitted to, 0 for not yet.
	// A gain rather than a bool for emb.fitGain's reason: the bound the fit
	// is made against is a function of the gain — here the sphere's radius IS
	// the gain — so a fit made at one gain is not a fit at another.
	fitGain float32
}

var polar = polarMode{
	drive:  2,
	win:    85,
	gain:   10,
	cursor: tapUnjoined,
}

func init() {
	registerGenerate("polar", polar.generatePolar)
	attractorParams["polar"] = []paramDef{
		{"polar-chan", "src", &polar.chanF, 0, 0, float32(len(tapChanNames) - 1), 1},
		{"polar-map", "map", &polar.mapF, 0, 0, float32(takens.PolarCount - 1), 1},
		{"polar-drive", "drv", &polar.drive, 2, 0.2, 10, 0.1},
		// τ is takens-tau, not a polar copy of it: the polar figure IS the takens
		// delay embedding with the delay wrapped onto an angle, so a τ that
		// differed between them would be two names for one quantity — and the
		// recurrence plot and takens-smooth above already share this way.
		{"takens-tau", "τ", &emb.tau, takens.TauDef, 1, takens.TauMax, 1},
		{"polar-win", "win", &polar.win, 85, 5, 500, 5},
		{"polar-gain", "gain", &polar.gain, 10, 0.5, 50, 0.5},
		{"takens-smooth", "smth", &takensSmoothF, 4, 1, 16, 1},
	}
}

// mapSel is the map knob as an index, clamped.
//
// Audio modulation can drive any registered parameter, this one included, so
// the value arriving here is not necessarily one of the detents — and a
// modulator riding a feature that has gone to zero or to infinity can hand over
// an out-of-range float or a NaN. The range is checked BEFORE the conversion,
// not after: a float-to-int conversion whose value does not fit is
// implementation-defined in Go, so clamping the RESULT of int(±Inf + 0.5) is
// clamping whatever the runtime happened to produce. NaN falls out of the same
// comparison, since every comparison against a NaN is false. (This is
// stereoAxisSel's argument, and the same trap.)
func (p *polarMode) mapSel() int {
	v := p.mapF
	if !(v > 0) { // false for NaN too
		return 0
	}
	if v > float32(takens.PolarCount-1) {
		return takens.PolarCount - 1
	}
	return int(v + 0.5)
}

// polarFitExtent is the extent the camera is fitted to, and it is GAIN — not
// gain·√3.
//
// The √3 in takensFitExtent is the CUBE'S CORNER: the Takens mode's three
// coordinates are three independent samples, each bounded by gain, so the
// vector can reach √3·gain when all three peak together. Here the bound is on
// the vector's LENGTH rather than on its coordinates, takens.PolarRadius never exceeds
// 1, and the reachable set is therefore the ball of radius gain exactly. There
// is no corner to leave room for.
//
// That the bound holds for the drawn beam and not merely for the plotted points
// is a consequence of applying the map AFTER the spline; see generatePolar,
// where the ordering is argued and the factor it avoids is worked out.
func polarFitExtent(gain float32) float32 { return gain }

// generatePolar drains the audio source into a persistent ring and draws the
// newest window of delay vectors, radius-mapped. When there is no (or not yet
// enough) audio the previous frame is re-uploaded, so the model does not
// flicker while the source spins up — generateTakens' behavior, and its ring
// arithmetic, which is shared code up to the map.
func (p *polarMode) generatePolar() {
	src := aud.ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(emb.tau, sr)
	n, stride := takensWindow(p.win, sr, sim.steps)
	span := (n-1)*stride + 2*tau
	if need := span + 1; len(p.ring) < need {
		p.ring = make([]float32, need+need/2)
		p.w = 0
	}
	if p.scratch == nil {
		p.scratch = make([]float32, 8192)
	}
	if tap.ready() {
		// Its own cursor into the shared tap, as every frame-loop consumer has:
		// Drain hands each sample over exactly once, so two consumers on the
		// raw source would split the stream rather than each see it.
		for drained := 0; drained < 16384; {
			got := tap.readChan(&p.cursor, p.scratch, tapChanSel(p.chanF))
			if got <= 0 {
				break
			}
			for i := 0; i < got; i++ {
				p.ring[p.w%len(p.ring)] = p.scratch[i]
				p.w++
			}
			drained += got
			if got < len(p.scratch) {
				break
			}
		}
	}
	avail := p.w
	if avail > len(p.ring) {
		avail = len(p.ring)
	}
	nv := takensVerts(n)
	if avail < span+1 {
		p.fitGain = 0 // camera was fitted to silence — refit on real data
		gpu.uploadVerticesOnly(sim.vertBuf[:nv*4], gpu.drawMode, nv)
		return
	}
	rn := len(p.ring)
	base := p.w - 1 - span
	g := p.gain
	mapSel := p.mapSel()
	drive := p.drive

	// at reads source point k at delay offset off, clamping k to the window so
	// the spline's outer control points at either end are defined.
	at := func(k, off int) float32 {
		if k < 0 {
			k = 0
		} else if k > n-1 {
			k = n - 1
		}
		return p.ring[(base+2*tau+k*stride+off)%rn]
	}
	invN := float32(1) / float32(nv-1)
	vertices := sim.vertBuf[:nv*4]
	var v [3]float32
	sm := takensSmooth()
	for m := 0; m < nv; m++ {
		i := m / sm
		f := float32(m%sm) / float32(sm)
		j := m * 4
		for c, off := range [3]int{0, -tau, -2 * tau} {
			p0, p1, p2, p3 := at(i-1, off), at(i, off), at(i+1, off), at(i+2, off)
			// Catmull-Rom through p1..p2, exactly as takensSmooth documents:
			// a straight LINE_STRIP between delay vectors draws chords, and
			// the chords are an artifact of the drawing rather than something
			// in the signal.
			v[c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f)
		}
		// THE MAP IS APPLIED AFTER THE SPLINE, and the order is load-bearing.
		//
		// Mapping each source point first and interpolating the results would
		// be the obvious arrangement and it does not stay inside the sphere: a
		// Catmull-Rom is not confined to the hull of its control points. Its
		// four basis weights sum, in absolute value, to 1 + f − f² — a maximum
		// of 1.25 at f = ½ — so four points ON the unit sphere with the outer
		// two roughly opposite the inner two draw a curve reaching 1.25. The
		// figure would then leave the ball this mode exists to bound it in, and
		// the camera fit would have to carry a fudge factor that means nothing
		// to anybody.
		//
		// Mapping the interpolated vector instead bounds every point that is
		// actually drawn, by construction, and it is the more defensible thing
		// anyway: the spline is the bandlimited reconstruction of the signal
		// BETWEEN samples, and the radius map is a property of the display, so
		// the display map belongs on the reconstructed signal rather than on
		// the samples it was reconstructed from.
		s := takens.PolarScale(mapSel, float32(math.Sqrt(float64(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))), drive) * g
		vertices[j+0] = v[0] * s
		vertices[j+1] = v[1] * s
		vertices[j+2] = v[2] * s
		// The trail ramp, which is also what acolor.window's table indexes.
		vertices[j+3] = float32(m) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, nv)
	if p.fitGain != p.gain && !pmod.paramIsModulated("polar-gain") {
		// Fitted to the fixed scale's worst case rather than to this window's
		// extent, and only when GAIN moves that case — see generateTakens for
		// why fitting the instantaneous figure put loud passages off the
		// screen. Here the worst case is the sphere, so the fit is exact and
		// not merely safe.
		p.fitGain = p.gain
		view.fitOverride = polarFitExtent(p.gain)
		view.autoFitCamera()
	}
}

// colorWindow is acolor.window for this mode.
//
// It qualifies for the same reason Takens does: its vertices carry
// aTrailT = m/(nv−1), a straight ramp across the displayed window, so slot k of
// the color table lines up with the k'th slice of that window. Without it the
// mode would fall through to the flat fill, and with the gradient following the
// sound that fill is one color for the whole trail — which can be the dark end
// of the palette, at which point a correct figure is drawn in black on black
// and the mode looks broken. That is not hypothetical: it is exactly what the
// Stereo Embedding did until it was added there.
//
// The walk mirrors generatePolar, including the 2*tau offset: source point k is
// read at base+2*tau+k*stride because the two delayed coordinates reach BACK
// from there. Sampling from base instead would color the trail with audio from
// two delays earlier than the trail was drawn from.
//
// The RAW samples, not the mapped radius. The color says what was playing where
// the trail was drawn from, and the radius map is a display decision — coloring
// by it would make the gradient repeat what the geometry already shows.
func (p *polarMode) colorWindow() ([]float32, int) {
	if p.ring == nil {
		return nil, 0
	}
	src := aud.ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(emb.tau, sr)
	n, stride := takensWindow(p.win, sr, sim.steps)
	if n <= 0 {
		return nil, 0
	}
	rn := len(p.ring)
	span := (n-1)*stride + 2*tau
	if rn == 0 || p.w < span+1 {
		return nil, 0 // not enough audio yet; the flat fill is the honest answer
	}
	if cap(acolor.win) < n {
		acolor.win = make([]float32, n)
	}
	out := acolor.win[:n]
	base := p.w - 1 - span
	for k := 0; k < n; k++ {
		out[k] = p.ring[(base+2*tau+k*stride)%rn]
	}
	return out, sr
}
