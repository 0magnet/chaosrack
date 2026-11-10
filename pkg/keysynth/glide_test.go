package keysynth

import (
	"slices"
	"testing"
)

// keys ten pixels wide along x, numbered from 60; nothing below y = 0.
func strip(x, y float64) (int, bool) {
	if y < 0 || x < 0 {
		return 0, false
	}
	return 60 + int(x/10), true
}

func TestAGlideSoundsEveryKeyItCrossesInOrder(t *testing.T) {
	if got, want := GlidePath(5, 5, 45, 5, 60, strip), []int{61, 62, 63, 64}; !slices.Equal(got, want) {
		t.Errorf("a drag across four keys played %v, want %v", got, want)
	}
	if got, want := GlidePath(45, 5, 5, 5, 64, strip), []int{63, 62, 61, 60}; !slices.Equal(got, want) {
		t.Errorf("the same drag backwards played %v, want %v", got, want)
	}
}

func TestAGlideLeavesOutTheHeldKeyAndWhatIsOffTheKeys(t *testing.T) {
	if got := GlidePath(2, 5, 8, 5, 60, strip); len(got) != 0 {
		t.Errorf("a move inside the held key played %v", got)
	}
	if got := GlidePath(5, -20, 45, -20, 60, strip); len(got) != 0 {
		t.Errorf("a drag off the keys played %v", got)
	}
	// Leaving the held key and coming back to it is a new note.
	if got, want := GlidePath(15, 5, 5, 5, 62, strip), []int{61, 60}; !slices.Equal(got, want) {
		t.Errorf("played %v, want %v", got, want)
	}
}

func TestGlideSpanIsBounded(t *testing.T) {
	for in, want := range map[float64]float64{0: 0.02, 0.05: 0.05, 3: 0.15} {
		if got := GlideSpan(in); got != want {
			t.Errorf("GlideSpan(%v) = %v, want %v", in, got, want)
		}
	}
}
