package racksurface

import (
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

// A screen that cannot follow the start of its own section goes first and
// takes that start along, rather than leaving it a row of its own.
func TestAHeadTakesItsOwnSectionAlong(t *testing.T) {
	items := []Item{
		{Key: "colors", Slots: 2, Section: "DISPLAY"},
		{Key: "palette", Slots: 2, Section: "DISPLAY"},
		{Key: "record", Slots: 3, Section: "DISPLAY", Lead: true},
		{Key: "view", Slots: 2, Section: "DISPLAY"},
	}
	got := Pack(items, 12, nil)
	want := [][]int{{2, 0, 1, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Pack = %v, want %v", got, want)
	}
}

// Only the head's own section moves. What precedes it from another section
// stays where it was, and the head still opens a bay of its own.
func TestAHeadLeavesOtherSectionsBehind(t *testing.T) {
	items := []Item{
		{Key: "patchbay", Slots: 1, Section: "MODULATION"},
		{Key: "palette", Slots: 2, Section: "DISPLAY"},
		{Key: "record", Slots: 3, Section: "DISPLAY", Lead: true},
	}
	got := Pack(items, 12, nil)
	want := [][]int{{0}, {2, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Pack = %v, want %v", got, want)
	}
}

// A switched-out module has no width and must not hold a bay open: eight of
// them used to push a one-slot module onto a row of its own.
func TestSwitchedOutModulesTakeNoRoom(t *testing.T) {
	items := []Item{{Key: "rhythm", Slots: 4, Section: "OUTPUT"}}
	for range 8 {
		items = append(items, Item{Key: "mod", Slots: 0, Section: "OUTPUT"})
	}
	items = append(items, Item{Key: "presets", Slots: 1, Section: "UTILITY"})
	if got := Pack(items, 12, nil); len(got) != 1 {
		t.Fatalf("Pack made %d bays, want 1: %v", len(got), got)
	}
}

// Pack always returns. It once looped for ever on a bay that opened with
// zero-width modules: a head entering it was not recorded as leading it,
// and two heads of one section then took turns carrying each other behind
// themselves. Random racks, each given a second to finish.
func TestPackAlwaysReturns(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible test racks, not a secret
	sections := []string{"A", "B", "C"}
	for n := range 20000 {
		items := make([]Item, 3+rng.IntN(14))
		for i := range items {
			items[i] = Item{Slots: rng.IntN(5), Section: sections[rng.IntN(len(sections))], Lead: rng.IntN(3) == 0}
		}
		done := make(chan [][]int, 1)
		go func() { done <- Pack(items, 12, nil) }()
		select {
		case units := <-done:
			seen := map[int]int{}
			for _, u := range units {
				for _, i := range u {
					seen[i]++
				}
			}
			for i := range items {
				if seen[i] != 1 {
					t.Fatalf("case %d: item %d placed %d times: %+v -> %v", n, i, seen[i], items, units)
				}
			}
		case <-time.After(time.Second):
			t.Fatalf("case %d: Pack did not return: %+v", n, items)
		}
	}
}

// The rules Pack promises, on racks shaped like the page's: each section's
// modules contiguous, some of them heads. No bay holds more than it has room
// for, unless it is one module too wide for any; a bay holding a screen begins
// with one; and a section is never split around another.
func TestPackKeepsItsRules(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // reproducible test racks, not a secret
	for n := range 20000 {
		var items []Item
		for s := range 1 + rng.IntN(5) {
			for range 1 + rng.IntN(6) {
				items = append(items, Item{Slots: rng.IntN(6), Section: string(rune('A' + s)), Lead: rng.IntN(4) == 0})
			}
		}
		monitor := map[string]int{}
		if rng.IntN(2) == 0 {
			monitor["B"] = 1 + rng.IntN(3)
		}
		units := Pack(items, 12, monitor)
		var flat []int
		for u, idx := range units {
			flat = append(flat, idx...)
			used, first := 0, -1
			for _, i := range idx {
				used += items[i].Slots
				if first < 0 && items[i].Slots > 0 {
					first = i
				}
			}
			if first >= 0 {
				used += min(monitor[items[first].Section], 11)
			}
			if used > 12 && len(idx) > 1 {
				t.Fatalf("case %d: bay %d holds %d slots: %+v -> %v", n, u, used, items, units)
			}
			for _, i := range idx {
				if items[i].Lead && items[i].Slots > 0 && !items[first].Lead && monitor[items[first].Section] == 0 {
					t.Fatalf("case %d: bay %d has a screen but begins with item %d: %+v -> %v", n, u, first, items, units)
				}
			}
		}
		done := map[string]bool{}
		for k, i := range flat {
			s := items[i].Section
			if k > 0 && items[flat[k-1]].Section != s {
				if done[s] {
					t.Fatalf("case %d: section %s split: %+v -> %v", n, s, items, units)
				}
			}
			if k+1 < len(flat) && items[flat[k+1]].Section != s {
				done[s] = true
			}
		}
	}
}
