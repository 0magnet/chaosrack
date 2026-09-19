package attractor

import (
	"math"
	"testing"
)

// The range switches are 1-2-5 because that is what makes a division worth a
// round number. If the sequence drifts, the graticule stops being countable
// and the whole reason for a detented switch is gone.
func TestTheRangeSwitchesStepOneTwoFive(t *testing.T) {
	got := scopeSteps125(0.01, 10)
	want := []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		// Exact after the snap: these numbers are printed on a panel, and
		// 0.05000000000000001 is not a thing to silkscreen.
		if got[i] != want[i] {
			t.Errorf("step %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// Ascending, with every step a real detent — no duplicates, and no step
// outside the range the panel claims.
func TestTheRangeSwitchesAreOrderedAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		steps  []float64
		lo, hi float64
	}{
		{"TIME/DIV", scopeTimebases, scopeSecPerDivMin, scopeSecPerDivMax},
		{"VOLTS/DIV", scopeVoltsDivs, scopeVoltsDivMin, scopeVoltsDivMax},
	} {
		if len(tc.steps) < 3 {
			t.Errorf("%s has %d detents, which is not a range switch", tc.name, len(tc.steps))
		}
		for i, s := range tc.steps {
			if s < tc.lo || s > tc.hi {
				t.Errorf("%s detent %d is %v, outside [%v,%v]", tc.name, i, s, tc.lo, tc.hi)
			}
			if i > 0 && s <= tc.steps[i-1] {
				t.Errorf("%s detent %d (%v) does not follow %v", tc.name, i, s, tc.steps[i-1])
			}
		}
		// Both ends of the advertised range are reachable, or the panel
		// claims a spec the knob cannot be set to.
		if tc.steps[0] != tc.lo {
			t.Errorf("%s starts at %v, want its stated minimum %v", tc.name, tc.steps[0], tc.lo)
		}
		if last := tc.steps[len(tc.steps)-1]; last != tc.hi {
			t.Errorf("%s ends at %v, want its stated maximum %v", tc.name, last, tc.hi)
		}
	}
}

// The knob has to mean what is written beside it: ten divisions of
// secPerDiv, at the sample rate, is what crosses the screen.
func TestTheTimebaseMeansSecondsPerDivision(t *testing.T) {
	const sr = 48000
	for _, sec := range []float64{1e-3, 5e-3, 0.1} {
		n := scopeSweepSamples(sec, sr)
		gotSec := float64(n) / sr / float64(gratDivX)
		if math.Abs(gotSec-sec)/sec > 0.001 {
			t.Errorf("%v s/div swept %d samples = %v s/div", sec, n, gotSec)
		}
	}
}

// A sweep faster than the converter can feed still has to draw something.
func TestAnImpossiblyFastSweepStillHasALineToDraw(t *testing.T) {
	if got := scopeSweepSamples(1e-9, 48000); got < 2 {
		t.Errorf("got %d samples, want at least the 2 a line needs", got)
	}
	if got := scopeSweepSamples(1e-3, 0); got < 2 {
		t.Errorf("no sample rate: got %d, want at least 2", got)
	}
}

// VOLTS/DIV means what it says, and does NOT clip: a trace that overruns the
// screen is how you learn the range is too fine, and folding it back would
// hide the one fault the knob exists to find.
func TestVoltsPerDivisionDeflectsAndDoesNotClip(t *testing.T) {
	// At 0.5/div, a full-scale sample is two divisions up.
	if got := scopeYDiv(1.0, 0.5, 0); math.Abs(got-2) > 1e-9 {
		t.Errorf("full scale at 0.5/div is %v divisions, want 2", got)
	}
	// At 0.1/div it is ten — off an eight-division screen, and that is right.
	if got := scopeYDiv(1.0, 0.1, 0); math.Abs(got-10) > 1e-9 {
		t.Errorf("full scale at 0.1/div is %v divisions, want 10 (off screen)", got)
	}
	if got := scopeYDiv(1.0, 0.1, 0); got <= float64(gratHalfH) {
		t.Error("an overdriven trace was clipped onto the screen")
	}
	// POSITION slides the whole trace without changing its size.
	a, b := scopeYDiv(0.25, 0.5, 0), scopeYDiv(0.25, 0.5, 1.5)
	if math.Abs((b-a)-1.5) > 1e-9 {
		t.Errorf("POSITION moved the trace by %v divisions, want 1.5", b-a)
	}
}

// The trigger is what makes a repeating waveform stand still. It fires on
// the EDGE and not on the level, or a signal sitting above the level would
// trigger on every sample of it.
func TestTheTriggerFiresOnTheEdgeAndNotTheLevel(t *testing.T) {
	// A square-ish wave: low, low, high, high, low, low, high...
	s := []float32{-1, -1, 1, 1, -1, -1, 1, 1}
	i := scopeTriggerIndex(s, 0, true, len(s))
	if i != 2 {
		t.Errorf("rising edge found at %d, want 2", i)
	}
	if j := scopeTriggerIndex(s, 0, false, len(s)); j != 4 {
		t.Errorf("falling edge found at %d, want 4", j)
	}
	// A signal entirely above the level has no rising crossing at all.
	high := []float32{0.5, 0.6, 0.7, 0.8}
	if k := scopeTriggerIndex(high, 0, true, len(high)); k != -1 {
		t.Errorf("a signal already above the level triggered at %d, want no crossing", k)
	}
	// Nor does a flat run exactly ON the level.
	flat := []float32{0, 0, 0, 0}
	if k := scopeTriggerIndex(flat, 0, true, len(flat)); k != -1 {
		t.Errorf("a flat run at the level triggered at %d, want no crossing", k)
	}
}

// Silence must not blank the tube: no crossing is reported as such, and the
// caller free-runs. A scope with no free-running sweep looks broken every
// time the trigger is misadjusted.
func TestSilenceReportsNoCrossingRatherThanAFalseOne(t *testing.T) {
	if got := scopeTriggerIndex(make([]float32, 512), 0.1, true, 512); got != -1 {
		t.Errorf("silence triggered at %d, want -1", got)
	}
}

// A knob restored from a saved value lands on the nearest DETENT, judged on
// the ratio scale the sequence actually is.
func TestARestoredKnobLandsOnTheNearestDetent(t *testing.T) {
	steps := []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1}
	for _, tc := range []struct {
		v    float64
		want int
	}{
		{0.01, 0}, {0.012, 0}, {0.018, 1}, {0.02, 1}, {0.9, 6}, {100, 6}, {0.0001, 0},
	} {
		if got := scopeNearestStep(steps, tc.v); got != tc.want {
			t.Errorf("%v landed on detent %d (%v), want %d (%v)",
				tc.v, got, steps[got], tc.want, steps[tc.want])
		}
	}
	if got := scopeNearestStep(nil, 1); got != 0 {
		t.Errorf("an empty switch returned %d, want 0", got)
	}
}

// What is silkscreened next to the knob. Never an exponent, never a trailing
// zero, and always the unit the number is in.
func TestTheKnobLegendsReadLikeAFrontPanel(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{scopeFormatTime(10e-6), "10 µs"},
		{scopeFormatTime(500e-6), "500 µs"},
		{scopeFormatTime(1e-3), "1 ms"},
		{scopeFormatTime(20e-3), "20 ms"},
		{scopeFormatTime(0.5), "500 ms"},
		{scopeFormatVolts(0.002), "2 mFS"},
		{scopeFormatVolts(0.05), "50 mFS"},
		{scopeFormatVolts(1), "1 FS"},
	} {
		if tc.in != tc.want {
			t.Errorf("legend is %q, want %q", tc.in, tc.want)
		}
	}
	// And every detent on both switches has a legend fit to print.
	for _, s := range scopeTimebases {
		if got := scopeFormatTime(s); got == "" || len(got) > 8 {
			t.Errorf("%v prints as %q, which does not fit a panel", s, got)
		}
	}
	for _, s := range scopeVoltsDivs {
		if got := scopeFormatVolts(s); got == "" || len(got) > 9 {
			t.Errorf("%v prints as %q, which does not fit a panel", s, got)
		}
	}
}
