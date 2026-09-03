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
	for s := 0.0; s <= 120.0; s += 0.25 {
		hz := freqFromKnob(s)
		back := knobFromFreq(hz)
		if math.Abs(back-s) > 0.001 {
			t.Errorf("round-trip %v semis → %v Hz → %v semis", s, hz, back)
		}
	}
}
