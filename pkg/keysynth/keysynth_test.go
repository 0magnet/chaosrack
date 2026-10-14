package keysynth

import (
	"math"
	"testing"
)

// piano is every knob at its default.
var piano = Params{Str: 1, Part: 12, Tilt: 1.15, Inhm: 2.5, Strk: 0.125, Uni: 0.6, Damp: 0.45,
	Dec: 3.2, Ktrk: 1, Brgt: 48, Dark: 48}

// The lowest three partials come from two strings a few cents apart, the
// rest from one, each run a little sharp of its harmonic.
func TestTheStringIsTwoStringsAndStretched(t *testing.T) {
	ps := piano.Partials(60, 24000)
	if len(ps) != 12+3 {
		t.Fatalf("C4 has %d sines, want 15: twelve partials, the lowest three doubled", len(ps))
	}
	if ps[0].Hz >= ps[1].Hz || math.Abs(1200*math.Log2(ps[1].Hz/ps[0].Hz)-1.2) > 0.01 {
		t.Errorf("the fundamental's strings are %v and %v Hz, want 1.2 cents apart", ps[0].Hz, ps[1].Hz)
	}
	top := ps[len(ps)-1]
	if harm := 12 * Hz(60); top.Hz <= harm {
		t.Errorf("the 12th partial is %v Hz, not sharp of %v", top.Hz, harm)
	}
	if top.Tau >= ps[0].Tau {
		t.Error("the 12th partial outlasts the fundamental")
	}
}

// No partial goes past the top of what is heard, nor near the Nyquist.
func TestTheStringStopsBelowTheNyquist(t *testing.T) {
	for _, p := range piano.Partials(100, 24000) {
		if p.Hz > 16000 {
			t.Errorf("a partial at %v Hz", p.Hz)
		}
	}
	if len((&Params{}).Partials(60, 24000)) != 0 {
		t.Error("a string at level 0 has partials")
	}
}

// Every preset is found by its name, and sets only knobs the engine has.
func TestThePresetsSetRealKnobs(t *testing.T) {
	var p Params
	for _, pr := range Presets {
		got, ok := PresetByName(pr.Name)
		if !ok || got.Name != pr.Name {
			t.Errorf("no preset %q", pr.Name)
		}
		for id := range pr.Set {
			if p.Field(id) == nil {
				t.Errorf("preset %s sets %s, which is no knob of the engine's", pr.Name, id)
			}
		}
	}
}
