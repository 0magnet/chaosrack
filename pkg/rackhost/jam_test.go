package rackhost

import (
	"testing"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// With Jam up the rack hops at once to another flow, with a fresh spin about
// Y, and again after 12–20 seconds, never to Custom.
func TestJamHopsBetweenFlows(t *testing.T) {
	r, c := idleRack(t, "polyhedron")
	set(t, r, "jam-sw", "1")
	looks(t, r, c, 0.05)
	r.mu.Lock()
	first, spin := r.model(), r.number("rotation-controls-y")
	r.mu.Unlock()
	if !dynamics.HasFlow(first) || first == "custom" {
		t.Fatalf("Jam hopped to %q, not a flow", first)
	}
	if spin == 0 {
		t.Error("Jam left the model still")
	}
	looks(t, r, c, 11)
	r.mu.Lock()
	still := r.model()
	r.mu.Unlock()
	if still != first {
		t.Errorf("Jam hopped again within 12 s, to %q", still)
	}
	looks(t, r, c, 9.5)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.model() == first {
		t.Errorf("Jam stayed on %q past 20 s", first)
	}
}
