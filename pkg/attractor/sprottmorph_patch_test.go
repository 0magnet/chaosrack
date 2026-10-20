package attractor

import (
	"strings"
	"testing"
)

// Stepping itself, the machine re-patches from whole system to whole system:
// two minutes at the default rate pass through several systems and never a
// blend (whose PATCH readout would read "D-E 42%").
func TestTheMorphStepsThroughWholeSystems(t *testing.T) {
	sp := NewSprottMorph(func() float64 { return 0.5 })
	sp.Enter(2000)
	seen := map[string]bool{}
	// Advance's n is steps at 60 a second: 7200 calls is two minutes.
	for range 7200 {
		sp.Advance(1, 1, 2000)
		if l := sp.Label(); strings.Contains(l, "-") {
			t.Fatalf("the machine played a blend, %q, while stepping itself", l)
		}
		seen[sp.Label()] = true
	}
	if len(seen) < 3 {
		t.Errorf("two minutes at the default rate reached only %v", seen)
	}
}

// With the rate at zero a fractional sys still blends, for exploring.
func TestTheSysKnobStillBlendsByHand(t *testing.T) {
	sp := NewSprottMorph(func() float64 { return 0.5 })
	sp.Set("smorph-rate", 0)
	sp.Set("smorph-sys", 3.5)
	sp.Enter(500)
	sp.Advance(1, 1, 500)
	if l := sp.Label(); !strings.Contains(l, "-") {
		t.Errorf("sys 3.5 at rate 0 reads %q, not a blend", l)
	}
}
