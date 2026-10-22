package attractor

import (
	"slices"
	"testing"
)

// The bank's groups (the section's plane, the Turtle's weight, the
// spectrogram, the water lens): a run after its models' own cells starts
// past the furthest of them, the tail runs stand against the end one before
// the other, and a model whose own cells reach a tail run goes without it.
func TestBankGroupsFitTheBankOrGoWithout(t *testing.T) {
	used := map[string]int{"lorenz": 4, "aizawa": 7, "turtle": 11, "stereo": 18, "henon": 2}
	groups := []bankGroup{
		{Size: 3, Models: []string{"lorenz", "aizawa"}},
		{Size: 4, Models: []string{"turtle"}},
		{Size: 8, Models: []string{"lorenz", "aizawa", "turtle", "stereo", "henon"}, Tail: true},
		{Size: 7, Models: []string{"lorenz", "aizawa", "turtle", "stereo", "henon"}, Tail: true},
	}
	starts, served := bankGroupSlots(used, groups, 25, nil)
	if want := []int{7, 11, 17, 10}; !slices.Equal(starts, want) {
		t.Fatalf("starts %v, want %v", starts, want)
	}
	if !slices.Equal(served[2], []string{"lorenz", "aizawa", "turtle", "henon"}) {
		t.Errorf("the water run serves %v: stereo's own cells reach it, the rest stop short", served[2])
	}
	// Turtle's weight takes it to 15, past the spectrogram's run at 10.
	if !slices.Equal(served[3], []string{"lorenz", "aizawa", "henon"}) {
		t.Errorf("the spectrogram run serves %v", served[3])
	}
	for i, g := range groups {
		if starts[i]+g.Size > 25 {
			t.Errorf("group %d runs to %d, past the end", i, starts[i]+g.Size)
		}
	}
}

// align moves a run after own cells on, and leaves the tail runs alone.
func TestBankGroupsAlignTheRunsAfterOwnCells(t *testing.T) {
	used := map[string]int{"lorenz": 3, "aizawa": 6}
	groups := []bankGroup{
		{Size: 3, Models: []string{"lorenz", "aizawa"}},
		{Size: 8, Models: []string{"lorenz", "aizawa"}, Tail: true},
	}
	next3 := func(p int) int { return (p + 2) / 3 * 3 }
	starts, _ := bankGroupSlots(used, groups, 25, next3)
	if want := []int{6, 17}; !slices.Equal(starts, want) {
		t.Errorf("starts %v, want %v", starts, want)
	}
	used["aizawa"] = 7
	if starts, _ = bankGroupSlots(used, groups, 25, next3); starts[0] != 9 {
		t.Errorf("a run after own cells reaching 7 starts at %d, want 9", starts[0])
	}
}
