package attractor

import (
	"math"
	"testing"
)

func TestParamTurnsKinds(t *testing.T) {
	for _, c := range []struct {
		id, label   string
		lo, hi, stp float64
		named       bool
		want        turnKind
	}{
		{"lorenz-sigma", "σ", 0, 50, 0.1, false, turnLinear},
		{"lorenz-dt", "dt", 0.0001, 0.02, 0.0001, false, turnLog},
		{"globe-lat", "lat", 0, 40, 1, false, turnBounded},
		{"x-mode", "mode", 0, 3, 0.1, true, turnBounded},
		{"hue-shift", "hue", 0, 360, 1, false, turnWrap}, // whole degrees, still a circle
		{"torus-twist", "twist", 0, 6.2831853, 0.01, false, turnWrap},
		{"view-rot", "rot", -180, 180, 0.5, false, turnWrap},
		{"custom-extent", "size", 0.2, 10, 0.1, false, turnLog},
	} {
		if got := paramTurns(c.id, c.label, c.lo, c.hi, c.stp, c.named); got != c.want {
			t.Errorf("%s: kind %d, want %d", c.id, got, c.want)
		}
	}
}

func TestLinearLapsContinueTheRange(t *testing.T) {
	s := turnSpec{turnLinear, 0, 50}
	for _, c := range []struct {
		v    float64
		lap  int
		frac float64
	}{{0, 0, 0}, {25, 0, 0.5}, {50, 0, 1}, {75, 1, 0.5}, {100, 1, 1}, {-25, -1, 0.5}} {
		lap, f := s.lap(c.v)
		if lap != c.lap || math.Abs(f-c.frac) > 1e-9 {
			t.Errorf("lap(%v) = %d, %v; want %d, %v", c.v, lap, f, c.lap, c.frac)
		}
	}
	if got := s.clamp(1e9); got != 50+maxLaps*50 {
		t.Errorf("clamp went to %v", got)
	}
}

func TestLogLapsAreTimesTen(t *testing.T) {
	s := turnSpec{turnLog, 0.0001, 0.02}
	for _, c := range []struct {
		v    float64
		lap  int
		frac float64
	}{{0.01, 0, (0.01 - 0.0001) / 0.0199}, {0.02, 0, 1}, {0.2, 1, 1}, {0.11, 1, 0.5}, {2, 2, 1}} {
		lap, f := s.lap(c.v)
		if lap != c.lap || math.Abs(f-c.frac) > 1e-9 {
			t.Errorf("lap(%v) = %d, %v; want %d, %v", c.v, lap, f, c.lap, c.frac)
		}
	}
	if got := s.clamp(-1); got != 0.0001 {
		t.Errorf("a log knob went below its range: %v", got)
	}
}

func TestWrapComesRound(t *testing.T) {
	s := turnSpec{turnWrap, 0, 360}
	if got := s.clamp(370); math.Abs(got-10) > 1e-9 {
		t.Errorf("370 wrapped to %v", got)
	}
	if got := s.clamp(-10); math.Abs(got-350) > 1e-9 {
		t.Errorf("-10 wrapped to %v", got)
	}
	if p := s.paint(180); p.from != 12 || p.to != 12 || p.uTo >= p.uFrom {
		t.Errorf("paint(180) = %+v, want one LED at 12 and nothing under it", p)
	}
}

func TestRingFillsALap(t *testing.T) {
	s := turnSpec{turnLinear, 0, 50}
	if p := s.paint(50); p.to != ringLEDs-1 || p.lap != 0 || p.uTo >= p.uFrom {
		t.Errorf("the top of the range painted %+v, want the whole ring in lap 0 over nothing", p)
	}
	if p := s.paint(0); p.to != -1 || p.uTo >= p.uFrom {
		t.Errorf("zero painted %+v", p)
	}
}

// Past the top, the next lap's color sweeps over the last lap's, so the ring
// is never dark on its way round again.
func TestALapPaintsOverTheOneBefore(t *testing.T) {
	s := turnSpec{turnLinear, 0, 50}
	p := s.paint(75)
	if p.from != 0 || p.to != ringLEDs/2-1 || p.lap != 1 {
		t.Errorf("half of lap 1 lit %d..%d in lap %d", p.from, p.to, p.lap)
	}
	if p.uFrom != ringLEDs/2 || p.uTo != ringLEDs-1 || p.uLap != 0 {
		t.Errorf("the rest of the ring is %d..%d in lap %d, want lap 0 still lit", p.uFrom, p.uTo, p.uLap)
	}
	// Just past the top: one LED of the new color, the rest the old.
	if p := s.paint(50.5); p.to != -1 && p.to != 0 || p.uTo != ringLEDs-1 {
		t.Errorf("just past the top painted %+v", p)
	}
}

// Below the range the colors come back the other way: lap -1 sweeps
// counter-clockwise from the top over a dark ring, and lap -2 over lap -1.
func TestBelowTheRangeSweepsBack(t *testing.T) {
	s := turnSpec{turnLinear, 0, 50}
	p := s.paint(-12.5) // a quarter of the way down lap -1
	if p.lap != -1 || p.from != ringLEDs*3/4 || p.to != ringLEDs-1 || p.uTo >= p.uFrom {
		t.Errorf("a quarter below painted %+v, want the last quarter in lap -1 over nothing", p)
	}
	p = s.paint(-62.5) // a quarter of the way down lap -2
	if p.lap != -2 || p.from != ringLEDs*3/4 || p.uFrom != 0 || p.uTo != ringLEDs*3/4-1 || p.uLap != -1 {
		t.Errorf("a quarter into lap -2 painted %+v, want it over lap -1", p)
	}
}

func TestLapColorsGoRoundTheList(t *testing.T) {
	for _, c := range []struct{ base, lap, want int }{
		{0, 0, 0}, {0, 1, 1}, {0, 5, 5}, {0, 6, 0}, {2, 1, 3}, {2, -1, 1}, {0, -1, 5}, {1, -8, 5},
	} {
		if got := lapColor(c.base, c.lap, 6); got != c.want {
			t.Errorf("lapColor(%d, %d, 6) = %d, want %d", c.base, c.lap, got, c.want)
		}
	}
}
