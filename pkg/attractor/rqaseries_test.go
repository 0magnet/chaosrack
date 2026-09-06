package attractor

import (
	"math"
	"testing"
)

// The series, tested without a DOM — the reason the ring and the scales live in
// an untagged file. What is checked here is not that the code runs but the
// handful of properties that make the chart a MEASUREMENT rather than a
// decoration: it is in time order across the wrap, an unmeasured stretch comes
// out as a hole rather than as a straight line through it, jitter does not
// become a hole, and each trace's mapping is fixed, monotone and readable.

// A helper that reads like the js side's call: one measurement.
func rqaAt(rr, det, lam float64) RQAResult {
	return RQAResult{RR: rr, DET: det, LAM: lam, Lit: 1}
}

// The ring wraps, and a chart drawn backwards or rotated by the wrap point is
// not obviously wrong to the eye and completely wrong as a reading. Pushed
// three times the ring's length, the snapshot must be the last RQASeriesLen
// values in the order they happened, newest in the last column.
func TestTheSeriesIsInTimeOrderAcrossTheWrap(t *testing.T) {
	var s RQASeries
	const n = 3 * RQASeriesLen
	for i := 0; i < n; i++ {
		// A ramp, so any rotation or reversal shows up as a value out of place
		// rather than as a plausible-looking wiggle.
		v := float64(i) / float64(n)
		s.Push(float64(i)*RQASamplePeriodMs, rqaAt(v, 1-v, v/2))
	}
	dst := make([]RQASample, RQASeriesLen)
	if got := s.Snapshot(dst); got != RQASeriesLen {
		t.Fatalf("snapshot reports %d real slots, want a full ring of %d", got, RQASeriesLen)
	}
	for i, v := range dst {
		want := float64(n-RQASeriesLen+i) / float64(n)
		if !v.OK {
			t.Fatalf("column %d is a gap in a series pushed at exactly the sample period", i)
		}
		if math.Abs(float64(v.RR)-want) > 1e-6 {
			t.Fatalf("column %d holds RR %v, want %v — the ring is rotated or reversed", i, v.RR, want)
		}
		if math.Abs(float64(v.DET)-(1-want)) > 1e-6 {
			t.Errorf("column %d holds DET %v, want %v", i, v.DET, 1-want)
		}
	}
}

// A half-filled series must leave the LEFT of the chart empty and keep the
// newest reading against the right edge — the newest is the one being watched,
// and it must not walk across the chart as the buffer fills.
func TestAPartlyFilledSeriesIsRightAligned(t *testing.T) {
	var s RQASeries
	const have = 10
	for i := 0; i < have; i++ {
		s.Push(float64(i)*RQASamplePeriodMs, rqaAt(0.5, 0.5, 0.5))
	}
	dst := make([]RQASample, RQASeriesLen)
	if got := s.Snapshot(dst); got != have {
		t.Fatalf("snapshot reports %d real slots after %d pushes", got, have)
	}
	for i := 0; i < RQASeriesLen-have; i++ {
		if dst[i].OK {
			t.Fatalf("column %d is a reading; the unfilled part of the chart must be empty", i)
		}
	}
	for i := RQASeriesLen - have; i < RQASeriesLen; i++ {
		if !dst[i].OK {
			t.Fatalf("column %d is empty; the newest %d readings must sit against the right edge", i, have)
		}
	}
}

// THE TEST THE GAP MACHINERY EXISTS FOR. Background a tab and rAF stops; come
// back and a chart that simply appends joins the two sides of the hole with a
// straight line through a period nobody measured — indistinguishable from a
// slow real drift, which is the exact reading this display is for.
func TestAStallBecomesAHoleRatherThanASplice(t *testing.T) {
	var s RQASeries
	s.Push(0, rqaAt(0.02, 0.9, 0.5))
	// Ten intervals go by with no measurement, then one arrives.
	s.Push(10*RQASamplePeriodMs, rqaAt(0.02, 0.2, 0.5))

	dst := make([]RQASample, RQASeriesLen)
	s.Snapshot(dst)
	// The newest slot is the second reading, the nine before it are the hole,
	// and the one before those is the first reading.
	if !dst[RQASeriesLen-1].OK {
		t.Fatal("the reading that ended the stall is not in the chart")
	}
	for i := 1; i <= 9; i++ {
		if dst[RQASeriesLen-1-i].OK {
			t.Fatalf("column %d back from the newest is a reading; the stall must be empty", i)
		}
	}
	if !dst[RQASeriesLen-11].OK {
		t.Error("the reading before the stall was dropped; the history either side of a hole still stands")
	}
}

// The other half of the same decision: frame jitter is not a hole. The tick
// fires on the first frame at or after its due time, so a nominal 160 ms
// arrives as 160–176 ms, and a slow frame — a 24 ms Chen integration next to a
// GC pause — can push it past 250. A chart stippled with one-slot breaks every
// time the browser hiccuped would be unreadable for no gain.
func TestFrameJitterDoesNotPunchHolesInTheChart(t *testing.T) {
	var s RQASeries
	now := 0.0
	for i, dt := range []float64{160, 176, 168, 250, 160, 310, 176} {
		now += dt
		s.Push(now, rqaAt(0.02, 0.9, 0.5))
		dst := make([]RQASample, RQASeriesLen)
		n := s.Snapshot(dst)
		if want := i + 1; n != want {
			t.Fatalf("after a %v ms frame the series holds %d slots, want %d — jitter under two "+
				"intervals must not insert a gap", dt, n, want)
		}
	}
}

// A stall longer than the whole ring leaves nothing behind it, and must not
// spend a loop proving that by writing thousands of empty slots.
func TestAStallLongerThanTheRingEmptiesIt(t *testing.T) {
	var s RQASeries
	for i := 0; i < RQASeriesLen; i++ {
		s.Push(float64(i)*RQASamplePeriodMs, rqaAt(0.02, 0.9, 0.5))
	}
	before := s.w
	s.Push(1e9, rqaAt(0.02, 0.9, 0.5)) // an hour later

	if wrote := s.w - before; wrote > RQASeriesLen+1 {
		t.Errorf("a stall of an hour wrote %d slots into a %d-slot ring", wrote, RQASeriesLen)
	}
	dst := make([]RQASample, RQASeriesLen)
	if n := s.Snapshot(dst); n != RQASeriesLen {
		t.Fatalf("snapshot reports %d slots", n)
	}
	for i := 0; i < RQASeriesLen-1; i++ {
		if dst[i].OK {
			t.Fatalf("column %d survived a stall longer than the ring", i)
		}
	}
	if !dst[RQASeriesLen-1].OK {
		t.Error("the reading that ended the stall is not on the chart")
	}
}

// A change of ε, source, m or τ is a change of QUESTION, and the answers either
// side of it are not comparable. It is one slot wide — a seam, not a reset:
// the history before it is still true of what it was, and one of the things
// the chart is good for is watching what your own ε change did.
func TestAMeasurementChangeIsASeamAndNotAReset(t *testing.T) {
	var s RQASeries
	for i := 0; i < 20; i++ {
		s.Push(float64(i)*RQASamplePeriodMs, rqaAt(0.02, 0.9, 0.5))
	}
	s.Break(20 * RQASamplePeriodMs)
	for i := 21; i < 25; i++ {
		s.Push(float64(i)*RQASamplePeriodMs, rqaAt(0.08, 0.4, 0.5))
	}
	dst := make([]RQASample, RQASeriesLen)
	if n := s.Snapshot(dst); n != 25 {
		t.Fatalf("the series holds %d slots after 24 pushes and one break, want 25", n)
	}
	// Counted from the newest: 4 readings at the new setting, one seam, 20 at
	// the old one — all of them still there.
	last := RQASeriesLen - 1
	for i := 0; i < 4; i++ {
		if !dst[last-i].OK {
			t.Fatalf("column %d back is empty; only the seam itself is", i)
		}
	}
	if dst[last-4].OK {
		t.Error("no seam was drawn where the measurement changed")
	}
	for i := 5; i < 25; i++ {
		if !dst[last-i].OK {
			t.Fatalf("column %d back is empty; a seam must not clear the history behind it", i)
		}
	}
}

// Before there is enough audio to fill the window the matrix has nothing lit in
// it, and that is not a reading of zero. A chart that drew it as one would show
// determinism collapsing every time the mode was entered.
func TestAnUnlitMatrixIsAGapAndNotThreeZeros(t *testing.T) {
	var s RQASeries
	s.Push(0, RQAResult{}) // Lit == 0: no measurement at all
	dst := make([]RQASample, RQASeriesLen)
	if n := s.Snapshot(dst); n != 1 {
		t.Fatalf("the series holds %d slots after one push", n)
	}
	if dst[RQASeriesLen-1].OK {
		t.Error("a matrix with nothing lit was recorded as a reading of 0/0/0")
	}
}

// THE SCALING DECISION, PINNED. The three traces do not share an axis: RR lives
// in the bottom few percent while DET is often above 0.9, so one 0..1 axis
// draws RR flat on the floor. Each pane's mapping must be fixed (the same
// reading is at the same height whenever it arrives), monotone, and must not
// clip at either end whatever ε is set to.
func TestEachTraceHasItsOwnFixedUnclippedScale(t *testing.T) {
	for _, tr := range []RQATrace{RQATraceRR, RQATraceDET, RQATraceLAM} {
		if got := RQATraceY(tr, 0); got != 0 {
			t.Errorf("%v maps 0 to %v, not to the bottom of its pane", tr, got)
		}
		if got := RQATraceY(tr, 1); got != 1 {
			t.Errorf("%v maps 1 to %v, not to the top of its pane", tr, got)
		}
		prev := -1.0
		for v := 0.0; v <= 1.0001; v += 0.01 {
			y := RQATraceY(tr, v)
			if y < prev {
				t.Fatalf("%v is not monotone: %v mapped below the value before it", tr, v)
			}
			if y < 0 || y > 1 {
				t.Fatalf("%v maps %v to %v, outside its pane", tr, v, y)
			}
			prev = y
		}
		// Out-of-range readings are clamped rather than drawn off the pane. RR
		// cannot exceed 1 by definition, but a clamp is what stops an arithmetic
		// slip somewhere upstream from drawing a line through the pane above.
		if RQATraceY(tr, -1) != 0 || RQATraceY(tr, 2) != 1 {
			t.Errorf("%v does not clamp out-of-range readings to its pane", tr)
		}
	}
}

// RR is drawn as √RR because RR is the LIT FRACTION OF A SQUARE — an area —
// and its root is the corresponding fraction of the square's side, which is
// the length the eye reads off the plot next door. What that buys, against the
// linear axis it replaced, is that the three sources' measured rates are
// visibly different heights instead of three lines on the floor.
func TestTheRootScaleSeparatesTheMeasuredRecurrenceRates(t *testing.T) {
	// Measured live at the default ε: raw audio, its m=3 delay embedding, and a
	// Lorenz trajectory.
	const audio, embed, traj = 0.088, 0.018, 0.025
	sep := func(a, b float64) float64 {
		return math.Abs(RQATraceY(RQATraceRR, a) - RQATraceY(RQATraceRR, b))
	}
	if got := sep(audio, embed); got < 2*math.Abs(audio-embed) {
		t.Errorf("audio and embed sit %.3f of a pane apart, no better than the %.3f a linear "+
			"axis gives; the root scale is not earning its place", got, math.Abs(audio-embed))
	}
	if got := sep(embed, traj); got < 0.02 {
		t.Errorf("embed and traj sit %.4f of a pane apart — under a pixel on a 152 px pane", got)
	}
	// And the band the plot is readable in has to be visible as a band rather
	// than as a line on the floor: 1–5% is 4% of the pane drawn linearly.
	band := RQATraceY(RQATraceRR, RQAReadableHi) - RQATraceY(RQATraceRR, RQAReadableLo)
	if band < 0.10 {
		t.Errorf("the readable band is %.3f of the pane; it is shaded behind the trace and has "+
			"to be thick enough to aim ε at", band)
	}
}

// DET and LAM are linear on purpose: they already use their whole range (0.29
// to 0.99 across the three sources), and where the trace sits IS the reading —
// near the top means deterministic. A root or a log there would move the one
// part of the chart that has to stay legible.
func TestDeterminismAndLaminarityAreDrawnAsThemselves(t *testing.T) {
	for _, tr := range []RQATrace{RQATraceDET, RQATraceLAM} {
		for _, v := range []float64{0.29, 0.36, 0.72, 0.88, 0.99} {
			if got := RQATraceY(tr, v); math.Abs(got-v) > 1e-12 {
				t.Errorf("%v maps %v to %v; it must be drawn as itself", tr, v, got)
			}
		}
	}
}

// The bound, in the units a reader of the chart cares about. Stated as a test
// because RQASeriesLen and RQASamplePeriodMs are both tunable and either one
// moves the scrollback without saying so.
func TestTheHistoryIsAboutFortySecondsOfWallClock(t *testing.T) {
	secs := float64(RQASeriesSpanMs) / 1000
	if secs < 30 || secs > 60 {
		t.Errorf("the chart holds %.1f s of history; the comment and the tooltip both say ~41 s", secs)
	}
	// The other half of the bound — one slot per column of the chart — is
	// pinned in the js build context, where the chart's geometry lives.
}

// Each trace has to say what it is on the chart, and the strings are what the
// panes are labeled with.
func TestEveryTraceIsLabeled(t *testing.T) {
	seen := map[string]bool{}
	for tr := RQATrace(0); tr < RQATraceCount; tr++ {
		s := tr.String()
		if s == "" {
			t.Errorf("trace %d has no label", tr)
		}
		if seen[s] {
			t.Errorf("two traces are both labeled %q", s)
		}
		seen[s] = true
	}
}

// The samples the ring stores must be the numbers RQA produced, addressed by
// trace — the painter walks the window once per trace and reads them this way.
func TestASampleReportsEachTraceSeparately(t *testing.T) {
	var s RQASeries
	s.Push(0, rqaAt(0.088, 0.29, 0.36))
	dst := make([]RQASample, 1)
	s.Snapshot(dst)
	for _, c := range []struct {
		tr   RQATrace
		want float64
	}{{RQATraceRR, 0.088}, {RQATraceDET, 0.29}, {RQATraceLAM, 0.36}} {
		if got := dst[0].Value(c.tr); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%v reads %v, want %v", c.tr, got, c.want)
		}
	}
}
