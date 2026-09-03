package attractor

import (
	"strings"
	"testing"
)

// The desk is a mode like any other and has to be reachable like one, for the
// same reasons the terminal does — see TestTerminalIsAReachableMode.
func TestDeskIsAReachableMode(t *testing.T) {
	info, ok := modeInfo["desk"]
	if !ok {
		t.Fatal("desk is not in the mode registry")
	}
	if info.Label == "" {
		t.Error("desk has no label")
	}
	if attractorDescriptions["desk"] == "" {
		t.Error("desk has no description, so its README section would be empty")
	}

	var found string
	for _, g := range Catalog() {
		for _, m := range g.Models {
			if m.Key == "desk" {
				found = g.Label
			}
		}
	}
	if found == "" {
		t.Error("desk is in no catalog category, so the selector cannot reach it")
	}

	if !isTexturePlane("desk") {
		t.Error("desk is not marked as a texture-plane mode")
	}
	if r := LyapunovFor("desk"); r.Verdict != "n/a" {
		t.Errorf("Lyapunov verdict for a desk is %q, want n/a", r.Verdict)
	}
}

func TestDeskAndTerminalAreSeparateModes(t *testing.T) {
	// They look alike from here — both are a canvas on a quad — and they are
	// not the same thing: one is a terminal and one is a window manager with
	// terminals in it. A copy-paste that left them sharing a key or a label
	// would give the selector two positions that do the same thing.
	if modeInfo["desk"].Label == modeInfo["terminal"].Label {
		t.Error("desk and terminal share a label")
	}
	if attractorDescriptions["desk"] == attractorDescriptions["terminal"] {
		t.Error("desk and terminal share a description")
	}
}

func TestDeskDescriptionSaysWhichSwitchToUseInstead(t *testing.T) {
	// The mode does not take input, deliberately, and a model you cannot type
	// into is indistinguishable from a broken one unless it says so. The
	// description is the only place that can, since there is nowhere on a
	// rotated quad to put a hint.
	d := attractorDescriptions["desk"]
	if !strings.Contains(d, "Desk") {
		t.Error("the desk model's description does not point at the Desk switch")
	}
	if !strings.Contains(strings.ToLower(d), "rotate") {
		t.Error("the description does not say what it is for")
	}
}
