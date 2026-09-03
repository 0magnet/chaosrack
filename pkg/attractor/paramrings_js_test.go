//go:build js && wasm

package attractor

import "testing"

// A dial's short ring labels have to match its options one for one, and stay
// short enough to read beside the knob.
//
// buildParamUnit DISCARDS a ring whose length disagrees with the option list
// and falls back to the full names — which is a reasonable safety net and a
// silent one. It bit exactly once already: spect-col listed three short labels
// while spectColNames grew to six (turbo, viridis and magma were appended), so
// the ring was thrown away and the dial came up ringed with "grayscale" and
// "viridis" running under the knob. That dial is the worst possible one to lose,
// because for a named setting the LED readout is hidden deliberately — the ring
// IS the readout, so an unreadable ring means no way to tell which color map
// is selected.
//
// Length is the part that silently degrades, so length is what this pins.
func TestRingLabelsMatchTheirOptions(t *testing.T) {
	if len(paramRingLabels) == 0 {
		t.Fatal("no ring labels at all, so this test proves nothing")
	}
	for id, ring := range paramRingLabels {
		opts, ok := paramLabels[id]
		if !ok {
			t.Errorf("%s has ring labels but no options, so the ring is dead markup", id)
			continue
		}
		if len(ring) != len(opts) {
			t.Errorf("%s: %d ring labels for %d options — buildParamUnit will throw the ring away "+
				"and ring the dial with the full names, which do not fit", id, len(ring), len(opts))
		}
	}
}

// The width limit is the reason the short list exists: a cell is a third of a
// module wide, and paramdefs_js.go puts the limit at about four characters.
// Five is allowed as slack; past that a label runs into its neighbor and the
// knob covers what is left.
func TestRingLabelsAreShortEnoughToRead(t *testing.T) {
	const maxRingLabel = 5
	for id, ring := range paramRingLabels {
		for _, l := range ring {
			if len([]rune(l)) > maxRingLabel {
				t.Errorf("%s: ring label %q is %d characters; past %d it runs into its neighbor "+
					"and the knob hides the rest", id, l, len([]rune(l)), maxRingLabel)
			}
		}
	}
}

// WHATEVER ENDS UP ROUND A DIAL HAS TO FIT.
//
// This used to demand that every named setting have a short ring, on the
// grounds that without one the dial is ringed with full names. That was the
// right worry and the wrong rule: some settings have names that cannot be
// shortened usefully — the twenty-one terminal demos are words like "metaballs"
// and "fireworks" — and abbreviating them to five characters would trade one
// unreadable dial for another. Those take selectorKnobReadout instead, which
// prints the whole name under the knob.
//
// So the rule is the outcome rather than the mechanism: for every named
// setting, whatever the panel would ring the dial with must FIT. A short ring
// that is itself too long is the case worth catching, because it looks like the
// problem has been dealt with.
func TestWhateverIsRingedFits(t *testing.T) {
	if len(paramLabels) == 0 {
		t.Fatal("no named settings at all, so this test proves nothing")
	}
	for id, opts := range paramLabels {
		if len(opts) == 2 {
			continue // a two-option setting is a switch; nothing is ringed
		}
		ring, ok := paramRingLabels[id]
		if !ok {
			// No short ring: the panel rings the full names when they fit and
			// falls back to the readout when they do not. Either is fine — what
			// is not fine is a short ring that does not help.
			continue
		}
		if !ringLabelsFit(ring) {
			t.Errorf("%s has a short ring that still does not fit: %v — either shorten it "+
				"or drop it and let the dial take the readout", id, ring)
		}
	}
}
