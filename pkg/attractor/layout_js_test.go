//go:build js && wasm

package attractor

import "testing"

// The drawer travels the whole edge. It used to stop 120px short of shut and
// 4% short of full, and a drawer that will not close and will not open all
// the way is a drawer arguing with the hand on it.
func TestTheDockTravelsFromShutToFull(t *testing.T) {
	const win = 900.0
	const grip = 10.0
	if got := clampDock(0, grip, win); got != grip {
		t.Errorf("pulled shut: got %v, want the grip at %v", got, grip)
	}
	if got := clampDock(-500, grip, win); got != grip {
		t.Errorf("pulled past shut: got %v, want the grip at %v", got, grip)
	}
	if got := clampDock(win, grip, win); got != win {
		t.Errorf("pulled to full: got %v, want the whole window %v", got, win)
	}
	if got := clampDock(win*2, grip, win); got != win {
		t.Errorf("pulled past full: got %v, want the whole window %v", got, win)
	}
	// And everything in between is left exactly where it was put.
	for _, v := range []float64{grip + 1, 120, 450, win - 1} {
		if got := clampDock(v, grip, win); got != v {
			t.Errorf("mid-travel %v came back as %v", v, got)
		}
	}
}

// A window smaller than the grip is not a reason to return a size larger
// than the window — the panel would hang off the screen it is docked to.
func TestTheDockNeverExceedsAWindowSmallerThanItsGrip(t *testing.T) {
	if got := clampDock(500, 40, 20); got != 20 {
		t.Errorf("got %v in a 20px window, want 20", got)
	}
}
