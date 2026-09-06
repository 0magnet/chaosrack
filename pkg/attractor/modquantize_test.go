package attractor

import "testing"

// The grid a count parameter is read on has to be the SLIDER's grid, which
// starts at min and steps from there. A value off that lattice is one the user
// cannot dial and the LED cannot round-trip, so audio would be able to put the
// figure into a state the panel cannot describe or reproduce.
//
// The cases with a min that is not a multiple of the step are the ones that
// matter: anchoring at zero passes every parameter in the build today by luck
// and fails the first one added whose range does not start on the lattice.
func TestSnapToStepUsesTheSlidersOwnGrid(t *testing.T) {
	cases := []struct {
		name                    string
		v, min, max, step, want float32
	}{
		{"sphere latitude rounds down", 29.4, 4, 100, 1, 29},
		{"sphere latitude rounds up", 29.6, 4, 100, 1, 30},
		{"exactly halfway rounds up", 29.5, 4, 100, 1, 30},
		{"overlap % lands on its multiple of five", 52.4, 5, 95, 5, 50},
		{"overlap % lands on the next multiple of five", 53.1, 5, 95, 5, 55},
		{"a min off the lattice keeps the offset", 8.4, 3, 99, 2, 9},
		{"a min off the lattice, other side", 7.6, 3, 99, 2, 7},
		{"below the range clamps to min", -20, 4, 100, 1, 4},
		{"above the range clamps to max", 400, 4, 100, 1, 100},
		{"clamping wins over the grid at the top", 94, 5, 95, 5, 95},
		{"no grid passes the value through clamped", 3.14159, 0, 10, 0, 3.14159},
	}
	for _, c := range cases {
		if got := snapToStep(c.v, c.min, c.max, c.step); got != c.want {
			t.Errorf("%s: snapToStep(%v, %v, %v, %v) = %v, want %v",
				c.name, c.v, c.min, c.max, c.step, got, c.want)
		}
	}
}

// Everything the geometry generators do with these values is int(v), which
// TRUNCATES. A snap that came back at 29.999997 would draw 29 lines while the
// LED read 30, and the two would disagree for as long as the knob sat there.
// Every integral step and min in the build is exactly representable in float32
// at these magnitudes, so this is a guard on the arithmetic rather than a
// suspicion about it — but it is the arithmetic the display depends on.
func TestSnapToStepLandsExactlyOnWholeNumbers(t *testing.T) {
	for _, g := range []struct{ min, max, step float32 }{
		{4, 100, 1}, {3, 100, 1}, {5, 95, 5}, {20, 2000, 20}, {500, 12000, 50}, {0, 20000, 100},
	} {
		for v := g.min; v <= g.max; v += g.step {
			for _, off := range []float32{-0.3, 0, 0.3} {
				got := snapToStep(v+off, g.min, g.max, g.step)
				if float32(int(got)) != got {
					t.Fatalf("snapToStep(%v, %v, %v, %v) = %v, which int() truncates to %d",
						v+off, g.min, g.max, g.step, got, int(got))
				}
			}
		}
	}
}

// Without a previous value there is nothing to be sticky about, so the first
// sample must land on the nearest notch rather than anywhere else. The path
// matters because it is the one taken the first frame a route is switched on:
// a knob that took a few frames to reach the right value would read as lag.
func TestQuantizeHeldSnapsWhenThereIsNothingHeld(t *testing.T) {
	if got := quantizeHeld(29.6, 0, false, 4, 100, 1); got != 30 {
		t.Errorf("first sample gave %v, want the nearest notch 30", got)
	}
	if got := quantizeHeld(29.4, 999, false, 4, 100, 1); got != 29 {
		t.Errorf("first sample gave %v, want 29 — a held value with hasHeld false must be ignored", got)
	}
}

// The trigger's switching points. Inside ±(0.5+deadband)·Step of the held
// value nothing moves; the first sample past it snaps to whatever is nearest.
// Written against the constant rather than against 0.65 so tuning the deadband
// re-derives the expectations instead of breaking them.
func TestQuantizeHeldHoldsInsideTheDeadbandAndReleasesOutside(t *testing.T) {
	const held = 30
	edge := float32(0.5 + modStepDeadband)
	for _, d := range []float32{0, 0.4, edge - 0.01, -(edge - 0.01), -0.4} {
		if got := quantizeHeld(held+d, held, true, 4, 100, 1); got != held {
			t.Errorf("v=%v (%v from the held value) moved to %v; inside the deadband it must hold",
				held+d, d, got)
		}
	}
	if got := quantizeHeld(held+edge+0.01, held, true, 4, 100, 1); got != 31 {
		t.Errorf("v just past the upper switching point gave %v, want 31", got)
	}
	if got := quantizeHeld(held-edge-0.01, held, true, 4, 100, 1); got != 29 {
		t.Errorf("v just past the lower switching point gave %v, want 29", got)
	}
}

// A held value must never lag behind a deliberate move. Dragging the base
// slider from one end of the range to the other jumps the modulated value by
// many steps at once, and the trigger has to follow it in a single frame —
// releasing to the NEAREST notch, not to the neighboring one.
func TestQuantizeHeldFollowsALargeJumpImmediately(t *testing.T) {
	if got := quantizeHeld(87.2, 30, true, 4, 100, 1); got != 87 {
		t.Errorf("a jump from 30 to 87.2 settled at %v, want 87", got)
	}
	if got := quantizeHeld(-100, 30, true, 4, 100, 1); got != 4 {
		t.Errorf("a jump below the range settled at %v, want the min 4", got)
	}
}

// A held value that is stale — left over from a route switched off and later
// switched back on, or from a step the user has since changed — must not pin
// the parameter off its own grid or outside its own range.
func TestQuantizeHeldCannotStrandAValueOffTheGrid(t *testing.T) {
	// Held at 1000 with the range 4..100: every real v is far outside the
	// deadband, so the very first sample resnaps.
	if got := quantizeHeld(30.2, 1000, true, 4, 100, 1); got != 30 {
		t.Errorf("a stale held value of 1000 gave %v, want 30", got)
	}
	// step <= 0 (a continuous parameter reaching the count path by mistake)
	// degrades to the clamped float rather than dividing by zero or holding.
	if got := quantizeHeld(3.14159, 2, true, 0, 10, 0); got != 3.14159 {
		t.Errorf("step 0 gave %v, want the value passed through clamped", got)
	}
}
