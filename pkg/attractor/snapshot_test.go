package attractor

import "testing"

// Recall resets every control before re-applying a snapshot, so a snapshot that
// cannot be applied has to be refused BEFORE the reset. Otherwise the reset
// lands and the restore does not, and the user is left with neither the patch
// they clicked nor the view they had.
func TestRecallRefusesAModelThisBuildLacks(t *testing.T) {
	known := func(m string) bool { return m == "thomas" || m == "aizawa" }

	for _, c := range []struct {
		name, snapshot string
		wantMode       string
		wantRefused    bool
	}{
		{"a patch for a model that is gone", "oldmodename&tl=5", "oldmodename", true},
		{"a bare mode token that is gone", "oldmodename", "oldmodename", true},
		{"a patch this build can read", "thomas&tl=5", "", false},
		{"a bare mode this build can read", "aizawa", "", false},
		{"an empty snapshot is not a refusal, it is nothing", "", "", false},
		// No model named at all: there is nothing to fail to find, and the
		// controls still mean something against whatever is on screen.
		{"a snapshot naming no model", "&tl=5", "", false},
	} {
		gone, refused := recallRefusedMode(c.snapshot, known)
		if refused != c.wantRefused || gone != c.wantMode {
			t.Errorf("%s: got (%q, %v), want (%q, %v)",
				c.name, gone, refused, c.wantMode, c.wantRefused)
		}
	}
}

// Every model the mode <select> offers has to be one knownMode agrees with,
// because that is the function the refusal above is decided by. A real model
// missing from the table would make its own patches unrecallable — and would
// already have made its permalinks fail to restore at boot, which reads the
// hash through the same check.
func TestEveryOfferedModeIsKnown(t *testing.T) {
	for _, g := range modeGroups {
		for _, k := range g.Keys {
			if !knownMode(k) {
				t.Errorf("%s offers %q but knownMode does not have it: "+
					"its patches would be refused and its permalinks would not restore", g.Label, k)
			}
		}
	}
}

func TestHashModeOfTakesTheTokenBeforeTheFirstAmpersand(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"thomas&tl=5&p.a=1", "thomas"},
		{"thomas", "thomas"},
		{"", ""},
		{"&tl=5", ""},
	} {
		if got := hashModeOf(c.in); got != c.want {
			t.Errorf("hashModeOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
