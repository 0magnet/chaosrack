package attractor

import "testing"

func TestFrameScaleIsOneAtSixtyHertz(t *testing.T) {
	if got := frameScale(1000.0 / 60.0); got < 0.999 || got > 1.001 {
		t.Fatalf("60 Hz frame scaled by %v, want 1 — the rates were dialed in against it", got)
	}
}

func TestFrameScaleTracksElapsedTime(t *testing.T) {
	// The property that matters: angle advanced per unit of TIME is the
	// same however the time is cut into frames. One 33.3 ms frame must
	// move the model as far as two 16.7 ms frames.
	one := frameScale(1000.0 / 30.0)
	two := 2 * frameScale(1000.0/60.0)
	if d := one - two; d > 0.002 || d < -0.002 {
		t.Fatalf("one 30 Hz frame = %v, two 60 Hz frames = %v: a dropped frame changes the speed", one, two)
	}
}

func TestFrameScaleClampsAResumeGap(t *testing.T) {
	// A tab returning from the background reports the whole absence.
	if got := frameScale(30000); got != 6 {
		t.Fatalf("a 30 s gap scaled by %v, want the 6-frame clamp — the model must not teleport", got)
	}
}

func TestFrameScaleSurvivesAZeroOrBackwardDelta(t *testing.T) {
	for _, d := range []float32{0, -1, -1e9} {
		if got := frameScale(d); got != 1 {
			t.Fatalf("frameScale(%v) = %v, want 1: a non-advancing clock must not stop or reverse the spin", d, got)
		}
	}
}
