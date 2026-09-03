package attractor

import "testing"

// The terminal is a mode like any other and has to be reachable like one. A
// mode that exists in the generator registry but not in the catalog is a mode
// nothing can select — the failure that hid the turtle behind twenty Sprott
// systems, and the reason the catalog invariants are asserted rather than
// assumed.
func TestTerminalIsAReachableMode(t *testing.T) {
	info, ok := modeInfo["terminal"]
	if !ok {
		t.Fatal("terminal is not in the mode registry")
	}
	if info.Label == "" {
		t.Error("terminal has no label")
	}
	if attractorDescriptions["terminal"] == "" {
		t.Error("terminal has no description, so its README section would be empty")
	}

	var found string
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if m.Key == "terminal" {
				found = g.Label
			}
		}
	}
	if found == "" {
		t.Error("terminal is in no catalog category, so the selector cannot reach it")
	}

	// It is drawn as a texture on a plane, not as a trail of vertices. Getting
	// this wrong is not a compile error — it would boot in a random pose and
	// try to draw a plane figure edge-on — so it is asserted.
	if !isTexturePlane("terminal") {
		t.Error("terminal is not marked as a texture-plane mode")
	}
	// And it has no dynamics: it is a picture, not a system. The Analysis
	// module must decline it rather than print an exponent for it.
	if r := LyapunovFor("terminal"); r.Verdict != "n/a" {
		t.Errorf("Lyapunov verdict for a terminal is %q, want n/a", r.Verdict)
	}
}
