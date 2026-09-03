package attractor

import "testing"

// The truth table for whether a module gets a show/hide switch.
//
// The case this exists for is the third row: a module the RACK has put away is
// display:none BECAUSE the rack put it away, and reading that as "pinned" meant
// no switch was built for it. Since restore hides modules before the switches
// are made, that state came back on every load — hide a module, reload, and the
// only way to see it again was to clear the browser's storage. Measured at the
// time: thirteen switches before the reload and twelve after.
func TestModulePinnedRule(t *testing.T) {
	for _, c := range []struct {
		name                               string
		neverSwitched, rackHidden, display bool
		want                               bool
	}{
		{"an ordinary visible module gets a switch", false, false, false, false},
		{"the Console never gets one", true, false, false, true},
		{"the Console never gets one, even hidden", true, true, true, true},
		{"a module the rack put away KEEPS its switch", false, true, true, false},
		{"a module a mode took away gets none", false, false, true, true},
	} {
		if got := modulePinnedFrom(c.neverSwitched, c.rackHidden, c.display); got != c.want {
			t.Errorf("%s: pinned=%v, want %v", c.name, got, c.want)
		}
	}
}

// A module the rack is hiding must never be pinned, whatever its display says.
// Stated separately from the table because it is the invariant the feature
// rests on: the switch is the only way back, so the one module that certainly
// needs one is the one that is currently put away.
func TestHiddenModuleAlwaysKeepsItsSwitch(t *testing.T) {
	for _, display := range []bool{true, false} {
		if modulePinnedFrom(false, true, display) {
			t.Errorf("a rack-hidden module was pinned (display:none=%v), so nothing would bring it back", display)
		}
	}
}
