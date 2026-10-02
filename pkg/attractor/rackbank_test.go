package attractor

import "testing"

// The embeddings: tau is takens' and polar's, smooth all three's. Each gets
// one position, and no model has two shared cells in one position.
func TestSharedCellsHaveOnePositionEach(t *testing.T) {
	shared := [][]string{{"takens", "polar"}, {"takens", "stereo", "polar"}}
	pos := bankSharedSlots(shared)
	if pos[0] == pos[1] {
		t.Fatalf("tau and smooth both at %d", pos[0])
	}
	for _, m := range []string{"takens", "stereo", "polar"} {
		seen := map[int]bool{}
		for i, models := range shared {
			for _, mm := range models {
				if mm == m {
					if seen[pos[i]] {
						t.Errorf("%s: two shared cells at %d", m, pos[i])
					}
					seen[pos[i]] = true
				}
			}
		}
	}
}

// A model's own cells fill the positions its shared cells leave, in order,
// and a position kept for a shared cell it is not in stays empty only if a
// later shared cell of its own comes after it.
func TestAModelsOwnCellsFillAroundItsSharedOnes(t *testing.T) {
	shared := [][]string{{"takens", "polar"}, {"takens", "stereo", "polar"}}
	pos := bankSharedSlots(shared) // tau 0, smooth 1
	got := bankModelSlots("stereo", shared, pos, 3)
	// stereo: its own 0 at position 0 (tau is not its), smooth at 1, own 1 and 2 after.
	want := []bankSlot{{-1, 0}, {1, -1}, {-1, 1}, {-1, 2}}
	if len(got) != len(want) {
		t.Fatalf("stereo: %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stereo position %d: %v, want %v", i, got[i], want[i])
		}
	}
	// A model with no own cells still reaches its shared ones, with a gap
	// before them where another model's shared cell stands.
	got = bankModelSlots("stereo", shared, pos, 0)
	if len(got) != 2 || got[0] != (bankSlot{-1, -1}) || got[1] != (bankSlot{1, -1}) {
		t.Errorf("stereo with no own cells: %v", got)
	}
}

func TestAReadoutReadsAsANumberOnACharacterDisplay(t *testing.T) {
	for in, want := range map[string]string{
		"043.0":  "    43.0",
		"0.500":  "   0.500",
		"-012.5": "   -12.5",
		"+0.02":  "   +0.02",
		"000":    "       0",
		"-173.0": "  -173.0",
		"":       "        ",
	} {
		if got := readoutText(in); got != want {
			t.Errorf("readoutText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDisplayTextDropsWhatTheFontCannotDraw(t *testing.T) {
	for in, want := range map[string]string{
		"— none —":         "none",
		"Takens Embedding": "Takens Embedding",
		"sub ÷2":           "sub 2",
	} {
		if got := displayText(in); got != want {
			t.Errorf("displayText(%q) = %q, want %q", in, got, want)
		}
	}
}
