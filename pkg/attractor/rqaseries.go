package attractor

import "math"

// The RQA scalars as a TIME SERIES — the ring behind the strip chart that sits
// beside the recurrence plot, and the fixed scales it is drawn against.
//
// RQA (recurrence.go) reads three numbers off the plot: RR, the density; DET,
// the share of recurrence points lying on diagonals; LAM, the share on
// verticals. Measured live at the mode's three sources they come out
//
//	source          RR    DET   LAM
//	audio          8.8%    29    36
//	embed (m=3)    1.8%    88    39
//	traj (Lorenz)  2.5%    99    72
//
// which is the textbook ordering — noise, its delay reconstruction, a
// deterministic flow — and is exactly why a single instant of them is a waste.
// What RQA is FOR is watching structure change: Marwan et al. (Phys. Rep. 438
// (2007) 237) spend most of their length on windowed RQA precisely because the
// interesting object is DET(t), not DET. A speaker changing, an instrument
// entering, a system leaving a periodic window — each of those is a step in one
// of these three numbers, and a readout that only ever shows the present value
// shows the step for one sixth of a second and then forgets it.
//
// UNTAGGED, for the reason recurrence.go is: the ring is index arithmetic over
// a wrapping buffer with a monotonic cursor, which is the classic place to be
// off by one in a way that still looks plausible on screen — a chart drawn
// backwards, or one that splices two moments together across a stall, is not
// obviously wrong to the eye and is completely wrong as a measurement. The
// scales are here too, so the claim "this mapping makes the readable band
// visible and never clips" is a test rather than an opinion.
//
// ── SAMPLING RATE ────────────────────────────────────────────────────────
//
// One sample per RQA measurement, at the rate the readout was ALREADY being
// measured at. The series adds no computation whatsoever: RQA is a 283 µs scan
// of a matrix that had to be built in order to be drawn, the js side has rate-
// limited it to RQASamplePeriodMs since the readout existed, and all this does
// is keep the answers instead of overwriting them.
//
// That is the whole argument for the number. Faster is not free — the matrix
// underneath costs 292 µs for raw audio and up to 24 ms for a Chen trajectory,
// and per-frame recomputation of a chart that advances one pixel every ten
// frames is work nobody can see. Slower misses the transition the chart exists
// to show. And 160 ms is matched to the thing being measured rather than to the
// display: the plot's own window (the WIN knob) is 20–2000 ms of audio, so at
// the default 100 ms consecutive samples are of DISJOINT windows — the fastest
// honest rate — while at 2000 ms they overlap by 92% and the trace is smooth by
// construction, which is a true statement about a two-second window and not a
// filter anyone applied.
//
// Nothing here can be provoked into a recomputation storm by a knob drag,
// because nothing here recomputes at all. The guards that already exist do the
// work: rpTrajSettleMs holds the expensive trajectory integration until the
// knobs have been still for 200 ms, the trajectory cache refills the matrix
// only when ε moves, and this ring is fed from the far side of both.
//
// ── HISTORY LENGTH ───────────────────────────────────────────────────────
//
// RQASeriesLen slots, one per column of the chart — the spectrogram's rule
// (one FFT per texture column, no resampling on the way in) applied to a much
// smaller picture. 256 of them at 160 ms is RQASeriesSpanMs: 41 seconds, which
// is the span a transition and its context have to fit inside. 256 rather than
// some rounder number because it is the width in pixels of the strip the panel
// cell can show; storing more would be history that is never drawn, and storing
// less would leave the left of the chart permanently blank.
//
// ── GAPS, AND WHY THE AXIS IS TIME RATHER THAN SAMPLE NUMBER ─────────────
//
// A ring of the last 256 samples drawn end to end is a chart of SAMPLE NUMBER,
// and it silently claims the samples are evenly spaced. They are not. Background
// a tab and requestAnimationFrame stops entirely; come back thirty seconds later
// and the naive chart joins the two sides of that hole with a straight line
// through a period nobody measured. That line is indistinguishable from a slow
// real drift, which is the exact reading this display is for.
//
// So a slot is a TIME SLOT. Push works out how many intervals went by
// unmeasured and writes that many empty slots before the new one, and the
// painter breaks the trace across them. The tolerance is deliberately loose —
// gaps are only inserted past two whole intervals — because a single long frame
// (a 24 ms Chen integration landing next to a GC pause) is jitter, not a hole,
// and a chart stippled with one-slot breaks every time the browser hiccuped
// would be unreadable for no gain.
//
// The same mechanism marks a change of MEASUREMENT. Move ε, switch source, turn
// m or τ, or re-integrate the trajectory under a different system, and the
// numbers after are not comparable with the numbers before — they are answers
// to a different question about a different object. The alternatives were to
// clear the history (which makes it impossible to see what your own ε change
// did, and that is one of the things the chart is good for) or to splice
// (which draws a step that looks exactly like a step in the signal). Writing a
// one-slot break says what actually happened: the old history stands, it is
// still true of what it was, and there is a visible seam where the question
// changed.

const (
	// RQASamplePeriodMs is both the rate limit on the RQA scan and the spacing
	// of the series, because they are one tick — see the sampling-rate note
	// above. Three numbers changing sixty times a second are unreadable, the
	// scan is 283 µs, and 160 ms is a hair over six samples a second, which is
	// fast enough to put an instrument entering inside one slot of when it did.
	RQASamplePeriodMs = 160

	// RQASeriesLen is the ring's length in slots: one per column of the chart.
	RQASeriesLen = 256

	// RQASeriesSpanMs is what that comes to in wall-clock time — 40.96 s of
	// scrollback. Stated as a constant because it is the number a reader of the
	// chart needs (the tooltip quotes it) and the number a change to either of
	// the two above silently moves.
	RQASeriesSpanMs = RQASeriesLen * RQASamplePeriodMs

	// RQAReadableLo and RQAReadableHi bracket the recurrence rate a plot can be
	// read at, the band recurrence.go names for the ε knob: below it there is
	// nothing but the diagonal, above it the square saturates. The chart shades
	// it behind the RR trace, which turns "turn ε until RR is a few percent"
	// from a sentence in a tooltip into something visible.
	RQAReadableLo = 0.01
	RQAReadableHi = 0.05
)

// RQATrace names one of the three quantities the chart draws.
type RQATrace int

// The traces, in the order they are stacked down the chart: the density first,
// because it is the one ε is turned by and the one that says whether the other
// two mean anything, then the two structure measures.
const (
	RQATraceRR RQATrace = iota
	RQATraceDET
	RQATraceLAM
	RQATraceCount
)

// String is the trace's label on the chart.
func (t RQATrace) String() string {
	switch t {
	case RQATraceRR:
		return "RR"
	case RQATraceDET:
		return "DET"
	case RQATraceLAM:
		return "LAM"
	}
	return ""
}

// RQASample is one slot of the series: a reading, or the absence of one.
//
// float32 rather than float64 because the ring is 256 of these and the values
// are fractions displayed to two significant figures; the exact number lives in
// the LED beside the chart, which is fed from the RQAResult directly.
type RQASample struct {
	RR, DET, LAM float32
	// OK distinguishes a reading from a gap. It is a field rather than a
	// sentinel value because every sentinel is a legal reading: RR is genuinely
	// 0 over silence with ε small, and DET is genuinely 0 for a plot with no
	// diagonals in it. A chart that drew "no measurement" as zero would show
	// determinism collapsing every time the tab was in the background.
	OK bool
}

// Value returns the sample's reading for one trace.
func (s RQASample) Value(t RQATrace) float64 {
	switch t {
	case RQATraceRR:
		return float64(s.RR)
	case RQATraceDET:
		return float64(s.DET)
	case RQATraceLAM:
		return float64(s.LAM)
	}
	return 0
}

// RQASeries is the ring. The zero value is an empty series; the buffer is
// allocated on the first write, so a mode that is never entered costs nothing.
type RQASeries struct {
	buf  []RQASample
	w    int     // monotonic write cursor; buf index is w % RQASeriesLen
	at   float64 // wall-clock ms of the slot at w-1
	have bool    // ...and whether there is one, because 0 is a real timestamp
}

// Push records one measurement taken at nowMs, inserting empty slots for any
// whole intervals that went by without one.
//
// A result with no lit cells at all is not a measurement — it is what the
// matrix looks like before there is enough audio to fill the window, or when
// the trajectory source has nothing to plot — so it goes in as a gap rather
// than as three zeros, for the reason RQASample.OK gives.
func (s *RQASeries) Push(nowMs float64, r RQAResult) {
	if r.Lit == 0 {
		s.Break(nowMs)
		return
	}
	s.fillGaps(nowMs)
	s.write(nowMs, RQASample{RR: float32(r.RR), DET: float32(r.DET), LAM: float32(r.LAM), OK: true})
}

// Break writes one empty slot, marking the point where the measurement itself
// changed — a new ε, a new source, a re-integrated trajectory. See the note on
// gaps above for why this is a seam rather than a reset or a splice.
func (s *RQASeries) Break(nowMs float64) {
	s.fillGaps(nowMs)
	s.write(nowMs, RQASample{})
}

// Snapshot copies the newest len(dst) slots into dst, oldest first, so dst is
// the chart left to right with the newest reading in the last column. Slots
// that have never been written come back as gaps. Returns how many of them are
// real slots, which is what tells a caller the chart is still filling.
//
// A copy rather than an index-into-the-ring accessor because the painter walks
// the whole window three times, once per trace, and doing the wrap arithmetic
// 768 times per redraw is 768 chances to get it wrong in a way that draws
// something plausible.
func (s *RQASeries) Snapshot(dst []RQASample) int {
	for i := range dst {
		dst[i] = RQASample{}
	}
	if s.buf == nil || len(dst) == 0 {
		return 0
	}
	n := s.w
	if n > RQASeriesLen {
		n = RQASeriesLen
	}
	if n > len(dst) {
		n = len(dst)
	}
	// Right-aligned: the i-th newest slot goes i columns in from the right, so
	// a half-full series leaves the LEFT of the chart empty. That is the
	// direction it has to be — the newest reading is the one being watched, and
	// it must not walk across the chart as the buffer fills.
	for i := 0; i < n; i++ {
		dst[len(dst)-1-i] = s.buf[(s.w-1-i)%RQASeriesLen]
	}
	return n
}

// fillGaps writes an empty slot for each whole sampling interval that elapsed
// beyond the tolerance since the last one.
//
// int(elapsed/period)-1 is a FLOOR minus one, not a rounding: it inserts
// nothing until two whole intervals have passed, which absorbs the frame
// jitter every value here is subject to (the tick fires on the first frame at
// or after its due time, so 160 ms nominal arrives as 160–176 ms, and a slow
// frame can push it past 250). Anything longer than two intervals is a real
// hole and is drawn as one.
//
// A clock that went backwards inserts nothing rather than a negative count.
// rAF timestamps are monotonic, but the tests drive this directly and a
// negative here would panic on the loop bound rather than misdraw.
func (s *RQASeries) fillGaps(nowMs float64) {
	if !s.have {
		return
	}
	n := int((nowMs-s.at)/RQASamplePeriodMs) - 1
	if n <= 0 {
		return
	}
	// More than the ring holds means nothing survives from before the hole, so
	// there is no point writing a thousand empty slots to prove it.
	if n > RQASeriesLen {
		n = RQASeriesLen
	}
	// base is read once: write moves s.at, so reading it inside the loop would
	// compound the offset and stamp the gaps at 1, 3, 6… intervals out.
	base := s.at
	for i := 0; i < n; i++ {
		s.write(base+float64(i+1)*RQASamplePeriodMs, RQASample{})
	}
}

// write puts one slot in the ring and records when it was.
func (s *RQASeries) write(atMs float64, v RQASample) {
	if s.buf == nil {
		s.buf = make([]RQASample, RQASeriesLen)
	}
	s.buf[s.w%RQASeriesLen] = v
	s.w++
	s.at, s.have = atMs, true
}

// ── The vertical scales ──────────────────────────────────────────────────
//
// THE THREE TRACES DO NOT SHARE AN AXIS. They are all fractions of one, which
// makes one axis look like the obvious answer and makes it the wrong one: RR
// lives in the bottom few percent (1.8% to 8.8% across the three sources) while
// DET is often above 0.9, so a shared 0..1 axis draws RR as a flat line on the
// floor. RR is the number ε is turned by and the first thing to move when
// something enters the signal; flattening it costs the chart most of its point.
//
// The three answers considered:
//
//   - A LOG AXIS. Rejected on both ends. RR = 0 is a real reading (silence with
//     a small ε) and has no place on a log axis at all, so it would need a
//     floor chosen out of nothing; and at the other end DET's interesting range
//     is 0.3 to 1.0, which log compresses into the top eighth of the pane —
//     the one part of the chart that must stay legible, since "DET fell off"
//     IS the transition.
//
//   - NORMALIZE EACH TRACE to its own running range. Rejected, and this repo
//     has already paid for the general form of the mistake twice: the Takens
//     mode auto-scaled its figure from the current audio level and the picture
//     zoomed with the music, and recurrence.go refuses to take ε from the
//     current window's spread for the same reason. Here it fails in the way
//     that matters most: a trace scaled to its own recent minimum and maximum
//     draws a dramatic step when the range rolls off the end of the window and
//     nothing happened, and draws almost nothing when a genuine step arrives
//     into a window whose range is already wide. A chart of transitions must
//     not have a scale that moves when a transition does.
//
//   - SEPARATE PANES, EACH ON ITS OWN FIXED SCALE. What this does. The traces
//     share the only axis they actually have in common — time — and stack
//     vertically, each with the full height of its pane and a mapping that
//     never changes. Fixed is the whole property: two readings at opposite ends
//     of the 41 seconds are at the same height if and only if they are the same
//     number, which is what makes the picture a measurement.
//
// DET and LAM are drawn linearly, because they already use their whole range:
// 0.29 to 0.99 across the three sources is most of the pane, and the absolute
// value is the meaningful thing (DET near 1 means deterministic, and where the
// trace sits says so at a glance).
//
// RR is drawn as √RR, which is not a cosmetic stretch. RR is the LIT FRACTION
// OF A SQUARE — an area — and its square root is the corresponding fraction of
// the square's SIDE, which is the length the eye is actually reading off the
// plot next to it. It expands exactly where RR lives, it maps 0 to the floor
// and 1 to the ceiling so it can never clip whatever ε is set to, and it is
// fixed. Measured: the 1.8% and 8.8% readings sit 0.07 of a pane apart drawn
// linearly and 0.16 apart under the root, and the 1–5% readable band goes from
// 4% of the pane to 12% of it.

// RQATraceY maps a reading onto its own pane: 0 is the bottom edge, 1 the top.
// Fixed per trace, never derived from the data — see the note above.
func RQATraceY(t RQATrace, v float64) float64 {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if t == RQATraceRR {
		return math.Sqrt(v)
	}
	return v
}
