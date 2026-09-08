package attractor

import (
	"math"
	"testing"
)

// The envelope-to-parameter mapping is the part of the audio drive that can be
// got wrong silently, and it is deliberately free of GL, the DOM and the audio
// stack so it can be checked here.

func TestBifAudioSpanCentersOnTheKnob(t *testing.T) {
	// Knob in the middle, a third of the range: the window straddles it.
	lo, span := bifAudioSpan(0.3, 15, 0, 30)
	if math.Abs(float64(span-9)) > 1e-5 {
		t.Errorf("span = %v, want 9 (0.3 of a range of 30)", span)
	}
	if math.Abs(float64(lo-10.5)) > 1e-5 {
		t.Errorf("lo = %v, want 10.5 (centered on 15)", lo)
	}
}

// The window slides in rather than the value being clamped — with the knob at
// an end, the cursor must still use the whole envelope instead of sitting
// pinned for half of it.
func TestBifAudioSpanShiftsInsteadOfClamping(t *testing.T) {
	for _, tc := range []struct {
		name           string
		center         float32
		wantLo, wantHi float32
	}{
		{"at the low end", 0, 0, 9},
		{"below the low end", -100, 0, 9},
		{"at the high end", 30, 21, 30},
		{"above the high end", 999, 21, 30},
	} {
		lo, span := bifAudioSpan(0.3, tc.center, 0, 30)
		if math.Abs(float64(lo-tc.wantLo)) > 1e-5 || math.Abs(float64(lo+span-tc.wantHi)) > 1e-5 {
			t.Errorf("%s: window [%v,%v], want [%v,%v]", tc.name, lo, lo+span, tc.wantLo, tc.wantHi)
		}
		// The full envelope must still cover the full window.
		if got := bifAudioValue(0, 0.3, tc.center, 0, 30); math.Abs(float64(got-tc.wantLo)) > 1e-5 {
			t.Errorf("%s: silence maps to %v, want %v", tc.name, got, tc.wantLo)
		}
		if got := bifAudioValue(1, 0.3, tc.center, 0, 30); math.Abs(float64(got-tc.wantHi)) > 1e-5 {
			t.Errorf("%s: full scale maps to %v, want %v", tc.name, got, tc.wantHi)
		}
	}
}

// Depth 1 is the whole axis; depth 0 pins the cursor to the knob, which is the
// honest reading of "no depth" and not a degenerate case to guard against.
func TestBifAudioSpanDepthExtremes(t *testing.T) {
	lo, span := bifAudioSpan(1, 15, 0, 30)
	if lo != 0 || math.Abs(float64(span-30)) > 1e-5 {
		t.Errorf("depth 1 gave [%v,%v], want the whole axis", lo, lo+span)
	}
	for _, env := range []float32{0, 0.5, 1} {
		if got := bifAudioValue(env, 0, 12, 0, 30); math.Abs(float64(got-12)) > 1e-5 {
			t.Errorf("depth 0, env %v gave %v, want the knob value 12", env, got)
		}
	}
}

// The envelope is nominally 0..1 but is smoothed and adaptively normalized
// upstream, so a value a hair outside must not walk the cursor off the axis.
func TestBifAudioValueClampsTheEnvelope(t *testing.T) {
	lo, span := bifAudioSpan(0.5, 15, 0, 30)
	if got := bifAudioValue(-0.2, 0.5, 15, 0, 30); got != lo {
		t.Errorf("env -0.2 gave %v, want the window's low end %v", got, lo)
	}
	if got := bifAudioValue(1.4, 0.5, 15, 0, 30); got != lo+span {
		t.Errorf("env 1.4 gave %v, want the window's high end %v", got, lo+span)
	}
}

// A parameter with a zero-width range (a constant, or a knob whose min and max
// were set equal) must not produce a NaN position.
func TestBifAudioDegenerateRange(t *testing.T) {
	v := bifAudioValue(0.7, 0.5, 5, 5, 5)
	if math.IsNaN(float64(v)) || v != 5 {
		t.Errorf("degenerate range gave %v, want 5", v)
	}
	if f := bifFrac(5, 5, 5); f != 0 {
		t.Errorf("bifFrac on a degenerate range gave %v, want 0", f)
	}
	if c := bifColumnFor(5, 5, 5, bifCols); c != 0 {
		t.Errorf("bifColumnFor on a degenerate range gave %v, want 0", c)
	}
}

// The cursor has to land on the column the sweep actually computed for that
// value, or it would highlight a slice of the attractor taken at a different
// parameter than the one it is reporting.
func TestBifColumnForMatchesTheSweep(t *testing.T) {
	const min, max float32 = 0.1, 1.45
	for _, j := range []int{0, 1, 17, bifCols / 2, bifCols - 2, bifCols - 1} {
		// The value the sweep uses for column j, verbatim from
		// generateBifurcation.
		pv := min + (max-min)*float32(j)/float32(bifCols-1)
		if got := bifColumnFor(pv, min, max, bifCols); got != j {
			t.Errorf("column %d sweeps to %v, which maps back to column %d", j, pv, got)
		}
	}
}

func TestBifColumnForStaysInRange(t *testing.T) {
	for _, v := range []float32{-1e6, 0, 0.5, 1, 1e6} {
		c := bifColumnFor(v, 0, 1, bifCols)
		if c < 0 || c > bifCols-1 {
			t.Errorf("value %v gave column %d, outside 0..%d", v, c, bifCols-1)
		}
	}
	if c := bifColumnFor(0.5, 0, 1, 1); c != 0 {
		t.Errorf("a one-column diagram gave column %d, want 0", c)
	}
}

// Monotonic in the envelope: louder must never move the cursor backwards, or
// the readout and the highlight would disagree with what is being heard.
func TestBifAudioValueIsMonotonic(t *testing.T) {
	prev := float32(math.Inf(-1))
	for i := 0; i <= 100; i++ {
		v := bifAudioValue(float32(i)/100, 0.4, 20, 0, 60)
		if v < prev {
			t.Fatalf("env %v gave %v, below the previous %v", float32(i)/100, v, prev)
		}
		prev = v
	}
}
