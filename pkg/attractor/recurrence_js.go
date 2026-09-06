//go:build js && wasm

package attractor

import (
	"strconv"
	"syscall/js"
)

// The recurrence plot — R[i][j] lit when the state at t_i is within ε of the
// state at t_j (Eckmann, Kamphorst & Ruelle, 1987). It is the one display here
// that shows the TIME structure of a system as a shape rather than as something
// moving: a steady tone draws unbroken diagonals spaced by its period, a
// chaotic or noisy source breaks them into short segments, a held note is a
// solid block, and the moment the source changes is a visible edge running
// across the square.
//
// It rides the spectrogram's rendering path — a texture on a plane through the
// shared 3D pipeline (textured_js.go) — so it rotates and zooms like every
// other model, on a SQUARE quad rather than the spectrogram's landscape one,
// because both of its axes are the same axis and the main diagonal has to come
// out at 45°.
//
// The matrix, the two normalizers and the RQA scalars are all in recurrence.go,
// untagged, so the properties that make the picture legible — a periodic signal
// gives a matrix invariant under shifting both indices by its period, DET
// separates an orbit from noise, one ε works across systems of wildly different
// width — are verified by native tests rather than by looking at it.
//
// ── SRC: what is being plotted ───────────────────────────────────────────
//
// A recurrence plot is defined on a STATE, and this app has three kinds lying
// around, so the knob has three positions rather than the mode having one
// source hardcoded:
//
//	audio  raw samples, |s_i − s_j|. What this mode has always drawn. It is the
//	       degenerate m = 1 case of the one below and shares its code path, but
//	       it is its own position rather than "embed with m = 1" because it is
//	       not the same object: the raw signal traces an interval, not a
//	       reconstructed curve, and it plots differently enough that offering
//	       the two as one setting would misdescribe both (measured: the same ε
//	       lights 6.1% of the square raw against 2.0% embedded, and a pure tone
//	       reads DET 0.50 raw against 0.96 embedded — see the tests).
//
//	embed  the Takens delay vector (s_t, s_{t−τ}, …, s_{t−(m−1)τ}) of the same
//	       live audio, which is what a recurrence plot is usually defined on:
//	       the reconstructed phase space, not the observable. This is the
//	       position that makes the mode a measurement rather than a picture. An
//	       anti-diagonal in the raw plot is an artifact of a scalar signal not
//	       being injective (sin t = sin(T/2 − t)); embedding removes it, which
//	       is most of the DET gap above.
//	       τ IS THE TAKENS MODE'S OWN τ KNOB, deliberately: it is the same delay
//	       of the same signal, its MEAS button measures it from the average
//	       mutual information, and having measured it there you want it here.
//	       Sharing the DOM id is what lets MEAS drive whichever of the two
//	       panels is on screen. It is safe against the routing tables because τ
//	       has an integer step, and both the patchbay and audio-mod skip integer
//	       settings.
//
//	traj   the running attractor's own trajectory, in (x,y,z). The bifurcation
//	       explorer's arrangement, and for its reason: this mode is not itself a
//	       flow, so it plots the most recent one that was (lastFlowMode). Switch
//	       to Lorenz, tune it, come back, and you are looking at its recurrences.
//
// ── ε, AND WHY IT IS NEVER MEASURED FROM THE PICTURE ─────────────────────
//
// ε is a fraction, always, of something that HOLDS STILL. The obvious
// convenience is to take it from the current window's spread each frame, and
// that is the mistake this repo already made and undid in the Takens mode: a
// threshold derived from the live audio makes the plot's density pulse in time
// with the music, and the eye reads density as structure. So:
//
//   - audio and embed normalize against the signal's KNOWN BOUND — samples are
//     ±1, so a delay vector lives in a cube whose half-diagonal is √m. Nothing
//     about the current audio enters it, and the same knob position means the
//     same thing at every m (RecurrenceVectorScale).
//
//   - traj normalizes against the attractor's own DIAMETER, which is legitimate
//     here for a reason that does not hold for audio: the trajectory is a
//     static object, integrated once and cached until the system changes, so
//     its diameter is exactly as still as the picture is. It is also the
//     convention the RQA literature quotes thresholds in, so 0.05 reads as "5%
//     of the attractor's width" — and it is what makes ONE default useful
//     across systems that differ in width by fifty times. Measured over all 32
//     registered flows at the default, the plot lights between 1.2% and 20% of
//     the square, with DET from 0.86 to 1.00: never blank, never saturated,
//     nothing to retune per model.
//
// ── WHAT IT COSTS, AND WHY THE TAB DOES NOT WEDGE ────────────────────────
//
// The matrix is O(N²·m) and N is fixed at 256 (rpN). That is not a compromise
// arrived at reluctantly: the texture is N×N, a 256-pixel square is what the
// quad shows at any reasonable zoom, and the cost at 256 is 292 µs for raw
// audio, 308 µs at m = 3 and 517 µs at m = 8 (native amd64; wasm slower by a
// roughly constant factor throughout). 512 would quadruple every one of those
// for detail the quad cannot display. Everything else is bounded against that
// number:
//
//   - the audio window is decimated to N columns by BOX AVERAGING, not by
//     taking one sample per stride — see the note at the decimation itself,
//     which is where an earlier version drew the recurrences of an aliased
//     signal nobody was playing;
//   - RQA runs at RQASamplePeriodMs rather than per frame, because 283 µs for
//     three numbers a human reads a few times a second is the kind of thing
//     that looks free and is not. That same tick feeds the strip chart beside
//     the readout (rqaseries_js.go), which is the whole of what the chart
//     costs: it keeps the answers rather than measuring anything of its own;
//   - and the trajectory source, the only genuinely expensive one, is cached
//     against the values that produced it AND held behind a settle delay. A
//     trajectory costs 0.5 ms for Lorenz and 24 ms for Chen, whose dt is ten
//     times smaller; recomputing that on every frame of a knob DRAG is exactly
//     the failure this file must not have, so a drag recomputes nothing until
//     the knob has been still for rpTrajSettleMs. The step budget in
//     recurrence.go caps the 24 ms; the settle delay caps how often it is paid.
const (
	// rpN is the matrix side, and the texture is rpN×rpN. 256 columns over
	// the window is enough for the diagonals to be legible and costs 65536
	// comparisons per frame, which is nothing next to a frame of audio.
	rpN = 256

	// rpDrainCap bounds the per-frame drain the same way the Takens mode
	// does: the signal generator synthesizes on demand and always fills the
	// buffer, so an uncapped drain-until-dry loop never terminates.
	rpDrainCap = 16384

	// rpMaxDim is the embedding dimension the point buffer is sized for — the
	// m knob's maximum. Allocated once at the top rather than regrown when the
	// knob moves: it is 16 KB, and a buffer that reallocates on a knob turn is
	// a buffer that reallocates once per pixel of a drag.
	rpMaxDim = 8

	// The SRC knob's positions. Ordered so the default, 0, is what the mode
	// drew before there was a knob — an existing permalink that omits it
	// restores the picture it was made from.
	rpSrcAudio = 0
	rpSrcEmbed = 1
	rpSrcTraj  = 2

	// rpTrajWinDiv converts the WIN knob's number into the trajectory source's
	// span in the system's own time units. The knob has one range to cover two
	// units — 20..2000 ms of audio, and roughly 2..200 units of model time —
	// and a factor of ten covers both, the way Model Out's RATE knob means Hz
	// in FLOW and sweeps per second in SCAN.
	//
	// The factor is not arbitrary. It puts the default (100 → 10 time units) at
	// about twenty points per Lorenz lobe circuit, which is the coarsest
	// sampling at which the orbit is still drawn as a curve rather than as a
	// scatter of unrelated points — and once consecutive points stop being
	// neighbors, the plot stops being a plot of an orbit. That failure is
	// visible in the readout rather than silent: DET collapses.
	rpTrajWinDiv = 10

	// rpTrajSettleMs is how long the source system's knobs must hold still
	// before the trajectory is re-integrated. A drag fires a value change per
	// pixel, and each one is a fresh integration of up to 300000 RK4 steps.
	// 200 ms is under the time it takes to let go of a knob and look at the
	// result, and long enough that nothing is recomputed mid-drag.
	rpTrajSettleMs = 200

	// The rate limit on the readout is RQASamplePeriodMs, in rqaseries.go —
	// moved there when the strip chart was added, because the tick that
	// rate-limits the scan and the tick that spaces the series are one tick and
	// two names for it would be two things to keep in step. The reasoning for
	// the number is with it.

	// rpMaxTau is the τ knob's maximum, and rpMaxLookback the deepest history
	// behind the plot window any (m, τ) can ask for. The ring is sized for
	// THAT rather than for the current setting, at a cost of 3584 float32s —
	// 14 KB — because a ring that grows when a knob moves also resets its write
	// cursor, and resetting the cursor throws the buffered audio away: turning
	// m from 1 to 8 would blank the plot eight times on the way. Sized for the
	// worst case once, m and τ are free to move.
	rpMaxTau      = 512
	rpMaxLookback = (rpMaxDim - 1) * rpMaxTau
)

var (
	rpTexture js.Value
	rpU8      js.Value // reused Uint8Array, rpN*rpN bytes
	rpReady   bool

	rpRing    []float32
	rpW       int // monotonic write cursor
	rpScratch []float32
	rpVec     []float64 // the rpN points of the current window, flat, m wide
	rpMat     []byte    // rpN*rpN, one byte per cell

	// rpWin is the span the square covers: milliseconds for the audio sources
	// — how much history the plot is of — and, divided by rpTrajWinDiv, the
	// trajectory source's span in the system's own time units. 100 ms is a few
	// dozen periods of a musical pitch across the square, and at 48 kHz it
	// decimates by 18, which leaves the fundamental range intact after the box
	// filter below.
	//
	// rpEps is the recurrence threshold as a fraction of the source's scale
	// (see the file comment on why that scale is never taken from the picture).
	//
	// rpSrc picks which state is plotted, and rpDim is the embedding dimension
	// the embed source builds its delay vectors at.
	rpWin float32 = 100
	rpEps float32 = 0.05
	rpSrc float32 = rpSrcAudio
	rpDim float32 = 3
)

func init() {
	registerGenerate("recurrence", generateRecurrence)
	attractorParams["recurrence"] = []paramDef{
		{"rec-src", "src", &rpSrc, rpSrcAudio, rpSrcAudio, rpSrcTraj, 1},
		{"rec-win", "win", &rpWin, 100, 20, 2000, 20},
		{"rec-eps", "ε", &rpEps, 0.05, 0.005, 0.5, 0.005},
		{"rec-dim", "m", &rpDim, 3, 1, rpMaxDim, 1},
		// The Takens mode's τ, under the same DOM id on purpose — see the SRC
		// note in the file comment. Def/Min/Max/Step must stay identical to the
		// row in takens_js.go, or Reset All resets one knob to two different
		// numbers depending on which of the two maps it walks last.
		{"takens-tau", "τ", &takensTau, 32, 1, 512, 1},
	}
}

func initRecurrencePlot() {
	if rpReady {
		return
	}
	rpTexture = gl.Call("createTexture")
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), rpTexture)
	// NEAREST, not LINEAR: the cells are a yes/no answer, and interpolating
	// between them invents half-recurrences that are not in the signal — at
	// this size it also smears the single-pixel diagonals into a haze.
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("NEAREST"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("NEAREST"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
	gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))

	// LUMINANCE rather than RGBA: a binary matrix needs one byte per cell, not
	// four, and the shared textured shader reads the single channel into all
	// three color channels for free.
	rpMat = make([]byte, rpN*rpN)
	rpU8 = js.Global().Get("Uint8Array").New(rpN * rpN)
	gl.Call("texImage2D",
		gl.Get("TEXTURE_2D"), 0, gl.Get("LUMINANCE"),
		rpN, rpN, 0,
		gl.Get("LUMINANCE"), gl.Get("UNSIGNED_BYTE"), rpU8)

	rpVec = make([]float64, rpN*rpMaxDim)
	rpScratch = make([]float32, 8192)
	rpReady = true
}

// rpEmbedDim is the m knob clamped to what the buffers hold, and 1 for the
// raw-audio position — which IS m = 1, and shares the code path for it.
func rpEmbedDim() int {
	if int(rpSrc) != rpSrcEmbed {
		return 1
	}
	m := int(rpDim)
	if m < 1 {
		return 1
	}
	if m > rpMaxDim {
		return rpMaxDim
	}
	return m
}

// rpWindow converts the WIN knob into a sample count and the stride that fits
// it into rpN columns. Split out from the generator so it can be tested
// without a GL context, as takensWindow is: it decides what the picture is OF.
func rpWindow(winMS float32, sampleRate int) (span, stride int) {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	span = int(winMS / 1000 * float32(sampleRate))
	if span < rpN {
		span = rpN
	}
	stride = span / rpN
	if stride < 1 {
		stride = 1
	}
	return stride * rpN, stride
}

// generateRecurrence rebuilds the matrix from whichever source the SRC knob
// names and uploads it. Called from generateForMode.
func generateRecurrence() {
	if !rpReady {
		initRecurrencePlot()
	}
	var fresh bool
	if int(rpSrc) == rpSrcTraj {
		fresh = rpFillFromTrajectory()
	} else {
		fresh = rpFillFromAudio()
		// Only the audio sources have a source to report on. The trajectory one
		// must not reach this: it would spin the microphone up and put an
		// "allow access?" overlay over a picture of the Lorenz attractor.
		maybeShowAudioStatus()
	}
	if fresh {
		rpMatDirty = true
		js.CopyBytesToJS(rpU8, rpMat)
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), rpTexture)
		gl.Call("texSubImage2D",
			gl.Get("TEXTURE_2D"), 0, 0, 0, rpN, rpN,
			gl.Get("LUMINANCE"), gl.Get("UNSIGNED_BYTE"), rpU8)
	}
	// Outside the fresh branch, and that is not a tidy-up. The strip chart's
	// axis is TIME, so it needs a slot per interval whether or not the matrix
	// moved — and for the trajectory source the matrix almost never moves,
	// because the series is static and cached. Measured on the fresh frames
	// only, the chart would advance in bursts on a knob turn and stand still
	// in between, which is a chart of edits rather than of the system.
	// Recomputation is still gated: rpMatDirty says whether there is anything
	// new to scan, so a frozen picture costs the readout nothing and the
	// series records the value it still holds.
	rpMaybeMeasure()
	drawTexturedSquare(rpTexture)
}

// rpFillFromAudio drains the live audio into the ring and fills rpMat from the
// newest window — as raw samples (m = 1) or as delay vectors. Reports whether
// the matrix was rebuilt; when there is not yet enough audio the previous frame
// stays on the texture rather than flickering to black.
func rpFillFromAudio() bool {
	src := ensureAudioSource()
	sr := 24000
	if src != nil && src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	dim := rpEmbedDim()
	// Clamped to the knob's own range rather than trusted, because the ring is
	// sized from rpMaxLookback: a τ past rpMaxTau — from a hand-edited
	// permalink, say — would ask for history behind the start of the buffer,
	// and the index arithmetic below would go negative rather than merely
	// wrong.
	tau := int(takensTau)
	if tau < 1 {
		tau = 1
	} else if tau > rpMaxTau {
		tau = rpMaxTau
	}
	span, stride := rpWindow(rpWin, sr)
	// The delay coordinates read BACKWARDS from the start of the plot window,
	// in source samples — τ is a delay in the signal, not in decimated columns,
	// which is what makes it the same τ the Takens mode measured. So the ring
	// has to hold the window plus the deepest delay, and the window's own base
	// index stays where it was.
	lookback := (dim - 1) * tau
	if need := span + rpMaxLookback + 1; len(rpRing) < need {
		rpRing = make([]float32, need+need/2)
		rpW = 0
	}
	if src != nil && src.Ready() {
		for drained := 0; drained < rpDrainCap; {
			n := src.Drain(rpScratch)
			if n <= 0 {
				break
			}
			for i := 0; i < n; i++ {
				rpRing[rpW%len(rpRing)] = rpScratch[i]
				rpW++
			}
			drained += n
			if n < len(rpScratch) {
				break
			}
		}
	}
	avail := rpW
	if avail > len(rpRing) {
		avail = len(rpRing)
	}
	// span + lookback, not span: with the delays included, requiring only span
	// would index behind the start of the ring on the first frames after a
	// resize. avail ≤ rpW, so this also guarantees base − lookback ≥ 0 and the
	// modulo below never sees a negative.
	if avail < span+lookback {
		return false
	}

	// Decimate the window to one point per column by AVERAGING each stride, not
	// by taking one sample out of it. Taking one is what this did first, and
	// the plot came out as speckle with a diagonal through it: a 500 ms window
	// at 48 kHz is a stride of 93, which puts the decimated Nyquist at 258 Hz,
	// and the signal generator's own default chord has a 294 Hz tone in it.
	// What was being drawn was the recurrences of an aliased signal — real
	// structure, belonging to a signal nobody was playing. A box average is the
	// anti-alias filter the decimation needs; it costs the detail above the new
	// Nyquist, which is detail the column count cannot represent either way.
	//
	// Each delay coordinate is averaged over its OWN stride at its own offset,
	// so every coordinate of the vector has had the same filter applied. Taking
	// the delayed coordinates as bare samples instead would embed a filtered
	// signal against an unfiltered one, and the reconstruction would be of
	// neither.
	rn := len(rpRing)
	base := rpW - span
	inv := 1 / float64(stride)
	for i := 0; i < rpN; i++ {
		for c := 0; c < dim; c++ {
			off := base + i*stride - c*tau
			var sum float32
			for k := 0; k < stride; k++ {
				sum += rpRing[(off+k)%rn]
			}
			rpVec[i*dim+c] = float64(sum) * inv
		}
	}
	RecurrenceMatrixVec(rpVec[:rpN*dim], dim, float64(rpEps)*RecurrenceVectorScale(dim), rpMat)
	return true
}

// ── The trajectory source ────────────────────────────────────────────────

var (
	rpTrajSeries []float64 // rpN points of (x,y,z), flat
	rpTrajDiam   float64   // its diameter, the ε normalizer
	rpTrajMode   string    // the flow it belongs to
	rpTrajWin    float32   // and the WIN knob it was integrated at
	rpTrajVals   []float32 // and that flow's parameter values

	// rpTrajStale is a flag rather than a "rpTrajStaleAt > 0" sentinel on the
	// timestamp beside it, because frameNowMs is the raw rAF timestamp and zero
	// is a value it can genuinely hold — it is zero until the first frame lands.
	// With the sentinel, a mode entered on that frame recorded staleness as 0,
	// read it back as "settled", and never integrated anything: a permanently
	// blank square with no error anywhere.
	rpTrajStale   bool
	rpTrajStaleAt float64 // frameNowMs when the knobs last moved
	rpTrajEps     float32 // the ε the matrix on the texture was built with
	rpTrajBuilt   bool    // a matrix has been built from the current series
	rpTrajGen     int     // bumped on every re-integration; see rqaConfigNow
)

// rpTrajChanged reports whether the source system, its parameters or the WIN
// knob differ from what the cached trajectory was integrated from, recording
// the current values as it goes.
//
// Comparing the VALUES rather than subscribing to an invalidation hook is the
// point: an edit that reaches a parameter by any route at all — a knob, a
// permalink, a preset recall, Reset All, a patch memory, MIDI, audio modulation
// — moves a float that this then sees. Nothing has to remember to tell it, and
// nothing added later can forget to.
func rpTrajChanged() bool {
	ps := attractorParams[lastFlowMode]
	changed := lastFlowMode != rpTrajMode || rpWin != rpTrajWin || len(rpTrajVals) != len(ps)
	if changed {
		rpTrajVals = make([]float32, len(ps))
	}
	for i, p := range ps {
		if rpTrajVals[i] != *p.Value {
			rpTrajVals[i] = *p.Value
			changed = true
		}
	}
	rpTrajMode, rpTrajWin = lastFlowMode, rpWin
	return changed
}

// rpFillFromTrajectory draws the most recent flow mode's own trajectory,
// re-integrating it only when the system it belongs to has changed AND has then
// held still. Reports whether the matrix was rebuilt this frame.
func rpFillFromTrajectory() bool {
	if rpTrajChanged() {
		// Do not integrate yet. See rpTrajSettleMs: a drag changes a value per
		// pixel, and Chen's trajectory is 24 ms of work.
		rpTrajStale, rpTrajStaleAt = true, frameNowMs
		return false
	}
	if rpTrajStale {
		if frameNowMs-rpTrajStaleAt < rpTrajSettleMs {
			return false
		}
		rpTrajStale = false
		span := RecurrenceSpan(lastFlowMode, float64(rpWin)/rpTrajWinDiv, rpN)
		rpTrajSeries = TrajectorySeries(lastFlowMode, rpN, span)
		rpTrajDiam = RecurrenceDiameter(rpTrajSeries, 3)
		rpTrajBuilt = false
		// A different curve, so the RQA read off it is an answer about a
		// different object and the strip chart takes a seam. Counted rather
		// than flagged because rqaConfigNow compares values and has no way to
		// clear a flag it did not set — a counter it can only ever observe
		// changing needs no handshake.
		rpTrajGen++
	}
	if rpTrajSeries == nil || rpTrajDiam <= 0 {
		// No vector field (the last mode was geometry or a map), or a run that
		// diverged before it had rpN points. Leaving the previous picture up is
		// the honest thing here: the panel names the system it belongs to, so
		// what is on screen is still labeled correctly.
		return false
	}
	// The series is static, so the matrix only has to be refilled when ε moves.
	// Every other frame is a redraw of a texture that is already correct — and
	// ε, unlike the source parameters, is cheap enough to follow immediately,
	// so it gets no settle delay.
	if rpTrajBuilt && rpTrajEps == rpEps {
		return false
	}
	rpTrajEps, rpTrajBuilt = rpEps, true
	RecurrenceMatrixVec(rpTrajSeries, 3, float64(rpEps)*rpTrajDiam, rpMat)
	return true
}

// ── The RQA readout ──────────────────────────────────────────────────────
//
// Three numbers beside the plot, in the mode's own parameter grid rather than
// in the Analysis module next to the Lyapunov exponent. That placement is
// deliberate: the Analysis module measures THE MODEL ON SCREEN and switches on
// and off for the whole rack, while these three describe THIS PLOT AT THIS ε —
// move ε and they move, which is not a property of the system at all. Filed
// under Analysis they would read as facts about the attractor. Filed under the
// knob that moves them, they read as what they are: the number ε is turned by,
// plus the two that say whether the texture is structure or speckle.
//
// The same tick drives the strip chart in the cell beside it — the history of
// these three numbers, which is the thing RQA is actually for (rqaseries.go).

var (
	rpRQAEl   js.Value
	rpRQA     RQAResult
	rpRQANext float64 // frameNowMs the next measurement is due

	// rpMatDirty says the matrix has changed since the last scan. It is sticky
	// rather than the caller's per-frame "fresh", because the two clocks do not
	// line up: the tick fires every tenth frame or so, and for the trajectory
	// source the matrix is rebuilt on one frame and then not again for a long
	// time. Asking "was it fresh THIS frame?" at the tick would answer no
	// almost every time and the readout would sit on a stale number forever.
	rpMatDirty bool
)

// rpMaybeMeasure recomputes the scalars from the matrix, at most every
// RQASamplePeriodMs, and only while the readout is actually on the panel —
// there is no reason to scan 64 KB for a number nothing is displaying. The
// scan is skipped again when the matrix has not moved since the last one,
// which is the common case for the trajectory source and free for the others.
//
// The SERIES is fed on every tick regardless, scan or no scan: its axis is
// wall-clock time, so a value that has not changed is still a reading, and a
// tick that produced no reading at all has to become a slot in the record
// rather than nothing. That is what keeps the chart from splicing across the
// stretches it was not looking.
func rpMaybeMeasure() {
	if !rpRQAEl.Truthy() || frameNowMs < rpRQANext {
		return
	}
	rpRQANext = frameNowMs + RQASamplePeriodMs
	if rpMatDirty {
		rpMatDirty = false
		rpRQA = RQA(rpMat, rpN)
		rpRQAEl.Set("textContent", rpFormatRQA(rpRQA))
	}
	rqaSample(frameNowMs, rpRQA)
}

// rpFormatRQA renders the three scalars as percentages at a FIXED WIDTH, for
// the reason formatLyap gives about the Lyapunov readout: a number that changes
// width makes the whole cell jump, and this one updates several times a second.
// RR keeps a decimal because its useful range is the bottom few percent, where
// whole numbers would read 2, 3, 2, 3 and say nothing.
func rpFormatRQA(r RQAResult) string {
	if r.Lit == 0 {
		return " --.- --- ---"
	}
	pad := func(s string, w int) string {
		for len(s) < w {
			s = " " + s
		}
		return s
	}
	return pad(strconv.FormatFloat(r.RR*100, 'f', 1, 64), 5) + " " +
		pad(strconv.Itoa(int(r.DET*100+0.5)), 3) + " " +
		pad(strconv.Itoa(int(r.LAM*100+0.5)), 3)
}

// appendRecurrenceRQA adds the RQA cell to the recurrence parameter grid, the
// way appendTakensEstimate adds the Takens mode's MEAS cell — into the GRID
// rather than below it, because the grid is the height-bounded column-wrap
// container and anything appended after it is clipped.
func appendRecurrenceRQA(grid js.Value) {
	card := doc.Call("createElement", "div")
	card.Set("className", "punit")

	lbl := doc.Call("createElement", "span")
	lbl.Set("className", symClass("u-lbl", false))
	lbl.Set("textContent", "rqa")
	card.Call("appendChild", lbl)

	rpRQAEl = doc.Call("createElement", "span")
	rpRQAEl.Set("className", "led counter-led")
	rpRQAEl.Set("title", "Recurrence quantification, as percentages: RR · DET · LAM. "+
		"RR is how much of the square is lit — the number to turn ε by, and 1–5% is the readable range. "+
		"DET is the share of those points lying on diagonal lines, which is what separates a system from "+
		"noise: an orbit reads near 100, white noise near 0. LAM is the share on vertical lines — states "+
		"the system sat in rather than passed through, so high LAM against lower DET is intermittency. "+
		"The line of identity is left out of DET: every point recurs with itself, and counting that in "+
		"would give noise a confident score for nothing.")
	rpRQAEl.Set("textContent", rpFormatRQA(rpRQA))
	card.Call("appendChild", rpRQAEl)

	// A source LABEL rather than a second knob. The trajectory source plots
	// whichever flow was on screen last, and a plot of an unnamed system is not
	// a measurement of anything — the same reason, and the same wording, as the
	// bifurcation explorer's SWEEP label, which picks its system the same way.
	row := doc.Call("createElement", "span")
	row.Set("className", "grp")
	note := doc.Call("createElement", "span")
	note.Set("className", "plabel")
	if int(rpSrc) == rpSrcTraj {
		note.Set("textContent", modeInfo[lastFlowMode].Label)
		note.Set("title", "The system being plotted — the most recent flow mode. Switch to an attractor, tune it, then come back.")
	} else {
		note.Set("textContent", "audio in")
		note.Set("title", "The live audio source feeds this plot; pick it in the Audio module.")
	}
	row.Call("appendChild", note)
	card.Call("appendChild", row)
	grid.Call("appendChild", card)
}
