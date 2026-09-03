//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// Takens delay embedding — the mode that turns LIVE AUDIO into an attractor.
// Takens' theorem (1981): the delay vector (s(t), s(t−τ), s(t−2τ)) of a
// single observable reconstructs a manifold diffeomorphic to the source
// system's attractor. Feed it a pure tone and you get a closed loop (a
// Lissajous-like ellipse whose shape is set by τ); feed it music, speech or
// a chaotic circuit's output and you see the geometry of whatever generated
// the signal, live. The reconstructed trail rides the NORMAL 3D pipeline —
// rotate, zoom, persist, gradient, beam-dwell, even Model Out SCAN all work.
//
// The τ knob is in samples of the source stream. Too small stretches the
// figure along the diagonal, too large folds it. The MEAS button measures the
// right value instead of guessing it — the first minimum of the signal's
// average mutual information, with the false-nearest-neighbor dimension
// alongside it (see embedding.go, and takensMeasure below for why that is a
// button and not something the frame loop does).
//
// The WIN knob is what makes it feel live, and it is a DURATION, not a point
// count. This mode used to take its span from the global trail knob the way
// the ODE modes do, which meant the default 20000 points became a 20000-SAMPLE
// window: 833 ms at 24 kHz. A frame then replaced ~2% of a dense tangle, and
// measured against the previous frame 250 ms earlier, 0.0% of the lit pixels
// changed — an image that is technically live and looks frozen. The xy scope
// next door had it right all along with a fixed xyWindow of 2048 samples, so
// that is the default here too: 85 ms, at which the same measurement changes
// ~80%. The trail knob still sets the point BUDGET (the stride is derived to
// fit the window into it), so a long window stays affordable.
//
// The SCALE IS FIXED. GAIN maps a full-scale sample to world units and nothing
// adjusts it at runtime — a quiet passage draws a small figure and a loud one
// draws a large figure, which is the honest picture and, more to the point, the
// one that holds still.
//
// This is worth stating plainly because the obvious alternative was tried and
// is wrong. Any automatic scaling divides by a measured level, and the drawn
// size is level/divisor, so the moment the divisor moves the figure resizes:
// that reads as the whole thing zooming in and out in time with the music.
// Normalizing the peak is worse than it sounds — measured live it held the
// outermost point to a 1.32x range while the BODY of the figure swung 3.26x,
// because the crest factor of music itself varies (2.8x over fourteen seconds).
// Normalizing the RMS instead fixed that (body 1.12x) but still moved, and it
// still had to reserve screen space for peaks it could not predict. A constant
// never moves, and there is nothing left to tune.
//
// Nothing can leave the frame, because a fixed scale has a knowable worst case:
// samples are bounded to ±1, so no coordinate exceeds GAIN and the camera is
// fitted once to that bound rather than to whatever happened to be playing.

var (
	takensTau     float32 = 32 // delay τ, in source samples
	takensGain    float32 = 10 // world units a full-scale (±1) sample maps to
	takensWin     float32 = 85 // display window, milliseconds
	takensRing    []float32
	takensW       int // monotonic write cursor into takensRing
	takensScratch []float32
	takensFitted  bool // camera fitted since real audio arrived
)

// takensCubeDiag is √3: a delay vector reaches this multiple of its largest
// single coordinate when all three coordinates peak together, which they do on
// anything with strong low-frequency content. autoFitCamera measures extent as
// max|coordinate|, so fitting it the raw bound left the figure exactly filling
// the viewport and clipping top and bottom at the rotations that swing that
// corner toward the camera.
const takensCubeDiag = 1.7320508

func init() {
	registerGenerate("takens", generateTakens)
	attractorParams["takens"] = []paramDef{
		{"takens-tau", "τ", &takensTau, 32, 1, 512, 1},
		{"takens-win", "win", &takensWin, 85, 5, 500, 5},
		{"takens-gain", "gain", &takensGain, 10, 0.5, 50, 0.5},
	}
}

// takensSmooth is the beam-smoothing upsample factor, exactly as the xy scope
// uses it: each sample-to-sample step is drawn as this many Catmull-Rom spline
// steps. A LINE_STRIP straight from one delay vector to the next is a chord,
// and chords are what put the visible straight runs and hard corners in the
// figure — they are an artifact of the drawing, not something in the signal.
// The true signal between two samples is a bandlimited curve, so the spline is
// the more faithful reconstruction, not a prettier lie. It also covers the
// decimated case: when a long window forces stride > 1, the curve passes
// through the kept samples instead of cutting across them.
const takensSmooth = 4

// takensWindow converts the WIN knob into a sample count, a stride that fits
// that many samples into the point budget, and the resulting source-point
// count. Split out from generateTakens so the arithmetic can be tested without
// a GL context: it is the whole difference between a live figure and a frozen
// one. budget is the VERTEX budget; smoothing spends takensSmooth of them per
// source point.
func takensWindow(winMS float32, sampleRate, budget int) (n, stride int) {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	win := int(winMS / 1000 * float32(sampleRate))
	if win < 64 {
		win = 64
	}
	src := budget / takensSmooth
	if src < 2 {
		src = 2
	}
	// Enough points to draw the window, decimating only when it does not fit.
	stride = (win + src - 1) / src
	if stride < 1 {
		stride = 1
	}
	// n <= src follows from the ceiling above; the two delays cost ring
	// space, not vertex budget, and the ring is sized from the span.
	n = win / stride
	if n < 2 {
		n = 2
	}
	return n, stride
}

// takensVerts is how many vertices takensWindow's n source points draw.
func takensVerts(n int) int { return (n-1)*takensSmooth + 1 }

// takensFitExtent is the extent the camera is fitted to: the worst case a fixed
// scale can produce. Samples are bounded to ±1, so no coordinate exceeds gain,
// and the cube's corner is √3 of that.
func takensFitExtent(gain float32) float32 { return gain * takensCubeDiag }

// generateTakens drains the audio source into a persistent ring and draws the
// newest window of delay vectors. When there's no (or not yet enough) audio
// the previous frame is re-uploaded, so the model doesn't flicker while the
// source spins up.
func generateTakens() {
	src := ensureAudioSource()
	tau := int(takensTau)
	if tau < 1 {
		tau = 1
	}
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	n, stride := takensWindow(takensWin, sr, steps)
	span := (n-1)*stride + 2*tau
	if need := span + 1; len(takensRing) < need {
		takensRing = make([]float32, need+need/2)
		takensW = 0
	}
	if takensScratch == nil {
		takensScratch = make([]float32, 8192)
	}
	if src != nil && src.Ready() {
		// Per-frame drain CAP, not drain-until-dry: the function generator
		// synthesizes on demand and always fills the buffer, so an uncapped
		// loop never terminates (froze the page on the first fg-on test).
		for drained := 0; drained < 16384; {
			n := src.Drain(takensScratch)
			if n <= 0 {
				break
			}
			for i := 0; i < n; i++ {
				takensRing[takensW%len(takensRing)] = takensScratch[i]
				takensW++
			}
			drained += n
			if n < len(takensScratch) {
				break
			}
		}
	}
	avail := takensW
	if avail > len(takensRing) {
		avail = len(takensRing)
	}
	nv := takensVerts(n)
	if avail < span+1 {
		takensFitted = false // camera was fitted to silence — refit on real data
		uploadVerticesOnly(vertBuf[:nv*4], attractorDrawMode, nv)
		return
	}
	rn := len(takensRing)
	base := takensW - 1 - span // oldest sample the window needs
	g := takensGain

	// at reads source point k at delay offset off, clamping k to the window so
	// the spline's outer control points at either end are defined.
	at := func(k, off int) float32 {
		if k < 0 {
			k = 0
		} else if k > n-1 {
			k = n - 1
		}
		return takensRing[(base+2*tau+k*stride+off)%rn]
	}
	invN := float32(1) / float32(nv-1)
	vertices := vertBuf[:nv*4]
	for m := 0; m < nv; m++ {
		i := m / takensSmooth
		f := float32(m%takensSmooth) / takensSmooth
		j := m * 4
		for c, off := range [3]int{0, -tau, -2 * tau} {
			p0, p1, p2, p3 := at(i-1, off), at(i, off), at(i+1, off), at(i+2, off)
			// Catmull-Rom through p1..p2.
			vertices[j+c] = 0.5 * (2*p1 + (-p0+p2)*f +
				(2*p0-5*p1+4*p2-p3)*f*f +
				(-p0+3*p1-3*p2+p3)*f*f*f) * g
		}
		vertices[j+3] = float32(m) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, nv)
	if !takensFitted {
		// The mode-entry auto-fit saw silence (a dot), so fit once when the
		// first full window of real audio arrives — and fit to the FIXED
		// scale's worst case, not to this window's extent. Fitting the
		// instantaneous figure is what used to put louder passages off the
		// screen: whatever was playing at that moment became the whole
		// viewport. Fitting the bound instead is correct for good, so this
		// stays one-shot and never fights a manual zoom.
		takensFitted = true
		fitExtentOverride = takensFitExtent(takensGain)
		autoFitCamera()
	}
}

// ── Measuring τ instead of guessing it ───────────────────────────────────
//
// ON A BUTTON, ONCE. This is the whole design of the feature and it is a
// constraint, not an implementation detail: per-frame auto-adjustment was
// deliberately taken OUT of this mode because a quantity that re-tunes itself
// sixty times a second makes the figure move in time with the music, and the
// eye reads that as the signal. τ is a static property of the source — a
// violin's first MI minimum does not change between frames — so it is measured
// when asked for, written into the knob, and then holds still and can be
// turned by hand like any other knob. Nothing here is reachable from
// generateTakens, and nothing here may become reachable from it.

// takensEstMax caps the measurement window. The estimators are O(n²) in the
// FNN search; 4096 samples (~170 ms at 24 kHz) is a run of a few milliseconds
// and already far more data than the histograms need.
const takensEstMax = 4096

var (
	takensMeasEl js.Value        // the readout beside the button
	takensMeas   EmbeddingResult // last measurement, for the readout
)

// takensEstWindow copies the newest samples out of the ring, oldest first, as
// the float64 series the estimators take. Returns nil when there is not enough
// audio to measure — the ring is empty until the mode has been running.
func takensEstWindow() []float64 {
	avail := takensW
	if avail > len(takensRing) {
		avail = len(takensRing)
	}
	if avail > takensEstMax {
		avail = takensEstMax
	}
	if avail < 512 {
		return nil
	}
	out := make([]float64, avail)
	rn := len(takensRing)
	base := takensW - avail
	for i := range out {
		out[i] = float64(takensRing[(base+i)%rn])
	}
	return out
}

// takensMeasure runs both estimators on the current window and writes τ into
// the knob. The dimension is a READOUT rather than a second knob: the trail is
// three delay coordinates because the screen has three axes, so when the
// signal needs more than three the honest thing to say is that what is drawn
// is a projection — not to silently draw something else.
func takensMeasure() {
	x := takensEstWindow()
	if x == nil {
		takensMeas = EmbeddingResult{}
		showTakensMeasurement("no audio")
		return
	}
	// The knob's range is the search range: an answer the knob cannot hold
	// would be a number to look at and nothing more.
	r := EstimateEmbedding(x, 512, 8)
	takensMeas = r
	if r.Tau < 1 {
		// White noise has no first minimum — its mutual information is at the
		// estimator's floor for every delay — and neither does a signal too
		// short to have one. Saying so beats moving the knob to a number that
		// means nothing.
		showTakensMeasurement("no min")
		return
	}
	setTakensTau(r.Tau)
	msg := "τ" + strconv.Itoa(r.Tau)
	if r.OK {
		msg += " m" + strconv.Itoa(r.Dim)
	} else {
		msg += " m>" + strconv.Itoa(r.Dim) // FNN never settled inside the search
	}
	showTakensMeasurement(msg)
}

// setTakensTau moves the knob, rather than only the variable behind it: the
// hidden range input is the value, and its input event is what repaints the
// dial and the LED. Writing takensTau alone would draw the new embedding under
// a knob still showing the old number.
func setTakensTau(tau int) {
	if tau < 1 {
		tau = 1
	} else if tau > 512 {
		tau = 512
	}
	takensTau = float32(tau)
	el := doc.Call("getElementById", "takens-tau")
	if !el.Truthy() {
		return
	}
	el.Set("value", strconv.Itoa(tau))
	el.Call("dispatchEvent", js.Global().Get("Event").New("input"))
}

func showTakensMeasurement(s string) {
	if takensMeasEl.Truthy() {
		takensMeasEl.Set("textContent", s)
	}
}

// appendTakensEstimate adds the MEAS cell — the button and its readout — to
// the Takens parameter grid.
func appendTakensEstimate(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "meas")
	card.Call("appendChild", lbl)

	takensMeasEl = doc.Call("createElement", "span")
	takensMeasEl.Set("className", "led counter-led")
	takensMeasEl.Set("title", "Measured embedding: τ is the first minimum of the signal's average mutual information (written into the τ knob); m is the false-nearest-neighbor dimension. m greater than 3 means the trail on screen is a projection of a higher-dimensional reconstruction.")
	if takensMeas.Tau > 0 {
		takensMeasEl.Set("textContent", "τ"+strconv.Itoa(takensMeas.Tau)+" m"+strconv.Itoa(takensMeas.Dim))
	} else {
		takensMeasEl.Set("textContent", "τ-- m-")
	}
	card.Call("appendChild", takensMeasEl)

	row := doc.Call("createElement", "span")
	row.Set("className", "grp")
	btn := doc.Call("createElement", "button")
	btn.Set("className", "rst")
	btn.Set("textContent", "↻")
	btn.Set("title", "Measure the embedding from the audio in the buffer and set τ from it. Once, on demand — this mode deliberately does not re-tune itself per frame, because a knob that moves with the music makes the figure move with it too.")
	btn.Call("addEventListener", "click", trackedFuncOf(func(this js.Value, a []js.Value) interface{} {
		takensMeasure()
		return nil
	}))
	row.Call("appendChild", btn)
	card.Call("appendChild", row)
	grid.Call("appendChild", card)
}
