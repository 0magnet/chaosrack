//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/glctx"
	"github.com/0magnet/chaosrack/pkg/recurrence"
	"github.com/0magnet/chaosrack/pkg/takens"
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
// The matrix, the two normalizers and the RQA scalars are all in pkg/recurrence,
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
//     same thing at every m (recurrence.VectorScale).
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
//   - RQA runs at recurrence.RQASamplePeriodMs rather than per frame, because 283 µs for
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
//     pkg/recurrence caps the 24 ms; the settle delay caps how often it is paid.
//
// The matrix and the knobs it is built at are Recurrence (recurrenceplot.go),
// which a native rack draws too. The constants the notes above name — rpN,
// the SRC positions, rpTrajWinDiv, rpTrajSettleMs and the lookback the ring
// is sized for — are there with it.
const (
	// rpDrainCap bounds the per-frame drain the same way the Takens mode
	// does: the signal generator synthesizes on demand and always fills the
	// buffer, so an uncapped drain-until-dry loop never terminates.
	rpDrainCap = 16384
)

// recurrencePlot is the Recurrence mode: the plot, the ring of the tap it
// reads, its texture, and the RQA readout.
type recurrencePlot struct {
	Recurrence
	texture js.Value
	u8      js.Value // reused Uint8Array, rpN*rpN bytes
	ready   bool
	ring    []float32
	w       int // monotonic write cursor
	scratch []float32
	vals    []float32 // the last flow's parameter values, read each frame

	rqaEls  [3]js.Value // RR, DET and LAM on the readout line
	rqa     recurrence.RQAResult
	rqaNext float64 // frameNowMs the next measurement is due

	// matDirty says the matrix has changed since the last scan. It is sticky
	// rather than the caller's per-frame "fresh", because the two clocks do not
	// line up: the tick fires every tenth frame or so, and for the trajectory
	// source the matrix is rebuilt on one frame and then not again for a long
	// time. Asking "was it fresh THIS frame?" at the tick would answer no
	// almost every time and the readout would sit on a stale number forever.
	matDirty bool
}

var rp = recurrencePlot{Recurrence: *NewRecurrence()}

func init() {
	registerGenerate("recurrence", rp.generateRecurrence)
	attractorParams["recurrence"] = []paramDef{
		{"rec-src", "src", &rp.src, rpSrcAudio, rpSrcAudio, rpSrcTraj, 1},
		{"rec-win", "win", &rp.win, 100, 20, 2000, 20},
		{"rec-eps", "ε", &rp.eps, 0.05, 0.005, 0.5, 0.005},
		{"rec-dim", "m", &rp.dim, 3, 1, rpMaxDim, 1},
		// The Takens mode's τ, under the same DOM id on purpose — see the SRC
		// note in the file comment. Def/Min/Max/Step must stay identical to the
		// row in takens_js.go, or Reset All resets one knob to two different
		// numbers depending on which of the two maps it walks last.
		{"takens-tau", "τ", &emb.tau, takens.TauDef, 1, takens.TauMax, 1},
	}
}

func (r *recurrencePlot) initRecurrencePlot() {
	if r.ready {
		return
	}
	r.texture = glctx.GL.Call("createTexture")
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), r.texture)
	// NEAREST, not LINEAR: the cells are a yes/no answer, and interpolating
	// between them invents half-recurrences that are not in the signal — at
	// this size it also smears the single-pixel diagonals into a haze.
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MIN_FILTER"), glctx.GL.Get("NEAREST"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MAG_FILTER"), glctx.GL.Get("NEAREST"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_S"), glctx.GL.Get("CLAMP_TO_EDGE"))
	glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_T"), glctx.GL.Get("CLAMP_TO_EDGE"))

	// LUMINANCE rather than RGBA: a binary matrix needs one byte per cell, not
	// four, and the shared textured shader reads the single channel into all
	// three color channels for free.
	r.alloc()
	r.u8 = js.Global().Get("Uint8Array").New(rpN * rpN)
	glctx.GL.Call("texImage2D",
		glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("LUMINANCE"),
		rpN, rpN, 0,
		glctx.GL.Get("LUMINANCE"), glctx.GL.Get("UNSIGNED_BYTE"), r.u8)

	r.scratch = make([]float32, 8192)
	r.ready = true
}

// generateRecurrence rebuilds the matrix from whichever source the SRC knob
// names and uploads it. Called from generateForMode.
func (r *recurrencePlot) generateRecurrence() {
	if !r.ready {
		r.initRecurrencePlot()
	}
	var fresh bool
	if int(r.src) == rpSrcTraj {
		fresh = r.fillFromTrajectory()
	} else {
		fresh = r.fillFromAudio()
		// Only the audio sources have a source to report on. The trajectory one
		// must not reach this: it would spin the microphone up and put an
		// "allow access?" overlay over a picture of the Lorenz attractor.
		aud.maybeShowAudioStatus()
	}
	if fresh {
		r.matDirty = true
		js.CopyBytesToJS(r.u8, r.mat)
		glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), r.texture)
		glctx.GL.Call("texSubImage2D",
			glctx.GL.Get("TEXTURE_2D"), 0, 0, 0, rpN, rpN,
			glctx.GL.Get("LUMINANCE"), glctx.GL.Get("UNSIGNED_BYTE"), r.u8)
	}
	// Outside the fresh branch, and that is not a tidy-up. The strip chart's
	// axis is TIME, so it needs a slot per interval whether or not the matrix
	// moved — and for the trajectory source the matrix almost never moves,
	// because the series is static and cached. Measured on the fresh frames
	// only, the chart would advance in bursts on a knob turn and stand still
	// in between, which is a chart of edits rather than of the system.
	// Recomputation is still gated: rp.matDirty says whether there is anything
	// new to scan, so a frozen picture costs the readout nothing and the
	// series records the value it still holds.
	r.maybeMeasure()
	texp.drawTexturedSquare(r.texture)
}

// fillFromAudio drains the live audio into the ring and fills the matrix from
// the newest window. Reports whether the matrix was rebuilt; when there is not
// yet enough audio the previous frame stays on the texture rather than
// flickering to black.
func (r *recurrencePlot) fillFromAudio() bool {
	src := aud.ensureAudioSource()
	sr := 24000
	if src.SampleRate() > 0 {
		sr = src.SampleRate()
	}
	span, _ := rpWindow(r.win, sr)
	if need := span + rpMaxLookback + 1; len(r.ring) < need {
		r.ring = make([]float32, need+need/2)
		r.w = 0
	}
	if src.Ready() {
		for drained := 0; drained < rpDrainCap; {
			n := tapRead(&rpCursor, r.scratch)
			if n <= 0 {
				break
			}
			for i := range n {
				r.ring[r.w%len(r.ring)] = r.scratch[i]
				r.w++
			}
			drained += n
			if n < len(r.scratch) {
				break
			}
		}
	}
	return r.FromAudio(r.ring, r.w, sr, emb.tau)
}

// ── The trajectory source ────────────────────────────────────────────────

// flowVals is the last flow mode's parameter values now.
func (r *recurrencePlot) flowVals() []float32 {
	r.vals = r.vals[:0]
	for _, p := range attractorParams[bif.lastFlowMode] {
		r.vals = append(r.vals, *p.Value)
	}
	return r.vals
}

// fillFromTrajectory draws the most recent flow mode's own trajectory.
// Reports whether the matrix was rebuilt this frame.
func (r *recurrencePlot) fillFromTrajectory() bool {
	return r.FromTrajectory(bif.lastFlowMode, r.flowVals(), frameNowMs)
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
// these three numbers, which is the thing RQA is actually for (pkg/recurrence).

// maybeMeasure recomputes the scalars from the matrix, at most every
// recurrence.RQASamplePeriodMs, and only while the readout is actually on the panel —
// there is no reason to scan 64 KB for a number nothing is displaying. The
// scan is skipped again when the matrix has not moved since the last one,
// which is the common case for the trajectory source and free for the others.
//
// The SERIES is fed on every tick regardless, scan or no scan: its axis is
// wall-clock time, so a value that has not changed is still a reading, and a
// tick that produced no reading at all has to become a slot in the record
// rather than nothing. That is what keeps the chart from splicing across the
// stretches it was not looking.
func (r *recurrencePlot) maybeMeasure() {
	if !r.rqaEls[0].Truthy() || frameNowMs < r.rqaNext {
		return
	}
	r.rqaNext = frameNowMs + recurrence.RQASamplePeriodMs
	if r.matDirty {
		r.matDirty = false
		r.rqa = recurrence.RQA(r.mat, rpN)
		for i, s := range rpRQAFields(r.rqa) {
			setDotText(r.rqaEls[i], s)
		}
	}
	rqa.sample(frameNowMs, r.rqa)
}

// rpRQAFields renders the three scalars as percentages, one for each of the
// three half displays: RR with a decimal, because its useful range is the
// bottom few percent where whole numbers would read 2, 3, 2, 3 and say
// nothing, and DET and LAM whole. A plot with nothing lit has no readings.
func rpRQAFields(r recurrence.RQAResult) [3]string {
	if r.Lit == 0 {
		return [3]string{"--.-", "---", "---"}
	}
	rr := strconv.FormatFloat(r.RR*100, 'f', 1, 64)
	if r.RR*100 >= 99.95 {
		rr = "100" // four characters is what the display holds
	}
	return [3]string{rr, strconv.Itoa(int(r.DET*100 + 0.5)), strconv.Itoa(int(r.LAM*100 + 0.5))}
}

// rpRQATips are the three readouts' tooltips.
var rpRQATips = [3]string{
	doc("ro.rr"),
	doc("ro.det"),
	doc("ro.lam"),
}

// appendRecurrenceRQA puts RR, DET and LAM on the model's readout line
// (liveReadoutHost), a half display each.
func (r *recurrencePlot) appendRecurrenceRQA(host js.Value) {
	f := rpRQAFields(r.rqa)
	for i, l := range []string{"rr", "det", "lam"} {
		r.rqaEls[i] = liveReadout(host, l, dispHalfChars, f[i], rpRQATips[i])
	}
}
