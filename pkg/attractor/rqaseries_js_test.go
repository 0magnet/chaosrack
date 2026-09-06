//go:build js && wasm

package attractor

import (
	"strconv"
	"strings"
	"testing"
)

// The half of the strip chart that only exists in the js build context: the
// geometry it is drawn at, the fingerprint that decides when the series takes a
// seam, and the path builder. Node has no DOM, so nothing here touches a canvas
// — what is checked is the arithmetic that decides what would be drawn on one.

// One slot per column is the whole reason the ring is the length it is. Sizing
// them apart is silent either way: a longer ring is history that never reaches
// the chart, a shorter one leaves columns that can never be filled.
func TestTheRingIsExactlyTheChartsWidth(t *testing.T) {
	if RQASeriesLen != rqaChartCols {
		t.Errorf("the ring holds %d slots for a %d-column chart", RQASeriesLen, rqaChartCols)
	}
	if rqaChartH != rqaPaneH*int(RQATraceCount) {
		t.Errorf("the canvas is %d px tall for %d panes of %d", rqaChartH, int(RQATraceCount), rqaPaneH)
	}
	// Every trace needs a color; a missing one is the empty string, which the
	// canvas ignores, and the trace would be drawn in whatever the pane before
	// it was using.
	for tr := RQATrace(0); tr < RQATraceCount; tr++ {
		if rqaTraceColor[tr] == "" {
			t.Errorf("%v has no color", tr)
		}
	}
}

// Every pane must map its whole range inside its own band of the canvas, or one
// trace draws over another and the chart reads as a single tangled plot.
func TestEachPaneStaysInsideItsOwnBandOfTheCanvas(t *testing.T) {
	for tr := RQATrace(0); tr < RQATraceCount; tr++ {
		top := int(tr) * rqaPaneH
		for _, f := range []float64{0, 0.5, 1} {
			y := rqaPaneY(top, f)
			if y < float64(top) || y > float64(top+rqaPaneH) {
				t.Errorf("%v at %v draws at y=%v, outside its pane %d..%d", tr, f, y, top, top+rqaPaneH)
			}
		}
		// Top and bottom are inset, so a reading pinned at either end — DET at
		// 1.00 on a clean orbit is the common one — is a line inside the pane
		// rather than half of one on the border between two.
		if rqaPaneY(top, 1) <= float64(top) {
			t.Errorf("%v draws a full reading on its top border", tr)
		}
		if rqaPaneY(top, 0) >= float64(top+rqaPaneH) {
			t.Errorf("%v draws a zero reading on its bottom border", tr)
		}
		// Higher is up. Getting this the wrong way round would draw a perfectly
		// plausible chart of the wrong thing.
		if rqaPaneY(top, 1) >= rqaPaneY(top, 0) {
			t.Errorf("%v draws 1 below 0", tr)
		}
	}
}

// The path is broken across the slots with no measurement in them: a hole in
// the record has to be a hole in the line, not a straight segment drawn through
// a stretch nobody looked at.
func TestThePathBreaksAcrossAGapRatherThanBridgingIt(t *testing.T) {
	saved := rqaSnap
	defer func() { rqaSnap = saved }()

	rqaSnap = make([]RQASample, rqaChartCols)
	for i := range rqaSnap {
		rqaSnap[i] = RQASample{RR: 0.04, DET: 0.9, LAM: 0.5, OK: true}
	}
	full := rqaTracePath(RQATraceDET, 0)
	if strings.Count(full, "M") != 1 {
		t.Errorf("an unbroken series drew %d subpaths, want 1", strings.Count(full, "M"))
	}

	// Punch a hole in the middle.
	for i := 100; i < 110; i++ {
		rqaSnap[i] = RQASample{}
	}
	broken := rqaTracePath(RQATraceDET, 0)
	if got := strings.Count(broken, "M"); got != 2 {
		t.Errorf("a series with one hole drew %d subpaths, want 2", got)
	}
	if len(broken) >= len(full) {
		t.Error("the broken path is no shorter than the whole one; the gap was drawn through")
	}

	// An all-gap window is nothing at all rather than a path with no points in
	// it, which Path2D would accept and stroke as a stray mark at the origin.
	for i := range rqaSnap {
		rqaSnap[i] = RQASample{}
	}
	if d := rqaTracePath(RQATraceRR, 0); d != "" {
		t.Errorf("an empty series drew %q", d)
	}
}

// A lone reading between two gaps — one measurement either side of a stall — is
// real data, and a butt-capped zero-length segment renders nothing. It is
// opened as a degenerate segment so the round cap draws it as a dot.
func TestAnIsolatedReadingIsStillDrawn(t *testing.T) {
	saved := rqaSnap
	defer func() { rqaSnap = saved }()

	rqaSnap = make([]RQASample, rqaChartCols)
	rqaSnap[7] = RQASample{RR: 0.04, DET: 0.9, LAM: 0.5, OK: true}
	d := rqaTracePath(RQATraceLAM, 0)
	if !strings.Contains(d, "M") || !strings.Contains(d, "L") {
		t.Errorf("a single reading drew %q; it needs a segment to have a cap to draw", d)
	}
}

// Time runs left to right with the newest reading against the right edge. The
// x of column i is i + a half — the middle of the pixel — so a one-column trace
// is not drawn half off the canvas.
func TestTheNewestColumnIsAtTheRightEdge(t *testing.T) {
	saved := rqaSnap
	defer func() { rqaSnap = saved }()

	rqaSnap = make([]RQASample, rqaChartCols)
	rqaSnap[0] = RQASample{DET: 0.1, OK: true}
	rqaSnap[rqaChartCols-1] = RQASample{DET: 0.9, OK: true}
	d := rqaTracePath(RQATraceDET, 0)
	if !strings.HasPrefix(d, "M0.5 ") {
		t.Errorf("the oldest column starts at %.20q, want the middle of pixel 0", d)
	}
	newest := "M" + strconv.Itoa(rqaChartCols-1) + ".5"
	if !strings.Contains(d, newest) {
		t.Errorf("the newest column is not at x=%s; the chart is drawn backwards or offset", newest[1:])
	}
}

// The fingerprint is what puts a seam in the chart when the MEASUREMENT
// changes, and it has to notice everything that changes the answer and nothing
// that does not. τ and m are the second half of that: they define the
// embedding, and only the embed source has one — turning τ while watching raw
// audio must not seam a trace that did not move.
func TestTheSeamFingerprintTracksWhatChangesTheAnswer(t *testing.T) {
	savedSrc, savedWin, savedEps := rpSrc, rpWin, rpEps
	savedDim, savedTau, savedGen := rpDim, takensTau, rpTrajGen
	defer func() {
		rpSrc, rpWin, rpEps = savedSrc, savedWin, savedEps
		rpDim, takensTau, rpTrajGen = savedDim, savedTau, savedGen
	}()

	rpSrc, rpWin, rpEps, rpDim, takensTau = rpSrcAudio, 100, 0.05, 3, 32
	base := rqaConfigNow()
	if rqaConfigNow() != base {
		t.Fatal("the fingerprint changes with nothing moving; every tick would be a seam")
	}
	for _, c := range []struct {
		name string
		move func()
	}{
		{"ε", func() { rpEps = 0.08 }},
		{"win", func() { rpWin = 500 }},
		{"src", func() { rpSrc = rpSrcTraj }},
		{"a re-integrated trajectory", func() { rpTrajGen++ }},
	} {
		rpSrc, rpWin, rpEps, rpDim, takensTau = rpSrcAudio, 100, 0.05, 3, 32
		before := rqaConfigNow()
		c.move()
		if rqaConfigNow() == before {
			t.Errorf("%s moved and the chart would splice straight across it", c.name)
		}
	}

	// τ and m only mean anything for the embed source.
	rpSrc, rpWin, rpEps, rpDim, takensTau = rpSrcAudio, 100, 0.05, 3, 32
	before := rqaConfigNow()
	takensTau, rpDim = 64, 6
	if rqaConfigNow() != before {
		t.Error("τ or m seamed the raw-audio trace, which is m=1 whatever those knobs say")
	}
	rpSrc = rpSrcEmbed
	before = rqaConfigNow()
	takensTau = 128
	if rqaConfigNow() == before {
		t.Error("τ moved under the embed source and the chart would splice across it")
	}
}
