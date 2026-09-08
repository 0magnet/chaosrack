//go:build js && wasm

package attractor

import "math"

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

// The radius maps, in knob order.
const (
	polarMapTanh = iota
	polarMapAlgebraic
	polarMapUnit
	polarMapCount
)

// polarMapNames are the dial's positions by name — the tooltip on each detent.
var polarMapNames = []string{"tanh", "algebraic", "direction only"}

// polarMapRing is what fits AROUND the dial: five runes at most, and for a
// named setting the ring IS the readout (the LED is hidden), so these have to
// be told apart at a glance. "unit" for the direction-only map because that is
// what it draws — the unit sphere — and "sphr" would describe all three.
var polarMapRing = []string{"tanh", "soft", "unit"}

var (
	polarMapF    float32 = 0  // map knob: an index into the constants above
	polarDrive   float32 = 2  // how hard the length is pushed into the map
	polarTau     float32 = 32 // delay τ, in source samples
	polarWin     float32 = 85 // display window, milliseconds
	polarGain    float32 = 10 // world units the sphere's surface sits at
	polarRing    []float32
	polarW       int // monotonic write cursor into polarRing
	polarScratch []float32
	polarCursor  = tapUnjoined // read position in the shared audio tap
	polarFitted  bool          // camera fitted since real audio arrived
)

func init() {
	registerGenerate("polar", generatePolar)
	attractorParams["polar"] = []paramDef{
		{"polar-map", "map", &polarMapF, 0, 0, float32(polarMapCount - 1), 1},
		{"polar-drive", "drv", &polarDrive, 2, 0.2, 10, 0.1},
		{"polar-tau", "τ", &polarTau, 32, 1, 512, 1},
		{"polar-win", "win", &polarWin, 85, 5, 500, 5},
		{"polar-gain", "gain", &polarGain, 10, 0.5, 50, 0.5},
	}
}

// polarMapSel is the map knob as an index, clamped.
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
func polarMapSel() int {
	v := polarMapF
	if !(v > 0) { // false for NaN too
		return 0
	}
	if v > float32(polarMapCount-1) {
		return polarMapCount - 1
	}
	return int(v + 0.5)
}

// polarRadius maps a delay vector's length to the length it is DRAWN at, as a
// fraction of GAIN. Every map returns a value in [0, 1] for every non-negative
// r and every positive drive, which is the whole property this mode is built
// on and what polarFitExtent relies on.
//
// A negative or non-finite r is answered with 0 rather than propagated. r comes
// from a square root of a sum of squares of audio samples, so it can only be
// either of those if the samples were, and a NaN coordinate multiplied through
// the vertex buffer is a hole in the trail that GL will not tell anyone about.
func polarRadius(m int, r, drive float32) float32 {
	if !(r > 0) { // false for NaN, and 0 is already the answer for 0
		return 0
	}
	if !(drive > 0) {
		drive = 0.2 // the knob's floor; a zero drive would collapse the figure to a point
	}
	switch m {
	case polarMapAlgebraic:
		// drive·r/(1 + drive·r), written as 1 − 1/(1 + drive·r). Below tanh
		// everywhere past the origin, so the same drive keeps more of the
		// mid-range and reaches the surface later.
		//
		// The rearrangement is not cosmetic: the direct form is ∞/∞ for an
		// infinite length and returns NaN, where this one returns the surface,
		// which is the answer. An infinite delay coordinate cannot come from a
		// sample bounded to ±1 — but the maps are also reachable from the audio
		// modulator's arithmetic, and a NaN written into the vertex buffer is a
		// hole in the trail that GL reports to nobody.
		return 1 - 1/(1+drive*r)
	case polarMapUnit:
		// Loudness removed entirely: the surface of the sphere and nothing else.
		return 1
	default: // polarMapTanh
		return float32(math.Tanh(float64(drive * r)))
	}
}

// polarScale is the factor each coordinate of a vector of length r is
// multiplied by. Isotropic by construction — one factor for all three — which
// is what keeps the direction, and therefore the reconstructed geometry,
// exactly as the delay vector had it.
func polarScale(m int, r, drive float32) float32 {
	if !(r > 0) {
		return 0 // a zero vector has no direction to preserve; it stays at the origin
	}
	return polarRadius(m, r, drive) / r
}

// polarFitExtent is the extent the camera is fitted to, and it is GAIN — not
// gain·√3.
//
// The √3 in takensFitExtent is the CUBE'S CORNER: the Takens mode's three
// coordinates are three independent samples, each bounded by gain, so the
// vector can reach √3·gain when all three peak together. Here the bound is on
// the vector's LENGTH rather than on its coordinates, polarRadius never exceeds
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
func generatePolar() {
	src := ensureAudioSource()
	tau := int(polarTau)
	if tau < 1 {
		tau = 1
	}
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	n, stride := takensWindow(polarWin, sr, steps)
	span := (n-1)*stride + 2*tau
	if need := span + 1; len(polarRing) < need {
		polarRing = make([]float32, need+need/2)
		polarW = 0
	}
	if polarScratch == nil {
		polarScratch = make([]float32, 8192)
	}
	if tapReady() {
		// Its own cursor into the shared tap, as every frame-loop consumer has:
		// Drain hands each sample over exactly once, so two consumers on the
		// raw source would split the stream rather than each see it.
		for drained := 0; drained < 16384; {
			got := tapRead(&polarCursor, polarScratch)
			if got <= 0 {
				break
			}
			for i := 0; i < got; i++ {
				polarRing[polarW%len(polarRing)] = polarScratch[i]
				polarW++
			}
			drained += got
			if got < len(polarScratch) {
				break
			}
		}
	}
	avail := polarW
	if avail > len(polarRing) {
		avail = len(polarRing)
	}
	nv := takensVerts(n)
	if avail < span+1 {
		polarFitted = false // camera was fitted to silence — refit on real data
		uploadVerticesOnly(vertBuf[:nv*4], attractorDrawMode, nv)
		return
	}
	rn := len(polarRing)
	base := polarW - 1 - span
	g := polarGain
	mapSel := polarMapSel()
	drive := polarDrive

	// at reads source point k at delay offset off, clamping k to the window so
	// the spline's outer control points at either end are defined.
	at := func(k, off int) float32 {
		if k < 0 {
			k = 0
		} else if k > n-1 {
			k = n - 1
		}
		return polarRing[(base+2*tau+k*stride+off)%rn]
	}
	invN := float32(1) / float32(nv-1)
	vertices := vertBuf[:nv*4]
	var v [3]float32
	for m := 0; m < nv; m++ {
		i := m / takensSmooth
		f := float32(m%takensSmooth) / takensSmooth
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
		s := polarScale(mapSel, float32(math.Sqrt(float64(v[0]*v[0]+v[1]*v[1]+v[2]*v[2]))), drive) * g
		vertices[j+0] = v[0] * s
		vertices[j+1] = v[1] * s
		vertices[j+2] = v[2] * s
		// The trail ramp, which is also what audioColorWindow's table indexes.
		vertices[j+3] = float32(m) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, nv)
	if !polarFitted {
		// Fitted once, to the fixed scale's worst case rather than to this
		// window's extent — see generateTakens for why fitting the
		// instantaneous figure put loud passages off the screen. Here the
		// worst case is the sphere, so the fit is exact and not merely safe.
		polarFitted = true
		fitExtentOverride = polarFitExtent(polarGain)
		autoFitCamera()
	}
}

// polarColorWindow is audioColorWindow for this mode.
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
func polarColorWindow() ([]float32, int) {
	if polarRing == nil {
		return nil, 0
	}
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := int(polarTau)
	if tau < 1 {
		tau = 1
	}
	n, stride := takensWindow(polarWin, sr, steps)
	if n <= 0 {
		return nil, 0
	}
	rn := len(polarRing)
	span := (n-1)*stride + 2*tau
	if rn == 0 || polarW < span+1 {
		return nil, 0 // not enough audio yet; the flat fill is the honest answer
	}
	if cap(audioColorWin) < n {
		audioColorWin = make([]float32, n)
	}
	out := audioColorWin[:n]
	base := polarW - 1 - span
	for k := 0; k < n; k++ {
		out[k] = polarRing[(base+2*tau+k*stride)%rn]
	}
	return out, sr
}
