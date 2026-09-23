package attractor

import "testing"

// The selector has to reach a module wherever the packer has put it.
//
// This is a regression test for a silent failure, not a style rule. With
// `.modules > .sect` and modules wrapped in bay units, the selector matched
// nothing, buildModEQModules skipped every group, and Audio mod built no
// modulation modules — with no error anywhere, because the CSS that hides
// them while mod is off went on working on modules that did not exist.
func TestModuleSelectorReachesNestedModules(t *testing.T) {
	if !selectorIsNested(moduleSelector) {
		t.Errorf("moduleSelector is %q, a direct-child selector: bay packing "+
			"wraps modules in .runit, so this will match nothing", moduleSelector)
	}
}

func TestSelectorIsNested(t *testing.T) {
	for _, c := range []struct {
		sel  string
		want bool
	}{
		{".modules .sect", true},
		{".modules > .sect", false},
		{".modules>.sect", false},
		{".a .b > .c", false},
	} {
		if got := selectorIsNested(c.sel); got != c.want {
			t.Errorf("selectorIsNested(%q) = %v, want %v", c.sel, got, c.want)
		}
	}
}
