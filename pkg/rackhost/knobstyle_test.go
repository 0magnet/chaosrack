package rackhost

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/attractor"
)

// KNOB restyles every knob, the bank's too, and a new bank comes up in the
// style the rack is in.
func TestKnobStyleRestylesTheKnobs(t *testing.T) {
	r := newRack(t)
	count := func(ks attractor.KnobStyle) (bodies, ptrs int) {
		p, err := r.Picture()
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range p.Items {
			switch {
			case it == nil || it.Fill == nil:
			case it.Pointer() && *it.Fill == ks.Pointer:
				ptrs++
			case it.Round == 1 && *it.Fill == ks.Body && it.Border != nil && *it.Border == ks.Border:
				bodies++
			}
		}
		return bodies, ptrs
	}
	std, gold := attractor.KnobStyles["std"], attractor.KnobStyles["gold"]
	b0, p0 := count(std)
	if b0 < 50 || p0 < 20 {
		t.Fatalf("found %d knobs and %d pointers in the std look", b0, p0)
	}
	set(t, r, "knob-style", "gold")
	if b, p := count(gold); b != b0 || p != p0 {
		t.Errorf("gold: %d knobs and %d pointers, want %d and %d", b, p, b0, p0)
	}
	if b, _ := count(std); b != 0 {
		t.Errorf("%d knobs stayed std", b)
	}
	set(t, r, r.selector, "torus")
	if b, _ := count(std); b != 0 {
		t.Errorf("the torus bank came up with %d std knobs", b)
	}
	set(t, r, "knob-style", "std")
	if b, _ := count(gold); b != 0 {
		t.Errorf("%d knobs stayed gold", b)
	}
}
