package attractor

import "testing"

func TestASwitchPotSnapsAcrossItsGap(t *testing.T) {
	s := switchPot{off: -0.2}
	for _, c := range []struct{ in, want float64 }{
		{-0.2, -0.2}, {-0.15, -0.2}, {-0.1, -0.2}, // past half the gap: OFF
		{-0.09, 0}, {-0.01, 0}, // short of it: the bottom of the range
		{0, 0}, {0.37, 0.37}, {1, 1}, // the range turns smoothly
	} {
		if got := s.snap(c.in); got != c.want {
			t.Errorf("snap(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if !s.isOff(-0.2) || s.isOff(0) {
		t.Error("OFF is the detent and nothing in the range")
	}
}

func TestASwitchPotStepsInAndOutOfOff(t *testing.T) {
	s := switchPot{off: -0.2}
	if got := s.step(0, -0.01); got != -0.2 {
		t.Errorf("one click down from the bottom = %v, want OFF", got)
	}
	if got := s.step(-0.2, -0.19); got != 0 {
		t.Errorf("one click up from OFF = %v, want the bottom of the range", got)
	}
	if got := s.step(0.5, 0.49); got != 0.49 {
		t.Errorf("a click inside the range = %v, want 0.49", got)
	}
}
