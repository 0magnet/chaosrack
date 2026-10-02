package attractor

import (
	"slices"
	"testing"
)

// A section with several bays names each one apart, in order.
func TestBayKeysNameASectionsBaysApart(t *testing.T) {
	got := bayKeys([]string{"cat-visual", "display", "gen", "gen", "gen", "mod"})
	want := []string{"cat-visual#0", "display#0", "gen#0", "gen#1", "gen#2", "mod#0"}
	if !slices.Equal(got, want) {
		t.Errorf("bayKeys = %v, want %v", got, want)
	}
}

// A saved order puts the bays it names where it says, and a bay it does not
// name stays at its factory place.
func TestArrangeBaysKeepsTheSavedOrderAndPlacesTheRest(t *testing.T) {
	factory := []string{"a#0", "b#0", "c#0", "d#0"}
	for _, c := range []struct {
		saved, want []string
		why         string
	}{
		{nil, factory, "nothing saved: the factory order"},
		{[]string{"d#0", "c#0", "b#0", "a#0"}, []string{"d#0", "c#0", "b#0", "a#0"}, "every bay named"},
		{[]string{"c#0", "a#0"}, []string{"c#0", "b#0", "a#0", "d#0"}, "two named, the other two at their places"},
		{[]string{"gone#0", "b#0", "b#0"}, []string{"a#0", "b#0", "c#0", "d#0"}, "a bay no longer there, and a duplicate, are dropped"},
	} {
		if got := arrangeBays(factory, c.saved); !slices.Equal(got, c.want) {
			t.Errorf("%s: arrangeBays(%v) = %v, want %v", c.why, c.saved, got, c.want)
		}
	}
}

// One screw moves a bay one place, and past either end it comes round.
func TestMoveBayStepsAndComesRound(t *testing.T) {
	o := []string{"a", "b", "c", "d"}
	for _, c := range []struct {
		i, dir int
		want   []string
	}{
		{2, -1, []string{"a", "c", "b", "d"}},
		{1, 1, []string{"a", "c", "b", "d"}},
		{0, -1, []string{"b", "c", "d", "a"}}, // the top pushed up goes to the bottom
		{3, 1, []string{"d", "a", "b", "c"}},  // the bottom pushed down goes to the top
		{9, 1, o},
	} {
		if got := moveBay(o, c.i, c.dir); !slices.Equal(got, c.want) {
			t.Errorf("moveBay(%d, %d) = %v, want %v", c.i, c.dir, got, c.want)
		}
	}
	if !slices.Equal(o, []string{"a", "b", "c", "d"}) {
		t.Errorf("moveBay changed the order it was given: %v", o)
	}
}
