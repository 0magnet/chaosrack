package attractor

import "testing"

// A unit's opening is a DECLARED size. That is the whole of the change: a
// module's home has to be a function of the order it is in and how wide it
// is, and of nothing else — not of how wide the browser happens to be.
func TestAUnitHoldsAWholeRowAndNotAWindowful(t *testing.T) {
	if got := unitCapacitySlots(); got != 12 {
		t.Errorf("a unit holds %d slots, want 12 (84 HP of opening at 7 HP a module)", got)
	}
}

// Fill and overflow, which is what putting cards in a rack is.
func TestModulesFillAUnitThenStartTheNext(t *testing.T) {
	// Twelve one-slot modules exactly fill one unit; the thirteenth starts
	// a second. The boundary is the only place packing ever goes wrong.
	for _, tc := range []struct {
		name  string
		slots []int
		cap   int
		want  [][]int
	}{
		{"exactly full", []int{1, 1, 1, 1}, 4, [][]int{{0, 1, 2, 3}}},
		{"one over", []int{1, 1, 1, 1, 1}, 4, [][]int{{0, 1, 2, 3}, {4}}},
		{"wide module pushed whole", []int{1, 1, 3}, 4, [][]int{{0, 1}, {2}}},
		{"wide module fits", []int{1, 3}, 4, [][]int{{0, 1}}},
		{"two full units", []int{2, 2, 2, 2}, 4, [][]int{{0, 1}, {2, 3}}},
		{"nothing at all", nil, 4, nil},
	} {
		got := packUnits(tc.slots, tc.cap)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range tc.want {
			if len(got[i]) != len(tc.want[i]) {
				t.Errorf("%s: unit %d is %v, want %v", tc.name, i, got[i], tc.want[i])
				continue
			}
			for j := range tc.want[i] {
				if got[i][j] != tc.want[i][j] {
					t.Errorf("%s: unit %d is %v, want %v", tc.name, i, got[i], tc.want[i])
					break
				}
			}
		}
	}
}

// A module wider than a whole unit cannot be made to fit. Refusing to place
// it would lose it, so it gets a unit of its own and overhangs — visible,
// which is the right outcome for a thing that is genuinely too big.
func TestAnOversizedModuleGetsItsOwnUnitRatherThanVanishing(t *testing.T) {
	got := packUnits([]int{1, 20, 1}, 4)
	if len(got) != 3 {
		t.Fatalf("got %v, want three units: the first, the oversized one alone, the last", got)
	}
	if len(got[1]) != 1 || got[1][0] != 1 {
		t.Errorf("the oversized module is in %v, want a unit of its own", got[1])
	}
	// And nothing is lost, whatever the widths.
	seen := map[int]bool{}
	for _, u := range got {
		for _, i := range u {
			if seen[i] {
				t.Errorf("module %d was placed twice", i)
			}
			seen[i] = true
		}
	}
	if len(seen) != 3 {
		t.Errorf("%d modules placed, want all 3", len(seen))
	}
}

// Every module goes somewhere, exactly once, in order — for any widths.
func TestPackingLosesNothingAndKeepsTheOrder(t *testing.T) {
	slots := []int{1, 2, 1, 3, 1, 1, 2, 4, 1, 1, 1, 2, 3, 1}
	units := packUnits(slots, unitCapacitySlots())
	var flat []int
	for _, u := range units {
		flat = append(flat, u...)
	}
	if len(flat) != len(slots) {
		t.Fatalf("%d modules came out of %d", len(flat), len(slots))
	}
	for i, v := range flat {
		if v != i {
			t.Errorf("position %d holds module %d — packing reordered the rack", i, v)
		}
	}
	// And no unit is over capacity unless it holds a single oversized module.
	for u, idx := range units {
		used := unitUsedSlots(slots, idx)
		if used > unitCapacitySlots() && len(idx) != 1 {
			t.Errorf("unit %d holds %d slots of a %d-slot opening", u, used, unitCapacitySlots())
		}
	}
}

// The leftover at the end of a unit is filled with blanks, because a rack
// does not have a ragged gap at the end of a row — it has blank panels, cut
// to the same widths.
func TestTheLeftoverIsBlankPanelsAndNeverNegative(t *testing.T) {
	slots := []int{2, 3}
	if got := unitBlankSlots(slots, []int{0, 1}, 12); got != 7 {
		t.Errorf("got %d blank slots after 5 used of 12, want 7", got)
	}
	if got := unitBlankSlots(slots, []int{0, 1}, 5); got != 0 {
		t.Errorf("an exactly full unit wants 0 blanks, got %d", got)
	}
	// An oversized module has already overrun; it cannot also be owed blanks.
	if got := unitBlankSlots([]int{20}, []int{0}, 12); got != 0 {
		t.Errorf("an overfull unit wants 0 blanks, got %d", got)
	}
}

// A capacity of zero is not a rack, and must not become an infinite loop or
// a unit per module with nothing in it.
func TestADegenerateCapacityStillPacks(t *testing.T) {
	got := packUnits([]int{1, 1, 1}, 0)
	if len(got) != 3 {
		t.Fatalf("got %v, want one module per unit at capacity 1", got)
	}
	for i, u := range got {
		if len(u) != 1 || u[0] != i {
			t.Errorf("unit %d is %v, want just module %d", i, u, i)
		}
	}
}

// A module that is switched OUT takes no room but keeps its place. If it
// took a slot, the rack would reserve space for something that is not in
// it; if it lost its place, putting it back would move it to the front and
// the arrangement the user made would come apart every time they toggled
// one.
func TestASwitchedOutModuleKeepsItsPlaceAndTakesNoRoom(t *testing.T) {
	// Twelve one-slot modules with three switched out between them still
	// fit one twelve-slot unit, because the three take nothing.
	// Twelve switched in, three switched out: the twelve fit exactly.
	slots := []int{1, 0, 1, 1, 0, 1, 1, 1, 1, 0, 1, 1, 1, 1, 1}
	units := packUnits(slots, 12)
	if len(units) != 1 {
		t.Errorf("got %d units, want 1 — the switched-out modules took room", len(units))
	}
	if got := unitUsedSlots(slots, units[0]); got != 12 {
		t.Errorf("unit holds %d slots, want the 12 that are switched in", got)
	}
	// Order is untouched, switched out or not.
	for i, v := range units[0] {
		if v != i {
			t.Errorf("position %d holds module %d — a switched-out module moved", i, v)
		}
	}
}

// Thirteen switched-IN modules do not fit twelve slots, whatever is
// switched out around them.
func TestSwitchedOutModulesDoNotHideAnOverflow(t *testing.T) {
	slots := []int{1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 1}
	units := packUnits(slots, 12)
	if len(units) != 2 {
		t.Fatalf("got %d units for 13 switched-in modules in 12 slots, want 2", len(units))
	}
	if got := unitUsedSlots(slots, units[0]); got > 12 {
		t.Errorf("the first unit holds %d slots of 12", got)
	}
}
