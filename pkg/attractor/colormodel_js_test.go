//go:build js && wasm

package attractor

import "testing"

// The colour model is two knobs: SRC says what the colour follows, MAP says how
// that value becomes a colour. These check the two places the split has to hold
// together — the uniform the geometry reads, and which modes consult SRC at all.

func withColorKnobs(t *testing.T) {
	t.Helper()
	s, c, m := gradientSource, gradientColors, selectedMode
	t.Cleanup(func() { gradientSource, gradientColors, selectedMode = s, c, m })
}

// TestSourceOffOnlySilencesTheGeometry is the load-bearing asymmetry.
//
// OFF means "the colour follows nothing", which is a thing you can say about a
// figure whose colour is a CHOICE — an attractor, an embedding, the waterfall.
// The spectrogram, the RTA and the transfer function each colour one quantity
// of their own and never consulted the src ring, so OFF must not silence them:
// they go on reading the map ring. That is why gradientColors stays the map and
// only the shader's uniform folds OFF in.
func TestSourceOffOnlySilencesTheGeometry(t *testing.T) {
	withColorKnobs(t)
	gradientColors = 9 // viridis

	gradientSource = 2 // Z
	if got := gradientColorsUniform(); got != 9 {
		t.Errorf("source Z: uniform %d, want the map ring's 9", got)
	}
	gradientSource = GradientSourceOff
	if got := gradientColorsUniform(); got != 1 {
		t.Errorf("source OFF: uniform %d, want the shader's monochrome 1", got)
	}
	// The map ring itself does not move, which is what the analyzers and the
	// spectrogram read.
	if gradientColors != 9 {
		t.Errorf("source OFF moved the map ring to %d", gradientColors)
	}
	if _, ok := analyzerPalette(); !ok {
		t.Error("the analyzers lost their colormap when the source was turned off")
	}
}

// TestOnlyTheGeometryHasASourceToChoose pins which modes dim the src ring.
func TestOnlyTheGeometryHasASourceToChoose(t *testing.T) {
	withColorKnobs(t)
	for _, m := range []string{"spectrogram", "rta", "xfer"} {
		if modeUsesGradientSource(m) {
			t.Errorf("%s has one quantity of its own and should not consult SRC", m)
		}
	}
	for _, m := range []string{"takens", "stereo", "polar", "waterfall", "lorenz", "xy"} {
		if !modeUsesGradientSource(m) {
			t.Errorf("%s colours geometry, so SRC is a real choice in it", m)
		}
	}
}

// TestEveryMapPositionMapsSomething checks the map ring holds only mappings.
//
// The ring used to carry mono, which discarded the value — the one position
// that contradicted the knob's own stated function. Every position left has to
// turn a 0..1 value into a colour and turn DIFFERENT values into DIFFERENT
// colours, or it is mono again under another name.
//
// Sampled across the range rather than at the two ends, because the hue sweep's
// ends legitimately meet: hue is a circle, so 0 and 1 are the same red. That is
// the same fact the shader's fold relies on — a colormap is a line whose ends
// are different colours and needs reflecting, a hue circle does not — and it is
// not the position failing to map anything.
func TestEveryMapPositionMapsSomething(t *testing.T) {
	withColorKnobs(t)
	gradientSource = 2
	rgb := func(v float64) [3]uint32 {
		r, g, b, _ := mapColorAt(v).RGBA()
		return [3]uint32{r, g, b}
	}
	for _, m := range []int{2, 3, 4, 5, 6, 7, 8, 9, 10} {
		gradientColors = m
		seen := map[[3]uint32]bool{}
		for _, v := range []float64{0, 0.2, 0.4, 0.6, 0.8, 1} {
			seen[rgb(v)] = true
		}
		if len(seen) < 3 {
			t.Errorf("map %d paints six values in only %d colours", m, len(seen))
		}
		// And it stays inside the map past either end rather than wrapping to
		// the far one, which would put a seam in the middle of a figure.
		if rgb(-5) != rgb(0) {
			t.Errorf("map %d does not clamp below 0", m)
		}
		if rgb(5) != rgb(1) {
			t.Errorf("map %d does not clamp above 1", m)
		}
	}
}
