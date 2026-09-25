package racksurface

import (
	"reflect"
	"testing"
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
