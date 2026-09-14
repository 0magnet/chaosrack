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
// The τ knob is a DELAY, and it is counted in samples at a fixed reference rate
// so that one position is one duration on every source — see tauSamples for why
// raw samples of the source stream were the wrong unit. Too small stretches the
// figure along the diagonal, too large folds it, and the right value is a
// property of the signal rather than something to guess: the first minimum of
// its average mutual information, with the false-nearest-neighbor dimension
// alongside it (see embedding.go). That is measured ONCE when the mode first
// has audio, again when the source changes, and on the MEAS button whenever
// asked — but never over a τ somebody has set, and never per frame. The
// section comment above takensMeasure is where that rule is argued.
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
	takensTau     float32 = takensTauDef // delay τ, in reference samples (see tauSamples)
	takensGain    float32 = 10           // world units a full-scale (±1) sample maps to
	takensWin     float32 = 85           // display window, milliseconds
	takensRing    []float32
	takensW       int // monotonic write cursor into takensRing
	takensScratch []float32
	takensCursor  = tapUnjoined // read position in the shared audio tap

	// takensFitGain is the GAIN the camera was last fitted to, or 0 for "not
	// fitted since audio arrived". It is a gain rather than a bool because the
	// bound the fit is made against IS a function of the gain — see
	// takensFitExtent — so a fit made at one gain is simply not a fit at
	// another. Held as a bool, raising GAIN grew the figure against a camera
	// that never moved and pushed it off the top of the screen, with only Zoom
	// to get it back: two controls on one axis, one of which could put the
	// model somewhere the other had to rescue it from.
	//
	// This does not weaken the once-only rule that the comment in
	// generateTakens argues for. That rule is about not re-fitting to the
	// signal, which is what makes the view move in time with the music; the
	// gain is a knob somebody turned, and a knob that moves the bound should
	// move the frame with it.
	takensFitGain float32
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
		{"takens-tau", "τ", &takensTau, takensTauDef, 1, takensTauMax, 1},
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

// ── τ is a DELAY, so it has to be a time ─────────────────────────────────
//
// The τ knob counts samples, and a sample is not a fixed amount of time: the
// microphone runs at whatever the browser's AudioContext runs at (48 kHz on
// most machines) and the server feed at 24 kHz. Read as raw samples, one knob
// position was therefore TWICE the delay on the feed that it was on the
// microphone — the same patch drawing a different figure depending on where the
// sound came from, and a permalink or a preset meaning different things on two
// machines, with nothing saying so.
//
// So the knob's unit is samples AT A FIXED REFERENCE RATE, and the actual delay
// in source samples is derived per frame from the live rate. A knob position is
// then a duration — tauRefRate units of 1/48000 s — and it is the same duration
// everywhere.
//
// The unit is not simply relabeled to milliseconds, which is what this wants to
// be, because the knob cannot carry a fractional step: τ is ONE knob shared by
// the Takens mode and the Recurrence Plot (see recurrence_js.go and the test
// that pins the two rows together), both rows have to agree, and both the
// patchbay and the audio-mod matrix key routings by parameter id alone. A
// millisecond knob fine enough to be useful needs two decimals. The reference
// rate gets the property that matters — one position, one delay — while leaving
// the id, the range, the step and every permalink already written alone, and
// tauMS turns the number into milliseconds wherever one is shown.
const tauRefRate = 48000

// takensTauMax is the τ knob's ceiling, in reference samples, and it is one
// constant because the knob is SHARED: the Takens mode, the Stereo and Polar
// embeddings and the Recurrence Plot all expose a τ, and Reset All walks every
// mode's parameter list — so two rows disagreeing about the range would reset
// one variable to two different numbers depending on map iteration order.
// 512 reference samples is 10.7 ms, which is a period of 94 Hz: past the point
// where a delay embedding of program material folds rather than unfolds.
const takensTauMax = 512

// takensTauDef is the τ knob's default, in reference samples: 72 of them is
// 1.5 ms.
//
// It was 32, which is 0.67 ms, and that is a tenth of a cycle of anything in
// the bass — far too short for program material, and "too short" has a shape:
// the three delay coordinates are then nearly the same sample, the figure
// collapses toward the main diagonal, and what should be an open reconstruction
// arrives as a streak. Measured against a three-tone signal, the MEAS estimator
// asked for 73.
//
// A default is only the opening position — MEAS and the auto-measurement below
// move it to what the signal actually wants — but it is what a permalink that
// never mentions τ restores, and what the mode is judged on in its first second.
const takensTauDef = 72

// tauSamples converts a τ knob value into the delay in source samples at the
// rate the audio is actually arriving at. At the reference rate it is the
// identity, which is why nothing changes on an ordinary microphone.
//
// Never returns less than 1: a zero delay makes all three coordinates the same
// sample, which collapses the embedding onto the diagonal, and a negative one
// indexes backwards out of the ring. A rate of zero means the source has not
// reported one yet, and the knob's own number is the best guess available.
func tauSamples(tauRef float32, sr int) int {
	if !(tauRef > 0) { // false for NaN
		return 1
	}
	t := tauRef
	if sr > 0 && sr != tauRefRate {
		t = tauRef * float32(sr) / float32(tauRefRate)
	}
	n := int(t + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// tauMS is a τ knob value as milliseconds, for readouts. It does not depend on
// the source rate, which is the whole point of the reference unit.
func tauMS(tauRef float32) float32 { return tauRef * 1000 / tauRefRate }

// generateTakens drains the audio source into a persistent ring and draws the
// newest window of delay vectors. When there's no (or not yet enough) audio
// the previous frame is re-uploaded, so the model doesn't flicker while the
// source spins up.
func generateTakens() {
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := tauSamples(takensTau, sr)
	n, stride := takensWindow(takensWin, sr, steps)
	span := (n-1)*stride + 2*tau
	if need := span + 1; len(takensRing) < need {
		takensRing = make([]float32, need+need/2)
		takensW = 0
	}
	if takensScratch == nil {
		takensScratch = make([]float32, 8192)
	}
	if tapReady() {
		// The tap has already drained this frame; take our own copy of it.
		for drained := 0; drained < 16384; {
			n := tapRead(&takensCursor, takensScratch)
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
		takensFitGain = 0 // camera was fitted to silence — refit on real data
		uploadVerticesOnly(vertBuf[:nv*4], attractorDrawMode, nv)
		return
	}
	// Once, when the ring first holds enough to measure from — see the section
	// comment below for why this is allowed to be reached from here at all, and
	// why it does nothing on every frame but one.
	takensAutoMeasure()
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
	if takensFitGain != takensGain && !paramIsModulated("takens-gain") {
		// The mode-entry auto-fit saw silence (a dot), so fit when the first
		// full window of real audio arrives — and fit to the FIXED scale's
		// worst case, not to this window's extent. Fitting the instantaneous
		// figure is what used to put louder passages off the screen: whatever
		// was playing at that moment became the whole viewport. Fitting the
		// bound instead is correct for good, so this never fights a manual
		// zoom and never moves with the signal. It fires again only when GAIN
		// moves, because GAIN is what the bound is made of.
		takensFitGain = takensGain
		fitExtentOverride = takensFitExtent(takensGain)
		autoFitCamera()
	}
}

// ── Measuring τ instead of guessing it ───────────────────────────────────
//
// ONCE. That is the constraint, and it is the whole design of the feature
// rather than an implementation detail: per-frame auto-adjustment was
// deliberately taken OUT of this mode because a quantity that re-tunes itself
// sixty times a second makes the figure move in time with the music, and the
// eye reads that as the signal. τ is a static property of the source — a
// violin's first MI minimum does not change between frames — so it is measured
// when there is something to measure, written into the knob, and then holds
// still and can be turned by hand like any other knob.
//
// The rule used to read "ON A BUTTON, ONCE", and the frame loop was forbidden
// to reach any of this at all. The button being the only way in had a cost, and
// the cost was the mode's first impression: a default τ is a guess, a guess
// that is wrong collapses the figure toward the diagonal, and the one control
// that fixes it was a button nobody knew to press. So the measurement now also
// runs ONCE when the mode first has enough audio to measure it from — see
// takensAutoMeasure.
//
// What has NOT changed is the property the old rule was protecting. The
// measurement is still one-shot and still never per-frame; the guard that makes
// it so is a flag that only mode entry and a change of source clear. Nothing
// here may acquire a caller that runs more often than that, and the frame loop
// reaches exactly one function here — takensAutoMeasure — which is written to
// do nothing on all but one of the frames it is called on.

// takensEstMax caps the measurement window. The estimators are O(n²) in the
// FNN search; 4096 samples (~170 ms at 24 kHz) is a run of a few milliseconds
// and already far more data than the histograms need.
const takensEstMax = 4096

var (
	takensMeasEl js.Value        // the readout beside the button
	takensMeas   EmbeddingResult // last measurement, for the readout

	// takensAutoDone is the one-shot guard. Set the first time the automatic
	// measurement runs, and cleared only by takensArmAutoMeasure — which mode
	// entry and a change of audio source call, and nothing else does.
	takensAutoDone bool

	// takensAutoSet is the τ the automatic measurement last wrote, and it is
	// what lets the measurement tell ITS OWN value from one somebody chose.
	//
	// Without it the automatic measurement is a control that takes the knob
	// away: set τ by hand, switch to Lorenz to compare, come back, and the
	// measurement has thrown the setting away and replaced it. A permalink is
	// the same problem one step worse — a link that carries a τ carries it
	// because whoever made the link meant that τ, and measuring over it
	// discards the one thing the link was for.
	//
	// So the measurement only runs while τ is a value NOBODY CHOSE: the
	// default, or whatever a previous automatic measurement put there. The
	// permalink case falls out of this rather than needing its own rule, since
	// a permalink records a parameter only when it differs from the default.
	takensAutoSet float32
)

// takensAutoMeasure runs the estimator ONCE, the first time the mode has enough
// audio to measure from, and writes the answer into the τ knob.
//
// Called from generateTakens, which is the only place that knows when the audio
// has arrived — and which calls it on every frame, so the very first thing it
// does is refuse. Two conditions guard it, and they are different in kind:
//
//   - takensAutoDone is the ONE-SHOT. It is what makes this a measurement
//     rather than a control loop, and it is why calling this from the frame
//     loop does not reintroduce the per-frame auto-tuning the section comment
//     above rejects.
//   - the window length is the QUALITY GATE. Measuring the first 512 samples
//     of a source that has just opened is measuring its fade-in, so the
//     estimate waits for a full takensEstMax of audio — about 85 ms at 48 kHz,
//     a fraction of a second after the mode is entered. It is also what keeps
//     the answer from being taken during the silence at the start.
//
// A measurement that comes back with no first minimum (white noise, or too
// little structure to have one) still counts as done. Retrying it every frame
// against a source that cannot produce an answer is exactly the per-frame work
// this is not allowed to be, and "no min" is an answer.
func takensAutoMeasure() {
	if !takensAutoDue() {
		return
	}
	// Set BEFORE measuring, not after. takensMeasure writes the τ knob, which
	// dispatches a DOM input event, and an event handler that reached the frame
	// loop again would find the guard still open and measure a second time.
	takensAutoDone = true
	takensMeasure()
	// Remember what it wrote, so a later automatic measurement can tell this
	// value from one somebody turned the knob to. Recorded whether or not the
	// estimate landed: "no min" leaves τ where it was, and where it was is
	// still not a value anybody chose.
	takensAutoSet = takensTau
}

// takensAutoDue is takensAutoMeasure's guard on its own, so that the one-shot,
// the quality gate and the deference to a chosen τ can all be tested without a
// DOM to write a knob into.
func takensAutoDue() bool {
	if takensAutoDone {
		return false
	}
	// Never over a τ somebody chose — see takensAutoSet. The knob is theirs
	// from the moment they touch it, and a measurement that overrides it is not
	// a convenience, it is a control fighting the person using it.
	if takensTau != takensTauDef && takensTau != takensAutoSet {
		return false
	}
	avail := takensW
	if avail > len(takensRing) {
		avail = len(takensRing)
	}
	return avail >= takensEstMax
}

// takensArmAutoMeasure re-arms the one-shot: the next time the mode has a full
// window it measures again.
//
// A change of source is the case that matters. τ is a property of what is
// playing, and swapping the microphone for a server feed or for the signal
// generator swaps what is playing entirely — the measured τ from the old source
// describes nothing about the new one. Mode entry re-arms for the same reason
// one step removed: the source may well have changed while the mode was away.
func takensArmAutoMeasure() { takensAutoDone = false }

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
	// The knob's reach is the search range: an answer the knob cannot hold
	// would be a number to look at and nothing more. The estimator works in
	// SOURCE samples, because that is what the series it is handed is made of,
	// while the knob counts reference samples — so the reach is converted into
	// the estimator's units on the way in and the answer back out again.
	sr := takensSourceRate()
	r := EstimateEmbedding(x, tauSamples(takensTauMax, sr), 8)
	takensMeas = r
	if r.Tau < 1 {
		// White noise has no first minimum — its mutual information is at the
		// estimator's floor for every delay — and neither does a signal too
		// short to have one. Saying so beats moving the knob to a number that
		// means nothing.
		showTakensMeasurement("no min")
		return
	}
	setTakensTauSamples(r.Tau, sr)
	// Milliseconds, not the sample count: the knob is the same delay on every
	// source now, and a readout in a unit that changes with the source would be
	// the one place left saying otherwise.
	showTakensMeasurement(takensMeasText())
}

// takensMeasText renders the measurement cell. One function rather than two
// because the cell is written from two places — here, when a measurement lands,
// and appendTakensEstimate, which rebuilds the cell on every panel rebuild and
// has to put back what was there. Written twice, the rebuild kept the old
// sample-count wording and the readout changed units whenever a module was
// toggled.
//
// Six characters is what the cell holds — a third of a module wide — so one
// decimal and no space: "τ1.5m3" where it used to say "τ73 m3".
func takensMeasText() string {
	if takensMeas.Tau < 1 {
		return "τ-- m-"
	}
	s := "τ" + formatLED(float64(tauMS(takensTau)), 1, 1, false)
	if takensMeas.OK {
		return s + "m" + strconv.Itoa(takensMeas.Dim)
	}
	return s + "m>" + strconv.Itoa(takensMeas.Dim) // FNN never settled inside the search
}

// takensSourceRate is the live source's sample rate, or the fallback every
// other function here uses when it has not reported one yet.
func takensSourceRate() int {
	if src := ensureAudioSource(); src != nil && src.SampleRate() > 0 {
		return src.SampleRate()
	}
	return 24000
}

// setTakensTauSamples writes a delay measured in SOURCE samples into the knob,
// converting to the knob's reference-rate unit on the way.
func setTakensTauSamples(tauSrc, sr int) {
	ref := float32(tauSrc)
	if sr > 0 && sr != tauRefRate {
		ref = float32(tauSrc) * float32(tauRefRate) / float32(sr)
	}
	setTakensTau(int(ref + 0.5))
}

// setTakensTau moves the knob, rather than only the variable behind it: the
// hidden range input is the value, and its input event is what repaints the
// dial and the LED. Writing takensTau alone would draw the new embedding under
// a knob still showing the old number.
func setTakensTau(tau int) {
	if tau < 1 {
		tau = 1
	} else if tau > int(takensTauMax) {
		tau = int(takensTauMax)
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
	takensMeasEl.Set("title", "Measured embedding, in milliseconds: τ is the first minimum of the signal's average mutual information, written into the τ knob; m is the false-nearest-neighbor dimension. m greater than 3 means the trail on screen is a projection of a higher-dimensional reconstruction. This runs by itself once the mode has enough audio, and again when the source changes — but never over a τ you have set yourself. The button measures again on demand.")
	takensMeasEl.Set("textContent", takensMeasText())
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
