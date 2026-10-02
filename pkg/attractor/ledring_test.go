package attractor

import "testing"

func TestTheRingFillsFromTheStartOrFromZero(t *testing.T) {
	for _, c := range []struct {
		v, lo, hi float64
		from, to  int
	}{
		{0, 0, 90, 0, 0},   // at the bottom: the first LED
		{90, 0, 90, 0, 20}, // at the top: all of them
		{45, 0, 90, 0, 10}, // halfway
		{0, -8, 8, 10, 10}, // signed at zero: the middle one
		{8, -8, 8, 10, 20}, // up from the middle
		{-8, -8, 8, 0, 10}, // and down from it
		{99, 0, 90, 0, 20}, // clamped
	} {
		from, to := ringLit(c.v, c.lo, c.hi, 21)
		if from != c.from || to != c.to {
			t.Errorf("ringLit(%v in %v..%v) = %d..%d, want %d..%d", c.v, c.lo, c.hi, from, to, c.from, c.to)
		}
	}
}

func TestASelectorLightsOneLEDPerPosition(t *testing.T) {
	if ringDot(0, 3, 21) != 0 || ringDot(1, 3, 21) != 10 || ringDot(2, 3, 21) != 20 {
		t.Error("three positions do not land at the start, middle and end")
	}
	if ringDot(5, 3, 21) != 20 {
		t.Error("a position past the last is not clamped to it")
	}
}
