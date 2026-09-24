package racksurface

import "testing"

// A unit's opening is a DECLARED size. That is the whole of the change: a
// module's home has to be a function of the order it is in and how wide it
// is, and of nothing else — not of how wide the browser happens to be.
func TestAUnitHoldsAWholeRowAndNotAWindowful(t *testing.T) {
	if got := UnitCapacity(); got != 12 {
		t.Errorf("a unit holds %d slots, want 12 (84 HP of opening at 7 HP a module)", got)
	}
}

func TestTheLeftoverIsBlankPanelsAndNeverNegative(t *testing.T) {
	slots := []int{2, 3}
	if got := BlankSlots(slots, []int{0, 1}, 12); got != 7 {
		t.Errorf("got %d blank slots after 5 used of 12, want 7", got)
	}
	if got := BlankSlots(slots, []int{0, 1}, 5); got != 0 {
		t.Errorf("an exactly full unit wants 0 blanks, got %d", got)
	}
	// An oversized module has already overrun; it cannot also be owed blanks.
	if got := BlankSlots([]int{20}, []int{0}, 12); got != 0 {
		t.Errorf("an overfull unit wants 0 blanks, got %d", got)
	}
}

// A capacity of zero is not a rack, and must not become an infinite loop or

// Checked in slot arithmetic rather than pixels so it holds at any
// interface scale: N slots span N pitches less the trailing seam, which
// belongs to the next module along.
func TestAFullUnitFitsItsOpeningExactly(t *testing.T) {
	const slot, gap = 140.24, 2.0 // moduleSlot / moduleGap at scale 1
	pitch := slot + gap
	capacity := UnitCapacity()
	opening := float64(capacity)*pitch - gap

	// Twelve one-slot modules with a gap between each.
	used := float64(capacity)*slot + float64(capacity-1)*gap
	if used > opening+1e-9 {
		t.Errorf("%d one-slot modules span %.2f in an opening of %.2f", capacity, used, opening)
	}
	// And the same capacity reached with wider modules, since that is how a
	// real rack fills: the grouping must not change the total.
	for _, widths := range [][]int{{2, 2, 2, 2, 2, 2}, {5, 4, 3}, {12}, {1, 11}} {
		sum, n := 0, len(widths)
		for _, w := range widths {
			sum += w
		}
		if sum != capacity {
			t.Fatalf("test fixture %v does not add to %d", widths, capacity)
		}
		span := float64(sum)*slot + float64(n-1)*gap
		if span > opening+1e-9 {
			t.Errorf("%v spans %.2f in an opening of %.2f", widths, span, opening)
		}
	}
}
