package attractor

import (
	"github.com/0magnet/chaosrack/pkg/recurrence"
	"github.com/0magnet/chaosrack/pkg/takens"
)

// The recurrence plot's matrix, shared by the page (recurrence_js.go, where
// the file comment says what it shows and why it is built as it is) and a
// native rack: the knobs, the decimated window or the cached trajectory, and
// the rpN×rpN cells. Where the samples come from, the texture, and the RQA
// readout are each side's own.

const (
	// rpN is the matrix side, and the texture is rpN×rpN. 256 columns over
	// the window is enough for the diagonals to be legible and costs 65536
	// comparisons per frame, which is nothing next to a frame of audio.
	rpN = 256

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

	// rpMaxTau is the τ knob's maximum, and rpMaxLookback the deepest history
	// behind the plot window any (m, τ) can ask for. The page's ring is sized
	// for THAT rather than for the current setting, at a cost of 3584
	// float32s — 14 KB — because a ring that grows when a knob moves also
	// resets its write cursor, and resetting the cursor throws the buffered
	// audio away: turning m from 1 to 8 would blank the plot eight times on
	// the way. Sized for the worst case once, m and τ are free to move.
	rpMaxTau = takens.TauMax

	// rpMaxTauSamples is rpMaxTau in SOURCE samples rather than in knob units.
	// The knob counts samples at takens.RefRate (see takens.TauSamples), so a
	// source running faster than the reference turns the same knob position
	// into more real samples — twice as many on a 96 kHz device. The ring has
	// to hold the deepest lookback any rate can produce, not the deepest the
	// knob's number looks like, or the delay coordinates read behind the start
	// of the buffer on exactly the hardware nobody tests on.
	//
	// 2x covers every rate a browser AudioContext reports in practice: 44.1 and
	// 48 kHz overwhelmingly, 88.2 and 96 on the interfaces that offer them. The
	// cost of the headroom is the ring, and the ring is 14 KB.
	rpMaxTauSamples = 2 * rpMaxTau
	rpMaxLookback   = (rpMaxDim - 1) * rpMaxTauSamples
)

// RecurrenceSide is the recurrence plot's side, in cells.
const RecurrenceSide = rpN

// Recurrence is a recurrence plot's matrix and what it is built from.
type Recurrence struct {
	vec []float64 // the rpN points of the current window, flat, m wide
	mat []byte    // rpN*rpN, one byte per cell

	// win is the span the square covers: milliseconds for the audio sources
	// — how much history the plot is of — and, divided by rpTrajWinDiv, the
	// trajectory source's span in the system's own time units. 100 ms is a few
	// dozen periods of a musical pitch across the square, and at 48 kHz it
	// decimates by 18, which leaves the fundamental range intact after the box
	// filter in FromAudio.
	//
	// eps is the recurrence threshold as a fraction of the source's scale
	// (see the page's file comment on why that scale is never taken from the
	// picture).
	//
	// src picks which state is plotted, and dim is the embedding dimension
	// the embed source builds its delay vectors at.
	win        float32
	eps        float32
	src        float32
	dim        float32
	trajSeries []float64 // rpN points of (x,y,z), flat
	trajDiam   float64   // its diameter, the ε normalizer
	trajMode   string    // the flow it belongs to
	trajWin    float32   // and the WIN knob it was integrated at
	trajVals   []float32 // and that flow's parameter values

	// trajStale is a flag rather than a "trajStaleAt > 0" sentinel on the
	// timestamp beside it, because the page's frameNowMs is the raw rAF
	// timestamp and zero is a value it can genuinely hold — it is zero until
	// the first frame lands. With the sentinel, a mode entered on that frame
	// recorded staleness as 0, read it back as "settled", and never integrated
	// anything: a permanently blank square with no error anywhere.
	trajStale   bool
	trajStaleAt float64 // nowMs when the knobs last moved
	trajEps     float32 // the ε the matrix was built with
	trajBuilt   bool    // a matrix has been built from the current series
	trajGen     int     // bumped on every re-integration; see rqaConfigNow
}

// NewRecurrence is a recurrence plot at the page's defaults.
func NewRecurrence() *Recurrence {
	return &Recurrence{win: 100, eps: 0.05, src: rpSrcAudio, dim: 3}
}

// RecurrenceKnobs are the plot's own knobs, by the page's ids. Its τ is the
// Takens mode's, takens-tau, which FromAudio is handed.
var RecurrenceKnobs = []string{"rec-src", "rec-win", "rec-eps", "rec-dim"}

func (r *Recurrence) field(id string) *float32 {
	switch id {
	case "rec-src":
		return &r.src
	case "rec-win":
		return &r.win
	case "rec-eps":
		return &r.eps
	case "rec-dim":
		return &r.dim
	}
	return nil
}

// Set turns knob id to v, reporting whether the plot has it.
func (r *Recurrence) Set(id string, v float32) bool {
	p := r.field(id)
	if p == nil {
		return false
	}
	*p = v
	return true
}

// Trajectory reports whether the SRC knob plots the last flow's trajectory
// rather than the audio.
func (r *Recurrence) Trajectory() bool { return int(r.src) == rpSrcTraj }

// Matrix is the plot's cells, row by row from the oldest, nonzero where lit.
func (r *Recurrence) Matrix() []byte {
	r.alloc()
	return r.mat
}

func (r *Recurrence) alloc() {
	if r.mat == nil {
		r.mat = make([]byte, rpN*rpN)
		r.vec = make([]float64, rpN*rpMaxDim)
	}
}

// embedDim is the m knob clamped to what the buffers hold, and 1 for the
// raw-audio position — which IS m = 1, and shares the code path for it.
func (r *Recurrence) embedDim() int {
	if int(r.src) != rpSrcEmbed {
		return 1
	}
	m := int(r.dim)
	if m < 1 {
		return 1
	}
	if m > rpMaxDim {
		return rpMaxDim
	}
	return m
}

// rpWindow converts the WIN knob into a sample count and the stride that fits
// it into rpN columns. Split out so it can be tested on its own, as
// takensWindow is: it decides what the picture is OF.
func rpWindow(winMS float32, sampleRate int) (span, stride int) {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	span = max(int(winMS/1000*float32(sampleRate)), rpN)
	stride = max(span/rpN, 1)
	return stride * rpN, stride
}

// tauSamples is the τ knob in samples at rate sr, clamped to the deepest
// lookback the page's ring holds.
//
// Converted from the knob's reference-rate unit to this source's samples —
// the same delay in TIME whatever the source runs at, which is the whole of
// takens.TauSamples' argument — and then clamped, because the ring is sized
// from rpMaxLookback: a τ past that bound, from a hand-edited permalink or
// from a sample rate higher than the headroom allows for, would ask for
// history behind the start of the buffer, and the index arithmetic in
// FromAudio would go negative rather than merely wrong.
func tauSamples(tauKnob float32, sr int) int {
	return min(takens.TauSamples(tauKnob, sr), rpMaxTauSamples)
}

// AudioNeed is how many of the newest samples at rate sr FromAudio reads at
// τ knob tauKnob: the window and its deepest delay.
func (r *Recurrence) AudioNeed(sr int, tauKnob float32) int {
	span, _ := rpWindow(r.win, sr)
	return span + (r.embedDim()-1)*tauSamples(tauKnob, sr)
}

// FromAudio fills the matrix from the newest window of a signal at rate sr —
// as raw samples (m = 1) or as delay vectors at τ knob tauKnob. The signal is
// ring, a circular buffer whose sample i is ring[i%len(ring)], written up to
// sample w; a plain slice is a ring written up to its length. Reports whether
// the matrix was rebuilt: with less than AudioNeed of signal, it is not.
func (r *Recurrence) FromAudio(ring []float32, w, sr int, tauKnob float32) bool {
	r.alloc()
	dim := r.embedDim()
	tau := tauSamples(tauKnob, sr)
	span, stride := rpWindow(r.win, sr)
	// The delay coordinates read BACKWARDS from the start of the plot window,
	// in source samples — τ is a delay in the signal, not in decimated columns,
	// which is what makes it the same τ the Takens mode measured. So the ring
	// has to hold the window plus the deepest delay, and the window's own base
	// index stays where it was.
	//
	// span + lookback, not span: with the delays included, requiring only span
	// would index behind the start of the ring on the first frames after a
	// resize. avail ≤ w, so this also guarantees base − lookback ≥ 0 and the
	// modulo below never sees a negative.
	if avail := min(w, len(ring)); avail < span+(dim-1)*tau {
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
	rn := len(ring)
	base := w - span
	inv := 1 / float64(stride)
	for i := range rpN {
		for c := range dim {
			off := base + i*stride - c*tau
			var sum float32
			for k := range stride {
				sum += ring[(off+k)%rn]
			}
			r.vec[i*dim+c] = float64(sum) * inv
		}
	}
	recurrence.MatrixVec(r.vec[:rpN*dim], dim, float64(r.eps)*recurrence.VectorScale(dim), r.mat)
	return true
}

// trajChanged reports whether the source system, its parameters or the WIN
// knob differ from what the cached trajectory was integrated from, recording
// the current values as it goes.
//
// Comparing the VALUES rather than subscribing to an invalidation hook is the
// point: an edit that reaches a parameter by any route at all — a knob, a
// permalink, a preset recall, Reset All, a patch memory, MIDI, audio modulation
// — moves a float that this then sees. Nothing has to remember to tell it, and
// nothing added later can forget to.
func (r *Recurrence) trajChanged(flow string, vals []float32) bool {
	changed := flow != r.trajMode || r.win != r.trajWin || len(r.trajVals) != len(vals)
	if changed {
		r.trajVals = make([]float32, len(vals))
	}
	for i, v := range vals {
		if r.trajVals[i] != v {
			r.trajVals[i] = v
			changed = true
		}
	}
	r.trajMode, r.trajWin = flow, r.win
	return changed
}

// FromTrajectory fills the matrix from flow's own trajectory, its constants
// now vals, re-integrating it only when the system has changed AND has then
// held still since nowMs. Reports whether the matrix was rebuilt.
func (r *Recurrence) FromTrajectory(flow string, vals []float32, nowMs float64) bool {
	r.alloc()
	if r.trajChanged(flow, vals) {
		// Do not integrate yet. See rpTrajSettleMs: a drag changes a value per
		// pixel, and Chen's trajectory is 24 ms of work.
		r.trajStale, r.trajStaleAt = true, nowMs
		return false
	}
	if r.trajStale {
		if nowMs-r.trajStaleAt < rpTrajSettleMs {
			return false
		}
		r.trajStale = false
		span := recurrence.Span(flow, float64(r.win)/rpTrajWinDiv, rpN)
		r.trajSeries = recurrence.TrajectorySeries(flow, rpN, span)
		r.trajDiam = recurrence.Diameter(r.trajSeries, 3)
		r.trajBuilt = false
		// A different curve, so the RQA read off it is an answer about a
		// different object and the strip chart takes a seam. Counted rather
		// than flagged because rqaConfigNow compares values and has no way to
		// clear a flag it did not set — a counter it can only ever observe
		// changing needs no handshake.
		r.trajGen++
	}
	if r.trajSeries == nil || r.trajDiam <= 0 {
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
	if r.trajBuilt && r.trajEps == r.eps {
		return false
	}
	r.trajEps, r.trajBuilt = r.eps, true
	recurrence.MatrixVec(r.trajSeries, 3, float64(r.eps)*r.trajDiam, r.mat)
	return true
}
