package attractor

// The delay-embedding arithmetic shared by the Takens, polar and stereo modes.
// Untagged, unlike the generators in takens_js.go and the files beside it, so
// that `chaosrack render` builds an embedding with exactly the page's delay,
// window and smoothing.

// takensSmoothF is the beam-smoothing upsample factor, exactly as the xy scope
// uses it: each sample-to-sample step is drawn as this many Catmull-Rom spline
// steps. A LINE_STRIP straight from one delay vector to the next is a chord,
// and chords are what put the visible straight runs and hard corners in the
// figure — they are an artifact of the drawing, not something in the signal.
// The true signal between two samples is a bandlimited curve, so the spline is
// the more faithful reconstruction, not a prettier lie. It also covers the
// decimated case: when a long window forces stride > 1, the curve passes
// through the kept samples instead of cutting across them.
//
// A KNOB, and it was a constant here while the xy scope — doing the identical
// thing, and named in the sentence above as where it came from — has had it on
// a knob all along. It is worth turning: it is the trade between how much
// WINDOW is on screen and how smooth the beam is, because the two come out of
// one vertex budget. Down at 1 the figure is raw chords through four times as
// many samples, which is what to use to see a long window; up at 16 it is a
// glass-smooth beam through a short one.
//
// Shared with the polar embedding, which draws its beam with the same
// arithmetic — the recurrence plot already shares takens-tau the same way.
var takensSmoothF float32 = 4

// takensSmooth is that knob as a step count, clamped.
//
// Clamped rather than trusted because it is an audio-modulation target and it
// is a DIVISOR: takensWindow spends budget/smooth on source points, so a
// modulator that drives it to zero is a division by zero rather than an ugly
// figure.
func takensSmooth() int {
	n := int(takensSmoothF + 0.5)
	if n < 1 {
		n = 1
	} else if n > 16 {
		n = 16
	}
	return n
}

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
	sm := takensSmooth()
	src := budget / sm
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
func takensVerts(n int) int { return (n-1)*takensSmooth() + 1 }

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
