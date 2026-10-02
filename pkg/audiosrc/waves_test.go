package audiosrc

import (
	"math"
	"testing"
)

// Exactly the four stimuli are waves, each a different one: a wave that
// played no stimulus would be silence on the speakers, and two playing the
// same one a position that does nothing.
func TestFourWavesAreStimuli(t *testing.T) {
	seen := map[TestSignal]int{}
	for w := range WaveCount {
		sig, ok := WaveStim(w)
		if ok != (w >= WaveWhite) {
			t.Errorf("wave %d: stimulus %v, want %v", w, ok, w >= WaveWhite)
		}
		if !ok {
			continue
		}
		if prev, dup := seen[sig]; dup {
			t.Errorf("waves %d and %d both play %s", prev, w, TestSignalNames[sig])
		}
		seen[sig] = w
	}
	if len(seen) != 4 {
		t.Errorf("%d stimulus waves, want 4", len(seen))
	}
}

// A loop is one whole period of what the rack's signal plays, inside full
// scale and not silent, so the speakers play what the scope draws.
func TestStimLoopsAreWholePeriods(t *testing.T) {
	const sr = 48000
	want := map[int]int{
		WaveWhite: sr * stimNoiseSeconds,
		WavePink:  sr * stimNoiseSeconds,
		WaveSweep: sr * sweepSeconds,
		WavePulse: sr / polarityHz,
	}
	for w, n := range want {
		pts := StimLoop(w, sr)
		if len(pts) != n {
			t.Errorf("wave %d: %d samples, want %d", w, len(pts), n)
		}
		var peak, sum float64
		for _, v := range pts {
			peak = math.Max(peak, math.Abs(float64(v)))
			sum += float64(v)
		}
		if peak > 1 || peak < 0.05 {
			t.Errorf("wave %d peaks at %.3f", w, peak)
		}
		if m := sum / float64(len(pts)); math.Abs(m) > 0.02 {
			t.Errorf("wave %d has a mean of %.4f, a DC offset", w, m)
		}
	}
	if StimLoop(WaveSine, sr) != nil {
		t.Error("a loop for the sine, which is not a stimulus")
	}
}

// The envelope rises over the attack, falls over the decay, and repeats.
func TestEnvRisesFallsAndRepeats(t *testing.T) {
	const atk, dcy = 0.01, 0.3
	for _, c := range []struct{ t, want float64 }{
		{0, 0}, {0.005, 0.5}, {0.01, 1}, {0.16, 0.5}, {0.31, 0}, {0.315, 0.5},
	} {
		if got := EnvAt(c.t, atk, dcy); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("at %.3fs: %.4f, want %.4f", c.t, got, c.want)
		}
	}
}
