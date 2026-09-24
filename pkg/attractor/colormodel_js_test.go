//go:build js && wasm

package attractor

import (
	"syscall/js"
	"testing"
)

// The color model is two knobs: SRC says what the color follows, MAP says how
// that value becomes a color. These check the two places the split has to hold
// together — the uniform the geometry reads, and which modes consult SRC at all.

func withColorKnobs(t *testing.T) {
	t.Helper()
	s, c, m := style.gradientSource, style.gradientColors, run.selectedMode
	t.Cleanup(func() { style.gradientSource, style.gradientColors, run.selectedMode = s, c, m })
}

// TestSourceOffOnlySilencesTheGeometry is the load-bearing asymmetry.
//
// OFF means "the color follows nothing", which is a thing you can say about a
// figure whose color is a CHOICE — an attractor, an embedding, the waterfall.
// The spectrogram, the RTA and the transfer function each color one quantity
// of their own and never consulted the src ring, so OFF must not silence them:
// they go on reading the map ring. That is why gradientColors stays the map and
// only the shader's uniform folds OFF in.
func TestSourceOffOnlySilencesTheGeometry(t *testing.T) {
	withColorKnobs(t)
	style.gradientColors = 9 // viridis

	style.gradientSource = 2 // Z
	if got := gradientColorsUniform(); got != 9 {
		t.Errorf("source Z: uniform %d, want the map ring's 9", got)
	}
	style.gradientSource = GradientSourceOff
	if got := gradientColorsUniform(); got != 1 {
		t.Errorf("source OFF: uniform %d, want the shader's monochrome 1", got)
	}
	// The map ring itself does not move, which is what the analyzers and the
	// spectrogram read.
	if style.gradientColors != 9 {
		t.Errorf("source OFF moved the map ring to %d", style.gradientColors)
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
			t.Errorf("%s colors geometry, so SRC is a real choice in it", m)
		}
	}
}

// TestEveryMapPositionMapsSomething checks the map ring holds only mappings.
//
// The ring used to carry mono, which discarded the value — the one position
// that contradicted the knob's own stated function. Every position left has to
// turn a 0..1 value into a color and turn DIFFERENT values into DIFFERENT
// colors, or it is mono again under another name.
//
// Sampled across the range rather than at the two ends, because the hue sweep's
// ends legitimately meet: hue is a circle, so 0 and 1 are the same red. That is
// the same fact the shader's fold relies on — a colormap is a line whose ends
// are different colors and needs reflecting, a hue circle does not — and it is
// not the position failing to map anything.
func TestEveryMapPositionMapsSomething(t *testing.T) {
	withColorKnobs(t)
	style.gradientSource = 2
	rgb := func(v float64) [3]uint32 {
		r, g, b, _ := mapColorAt(v).RGBA()
		return [3]uint32{r, g, b}
	}
	for _, m := range []int{2, 3, 4, 5, 6, 7, 8, 9, 10} {
		style.gradientColors = m
		seen := map[[3]uint32]bool{}
		for _, v := range []float64{0, 0.2, 0.4, 0.6, 0.8, 1} {
			seen[rgb(v)] = true
		}
		if len(seen) < 3 {
			t.Errorf("map %d paints six values in only %d colors", m, len(seen))
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

// TestResetHandlesBothControlVariants is the regression for a runtime that died
// on startup.
//
// Control grew a second variant — a selector-backed control, whose value lives
// in a <select> and whose slider field is therefore the zero js.Value. Run
// primes every registered control by dispatching an event at c.slider, and Call
// on an undefined js.Value is a PANIC, not a no-op: the first selector to reach
// that loop took the whole Go runtime down a moment after startup, leaving a
// panel whose every knob was dead and whose LEDs were frozen at the values they
// were given while the program was still alive.
//
// So: every path that touches a Control must branch on which element holds the
// value, and must survive a Control holding neither.
func TestResetHandlesBothControlVariants(t *testing.T) {
	newEl := func(v string) js.Value {
		el := js.Global().Get("Object").New()
		el.Set("value", v)
		el.Set("dispatchEvent", js.FuncOf(func(js.Value, []js.Value) any { return nil }))
		return el
	}

	// Selector-backed: no slider at all. This is the shape that crashed.
	sel := newEl("turbo")
	c := &Control{sel: sel, selDef: "viridis"}
	c.resetToDefault()
	if got := sel.Get("value").String(); got != "viridis" {
		t.Errorf("selector reset left %q, want the default %q", got, "viridis")
	}

	// Slider-backed: no sel. The original shape, which must still work.
	sl := newEl("7")
	c = &Control{slider: sl, def: 3}
	c.resetToDefault()
	if got := sl.Get("value").String(); got != "3" {
		t.Errorf("slider reset left %q, want the default %q", got, "3")
	}

	// Neither: a no-op, not a panic. Nothing should ever build one, which is
	// exactly why it must not be the thing that takes the runtime down.
	(&Control{}).resetToDefault()
}
