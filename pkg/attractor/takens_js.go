//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/led"
	"github.com/0magnet/chaosrack/pkg/takens"
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
// so that one position is one duration on every source — see takens.TauSamples for why
// raw samples of the source stream were the wrong unit. Too small stretches the
// figure along the diagonal, too large folds it, and the right value is a
// property of the signal rather than something to guess: the first minimum of
// its average mutual information, with the false-nearest-neighbor dimension
// alongside it (see pkg/takens). That is measured ONCE when the mode first
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
	takensTau     float32 = takens.TauDef // delay τ, in reference samples (see takens.TauSamples)
	takensGain    float32 = 10            // world units a full-scale (±1) sample maps to
	takensWin     float32 = 85            // display window, milliseconds
	takensRing    []float32
	takensW       int // monotonic write cursor into takensRing
	takensScratch []float32
	takensCursor  = tapUnjoined // read position in the shared audio tap

	// takensChanF is the SRC knob: which signal of the live pair is embedded.
	//
	// Takens' theorem takes ONE observable, and until the tap carried both
	// channels the only observable available was the mix — so the one mode
	// whose whole subject is "reconstruct the system behind this signal" could
	// not be pointed at a signal. The side channel is the interesting position:
	// it is what the two channels do NOT have in common, which on a real mix is
	// the reverb, the stereo width and the room rather than the instruments, and
	// it reconstructs a different manifold from the same recording.
	takensChanF float32

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
		{"takens-chan", "src", &takensChanF, 0, 0, float32(len(tapChanNames) - 1), 1},
		{"takens-tau", "τ", &takensTau, takens.TauDef, 1, takens.TauMax, 1},
		{"takens-win", "win", &takensWin, 85, 5, 500, 5},
		{"takens-gain", "gain", &takensGain, 10, 0.5, 50, 0.5},
		{"takens-smooth", "smth", &takensSmoothF, 4, 1, 16, 1},
	}
}

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
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	tau := takens.TauSamples(takensTau, sr)
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
			n := tapReadChan(&takensCursor, takensScratch, tapChanSel(takensChanF))
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
	// A different SRC is a different signal, so the τ measured from the last
	// one describes nothing about this one — the same argument the source swap
	// makes, one level down. Re-arming rather than measuring, so the one-shot
	// still has to see a full window before it fires.
	if ch := tapChanSel(takensChanF); ch != takensChanMeasured {
		takensChanMeasured = ch
		takensArmAutoMeasure()
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
	sm := takensSmooth()
	for m := 0; m < nv; m++ {
		i := m / sm
		f := float32(m%sm) / float32(sm)
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
		view.fitOverride = takensFitExtent(takensGain)
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
	takensMeasEl js.Value               // the readout beside the button
	takensMeas   takens.EmbeddingResult // last measurement, for the readout

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

	// takensChanMeasured is the SRC the standing measurement was taken from, so
	// that turning that knob re-arms the one-shot. It is deliberately NOT a
	// pointer at the knob: the knob is a float a modulator can wobble, and what
	// matters is the detent it lands on.
	takensChanMeasured tapChan
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
	if takensTau != takens.TauDef && takensTau != takensAutoSet {
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
		takensMeas = takens.EmbeddingResult{}
		showTakensMeasurement("no audio")
		return
	}
	// The knob's reach is the search range: an answer the knob cannot hold
	// would be a number to look at and nothing more. The estimator works in
	// SOURCE samples, because that is what the series it is handed is made of,
	// while the knob counts reference samples — so the reach is converted into
	// the estimator's units on the way in and the answer back out again.
	sr := takensSourceRate()
	r := takens.EstimateEmbedding(x, takens.TauSamples(takens.TauMax, sr), 8)
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
	s := "τ" + led.Format(float64(tauMS(takensTau)), 1, 1, false)
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
	if sr > 0 && sr != takens.RefRate {
		ref = float32(tauSrc) * float32(takens.RefRate) / float32(sr)
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
	} else if tau > int(takens.TauMax) {
		tau = int(takens.TauMax)
	}
	takensTau = float32(tau)
	el := dom.Doc.Call("getElementById", "takens-tau")
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
	card := dom.Doc.Call("createElement", "div")
	card.Set("className", "punit")

	lbl := dom.Doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "meas")
	card.Call("appendChild", lbl)

	takensMeasEl = dom.Doc.Call("createElement", "span")
	takensMeasEl.Set("className", "led counter-led")
	takensMeasEl.Set("title", "Measured embedding, in milliseconds: τ is the first minimum of the signal's average mutual information, written into the τ knob; m is the false-nearest-neighbor dimension. m greater than 3 means the trail on screen is a projection of a higher-dimensional reconstruction. This runs by itself once the mode has enough audio, and again when the source changes — but never over a τ you have set yourself. The button measures again on demand.")
	takensMeasEl.Set("textContent", takensMeasText())
	card.Call("appendChild", takensMeasEl)

	row := dom.Doc.Call("createElement", "span")
	row.Set("className", "grp")
	btn := dom.Doc.Call("createElement", "button")
	btn.Set("className", "rst")
	btn.Set("textContent", "↻")
	btn.Set("title", "Measure the embedding from the audio in the buffer and set τ from it. Once, on demand — this mode deliberately does not re-tune itself per frame, because a knob that moves with the music makes the figure move with it too.")
	btn.Call("addEventListener", "click", dom.FuncOf(func(this js.Value, a []js.Value) interface{} {
		takensMeasure()
		return nil
	}))
	row.Call("appendChild", btn)
	card.Call("appendChild", row)
	grid.Call("appendChild", card)
}

// tauMS is a τ knob value as milliseconds, for readouts. It does not depend on
// the source rate, which is the whole point of the reference unit.
func tauMS(tauRef float32) float32 { return tauRef * 1000 / takens.RefRate }
