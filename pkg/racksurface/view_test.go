package racksurface

import "testing"

func surf() Surface { return Surface{Cols: 200, Rows: 400} }

func TestClampKeepsTheWindowOnTheSurface(t *testing.T) {
	s := surf()
	v := View{X: 500, Y: 900, W: 80, H: 24}.Clamp(s)
	if v.X != 120 || v.Y != 376 {
		t.Errorf("clamped to %d,%d, want 120,376", v.X, v.Y)
	}
	v = View{X: -5, Y: -5, W: 80, H: 24}.Clamp(s)
	if v.X != 0 || v.Y != 0 {
		t.Errorf("clamped to %d,%d, want 0,0", v.X, v.Y)
	}
}

// When the viewer is bigger than the surface there is nothing to scroll, and
// the offset has to be zero — not the negative slack, which would draw the
// rack indented from the left as the window grows.
func TestAWindowBiggerThanTheSurfaceSitsAtTheOrigin(t *testing.T) {
	s := Surface{Cols: 40, Rows: 10}
	v := View{X: 3, Y: 3, W: 200, H: 60}.Clamp(s)
	if v.X != 0 || v.Y != 0 {
		t.Errorf("got %d,%d, want 0,0", v.X, v.Y)
	}
}

func TestRevealMovesAsLittleAsItCan(t *testing.T) {
	s := surf()
	v := View{X: 100, Y: 100, W: 80, H: 24}
	// Already visible: nothing moves.
	if got := v.Reveal(110, 110, 10, 4, s); got != v {
		t.Errorf("a visible rect moved the window to %v", got)
	}
	// Just off the right edge: the window moves by exactly the overshoot.
	got := v.Reveal(175, 100, 10, 4, s)
	if got.X != 105 {
		t.Errorf("X = %d, want 105 (185 - 80)", got.X)
	}
	// Off the left edge: the window's left goes to the rect's left.
	got = v.Reveal(60, 100, 10, 4, s)
	if got.X != 60 {
		t.Errorf("X = %d, want 60", got.X)
	}
}

func TestSees(t *testing.T) {
	v := View{X: 10, Y: 10, W: 20, H: 10}
	for _, c := range []struct {
		name       string
		x, y, w, h int
		want       bool
	}{
		{"inside", 12, 12, 2, 2, true},
		{"straddling the left edge", 5, 12, 10, 2, true},
		{"entirely left", 0, 12, 5, 2, false},
		{"entirely right", 30, 12, 5, 2, false},
		{"entirely above", 12, 0, 2, 5, false},
		{"entirely below", 12, 20, 2, 5, false},
		{"touching the right edge only", 30, 12, 0, 2, false},
	} {
		if got := v.Sees(c.x, c.y, c.w, c.h); got != c.want {
			t.Errorf("%s: Sees = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBarIsNothingWhenEverythingFits(t *testing.T) {
	if pos, l := Bar(0, 100, 50, 20); l != 0 || pos != 0 {
		t.Errorf("got pos=%d len=%d, want 0 and 0", pos, l)
	}
}

// The thumb must reach the far end of the track at the far end of the
// surface, or the bar says there is more to see when there is not.
func TestBarReachesBothEnds(t *testing.T) {
	const window, total, track = 20, 100, 10
	if pos, _ := Bar(0, window, total, track); pos != 0 {
		t.Errorf("at the top the thumb is at %d, want 0", pos)
	}
	pos, length := Bar(total-window, window, total, track)
	if pos+length != track {
		t.Errorf("at the bottom the thumb ends at %d, want %d", pos+length, track)
	}
}

func TestBarThumbIsNeverZeroLength(t *testing.T) {
	_, length := Bar(0, 1, 10000, 4)
	if length < 1 {
		t.Errorf("thumb length %d, want at least 1", length)
	}
}

func TestPanClampsAtTheEdges(t *testing.T) {
	s := surf()
	v := View{X: 0, Y: 0, W: 80, H: 24}
	if got := v.Pan(-10, -10, s); got.X != 0 || got.Y != 0 {
		t.Errorf("panning off the top-left gave %d,%d", got.X, got.Y)
	}
	if got := v.Pan(9999, 9999, s); got.X != 120 || got.Y != 376 {
		t.Errorf("panning off the bottom-right gave %d,%d, want 120,376", got.X, got.Y)
	}
}
