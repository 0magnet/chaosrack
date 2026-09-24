package scope

import (
	"math"
	"testing"
)

func TestScopeTraceColsIsOnePerPixel(t *testing.T) {
	if got := TraceCols(480); got != 480 {
		t.Fatalf("a 480 px tube got %d columns, want 480", got)
	}
	// A tube that has not been laid out yet must still leave something to
	// join a line between.
	for _, w := range []float64{0, 1, -5} {
		if got := TraceCols(w); got < 2 {
			t.Fatalf("TraceCols(%v) = %d, want at least 2", w, got)
		}
	}
}

func TestEnvelopeKeepsTheSignalsExtremes(t *testing.T) {
	// The whole point: a waveform faster than the timebase must still show
	// its full height. Point-sampling it would alias it into a slow phantom
	// at some fraction of the amplitude.
	const n = 24000
	src := make([]float32, n)
	for i := range src {
		src[i] = float32(math.Sin(float64(i) * 0.9)) // ~7 samples a cycle
	}
	dst := make([]float32, 2*480)
	cols := TraceEnvelope(dst, src, 480)
	if cols != 480 {
		t.Fatalf("filled %d columns, want 480", cols)
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for c := range cols {
		if dst[c*2] > dst[c*2+1] {
			t.Fatalf("column %d has min %v above max %v", c, dst[c*2], dst[c*2+1])
		}
		if dst[c*2] < lo {
			lo = dst[c*2]
		}
		if dst[c*2+1] > hi {
			hi = dst[c*2+1]
		}
	}
	if lo > -0.98 || hi < 0.98 {
		t.Fatalf("envelope spans %v..%v, want the signal's full ±1 — the trace would read short", lo, hi)
	}
	// And every column must be a band, not a line: a signal cycling many
	// times inside one column has to fill it.
	flat := 0
	for c := range cols {
		if dst[c*2+1]-dst[c*2] < 1.5 {
			flat++
		}
	}
	if flat > cols/10 {
		t.Fatalf("%d of %d columns are thinner than the signal, want nearly none", flat, cols)
	}
}

func TestEnvelopeCoversEverySampleExactlyOnce(t *testing.T) {
	// No sample may fall between two columns: a gap is a piece of the
	// signal the tube never shows.
	src := make([]float32, 1000)
	for i := range src {
		src[i] = float32(i)
	}
	dst := make([]float32, 2*97)
	cols := TraceEnvelope(dst, src, 97)
	if cols != 97 {
		t.Fatalf("cols=%d want 97", cols)
	}
	if dst[0] != 0 {
		t.Fatalf("first column starts at %v, want sample 0", dst[0])
	}
	if got := dst[(cols-1)*2+1]; got != 999 {
		t.Fatalf("last column ends at %v, want sample 999", got)
	}
	for c := 1; c < cols; c++ {
		if dst[c*2] != dst[(c-1)*2+1]+1 {
			t.Fatalf("gap between column %d (ends %v) and %d (starts %v)",
				c-1, dst[(c-1)*2+1], c, dst[c*2])
		}
	}
}

func TestEnvelopeHandlesFewerSamplesThanColumns(t *testing.T) {
	src := []float32{1, 2, 3}
	dst := make([]float32, 2*480)
	if got := TraceEnvelope(dst, src, 480); got != 3 {
		t.Fatalf("3 samples into 480 columns filled %d, want 3 — never an empty column", got)
	}
}

func TestEnvelopeRefusesToOverrunItsBuffer(t *testing.T) {
	src := make([]float32, 5000)
	dst := make([]float32, 10) // room for five columns
	if got := TraceEnvelope(dst, src, 480); got != 5 {
		t.Fatalf("filled %d columns into a 5-column buffer, want 5", got)
	}
}

func TestEnvelopeIsEmptyWithNothingToDraw(t *testing.T) {
	dst := make([]float32, 64)
	if got := TraceEnvelope(dst, nil, 100); got != 0 {
		t.Fatalf("no samples gave %d columns, want 0", got)
	}
	if got := TraceEnvelope(nil, []float32{1, 2}, 100); got != 0 {
		t.Fatalf("no buffer gave %d columns, want 0", got)
	}
}
