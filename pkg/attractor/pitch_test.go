package attractor

import (
	"math"
	"testing"
)

// Concert-pitch anchors: the semitone slider is A0-anchored, so every A lands
// exactly on a power of two times 27.5 Hz — 440 Hz must be semitone 48.
func TestPitchAnchors(t *testing.T) {
	cases := []struct {
		semis float64
		hz    float64
	}{
		{0, 27.5},     // A0
		{12, 55},      // A1
		{24, 110},     // A2
		{48, 440},     // A4 — concert pitch
		{120, 28160},  // A10
		{57, 739.989}, // F#5 ≈ 739.99 (equal temperament)
	}
	for _, c := range cases {
		got := freqFromKnob(c.semis)
		if math.Abs(got-c.hz) > 0.01 {
			t.Errorf("freqFromKnob(%v) = %v Hz, want %v", c.semis, got, c.hz)
		}
	}
}

// Round-trip: slider→Hz→slider must return within a thousandth of a semitone
// across the full range (the LED typed-entry path depends on this inverse).
func TestPitchRoundTrip(t *testing.T) {
	for s := float64(genKnobLo); s <= genKnobHi; s += 0.25 {
		hz := freqFromKnob(s)
		back := knobFromFreq(hz)
		if math.Abs(back-s) > 0.001 {
			t.Errorf("round-trip %v semis → %v Hz → %v semis", s, hz, back)
		}
	}
}

// FREQ goes eight octaves under A0, an octave a turn, and no further either
// way; its laps start at C, as the piano strip does.
func TestTheFreqEncoderReachesBelowThePiano(t *testing.T) {
	if hz := freqFromKnob(genKnobLo); math.Abs(hz-27.5/256) > 1e-9 {
		t.Errorf("the bottom is %v Hz, want %v", hz, 27.5/256)
	}
	if got := knobFromFreq(0.001); got != genKnobLo {
		t.Errorf("a frequency under the bottom types as %v, want %v", got, genKnobLo)
	}
	s := genFreqTurns
	if lo, hi := s.domain(); lo != genKnobLo || hi != genKnobHi {
		t.Errorf("domain %v..%v, want %v..%v", lo, hi, genKnobLo, genKnobHi)
	}
	if v := s.clamp(-1000); v != genKnobLo {
		t.Errorf("clamped low to %v", v)
	}
	if v := s.clamp(1000); v != genKnobHi {
		t.Errorf("clamped high to %v", v)
	}
	// Every C is the top of the ring, whatever the octave.
	for c := 3.0; c <= genKnobHi; c += 12 {
		if a := math.Mod(s.angle(c)+360, 360); a > 1e-9 && math.Abs(a-360) > 1e-9 {
			t.Errorf("C at %v semitones points at %v°", c, a)
		}
	}
	// And a semitone is a twelfth of a turn.
	if d := s.angle(genTurnLo+1) - s.angle(genTurnLo); math.Abs(d-30) > 1e-9 {
		t.Errorf("a semitone turns %v°, want 30", d)
	}
}
