package attractor

import "testing"

// A graticule is a ruler, so the test is a measurement and not a snapshot:
// count the divisions, check the spacing is even, check nothing escapes the
// screen. A graticule that is merely grid-shaped would pass a screenshot
// comparison and still be unreadable.

func TestGraticuleIsEightByTenDivisions(t *testing.T) {
	g := scopeGraticule()

	// Count the full-height verticals and full-width horizontals, center
	// axes included. Ten horizontal divisions are made by eleven verticals,
	// eight vertical divisions by nine horizontals.
	verts, horzs := 0, 0
	for _, l := range g {
		if l.W == gratWeightTick {
			continue
		}
		switch {
		case l.X0 == l.X1 && l.Y0 == -gratHalfH && l.Y1 == gratHalfH:
			verts++
		case l.Y0 == l.Y1 && l.X0 == -gratHalfW && l.X1 == gratHalfW:
			horzs++
		}
	}
	if verts != gratDivX+1 {
		t.Errorf("%d full-height lines, want %d for %d horizontal divisions",
			verts, gratDivX+1, gratDivX)
	}
	if horzs != gratDivY+1 {
		t.Errorf("%d full-width lines, want %d for %d vertical divisions",
			horzs, gratDivY+1, gratDivY)
	}
}

// The center axes are what a reading is taken against, so they must exist,
// must be exactly two, and must be drawn at their own weight — a graticule
// of uniform lines is the thing this file was written to stop being.
func TestTheCenterAxesAreDrawnHeavierAndAreNotDuplicated(t *testing.T) {
	var axes int
	for _, l := range scopeGraticule() {
		if l.W != gratWeightAxis {
			continue
		}
		axes++
		onCenter := (l.X0 == 0 && l.X1 == 0) || (l.Y0 == 0 && l.Y1 == 0)
		if !onCenter {
			t.Errorf("axis-weight line %v is not on a center axis", l)
		}
	}
	if axes != 2 {
		t.Errorf("%d axis-weight lines, want exactly 2", axes)
	}
	// And no division line may also sit on zero, or the axis is drawn twice
	// and reads heavier on one screen than another.
	for _, l := range scopeGraticule() {
		if l.W != gratWeightDiv {
			continue
		}
		if (l.X0 == 0 && l.X1 == 0) || (l.Y0 == 0 && l.Y1 == 0) {
			t.Errorf("division line %v duplicates a center axis", l)
		}
	}
}

// Minor ticks let you interpolate a fifth of a division. They must be evenly
// spaced, must not land on a division line, and must stop at the screen edge.
func TestMinorTicksSubdivideEveryDivisionEvenly(t *testing.T) {
	for _, tc := range []struct {
		name string
		half int
	}{{"horizontal", gratHalfW}, {"vertical", gratHalfH}} {
		ticks := gratTicks(tc.half)
		// Four ticks per division, over 2*half divisions.
		if want := 4 * 2 * tc.half; len(ticks) != want {
			t.Errorf("%s: %d ticks, want %d (four per division over %d)",
				tc.name, len(ticks), want, 2*tc.half)
		}
		for i, v := range ticks {
			if v < -float32(tc.half) || v > float32(tc.half) {
				t.Errorf("%s: tick %d at %v is off the screen", tc.name, i, v)
			}
			if i > 0 {
				// Never two ticks in the same place, and never a gap wider
				// than a division (which is what a missed skip looks like).
				if d := v - ticks[i-1]; d <= 0 || d > 1.0001 {
					t.Errorf("%s: tick %d follows the last by %v", tc.name, i, d)
				}
			}
		}
	}
}

// The risetime references are the reason the marks are there at all: 0% and
// 100% four divisions apart, 10% and 90% a tenth of that inside each.
func TestRisetimeReferencesAreFourDivisionsApartWithThePercentMarksInside(t *testing.T) {
	var full, short []float32
	for _, l := range scopeGraticule() {
		if l.W != gratWeightTick || l.Y0 != l.Y1 {
			continue
		}
		if l.X0 == -gratHalfW && l.X1 == gratHalfW {
			full = append(full, l.Y0)
		} else if l.X0 == -gratHalfW {
			short = append(short, l.Y0)
		}
	}
	if len(full) != 2 {
		t.Fatalf("%d full-width reference lines, want 2 (0%% and 100%%)", len(full))
	}
	if d := full[1] - full[0]; d != 2*gratRefDiv && d != -2*gratRefDiv {
		t.Errorf("the references are %v divisions apart, want %v", d, 2*gratRefDiv)
	}
	if len(short) != 2 {
		t.Fatalf("%d percent marks, want 2 (10%% and 90%%)", len(short))
	}
	// Each percent mark is inside its own reference, never outside it.
	for _, p := range short {
		near := full[0]
		if abs32(p-full[1]) < abs32(p-full[0]) {
			near = full[1]
		}
		if abs32(p) >= abs32(near) {
			t.Errorf("percent mark at %v is outside its reference at %v", p, near)
		}
		// Compared with a tolerance: these are float32 division coordinates
		// and 2.0-0.4 is not exactly 1.6 in them. A tenth of a tick is far
		// tighter than anything that could be read off a screen.
		if got := abs32(abs32(near) - abs32(p)); abs32(got-gratPctDiv) > gratTickDiv/10 {
			t.Errorf("percent mark at %v is %v from its reference, want %v", p, got, gratPctDiv)
		}
	}
}

// Nothing may be drawn outside the tube. A graticule that overruns is a
// graticule drawn over the bezel.
func TestNothingEscapesTheScreen(t *testing.T) {
	for _, l := range scopeGraticule() {
		for _, x := range []float32{l.X0, l.X1} {
			if x < -gratHalfW || x > gratHalfW {
				t.Errorf("line %v runs to x=%v, past the %v-division edge", l, x, gratHalfW)
			}
		}
		for _, y := range []float32{l.Y0, l.Y1} {
			if y < -gratHalfH || y > gratHalfH {
				t.Errorf("line %v runs to y=%v, past the %v-division edge", l, y, gratHalfH)
			}
		}
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
