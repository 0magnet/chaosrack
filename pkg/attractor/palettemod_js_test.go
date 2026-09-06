//go:build js && wasm

package attractor

import (
	"math"
	"testing"
)

// The defaults have to be the picture that was already on the screen. The
// colormaps shipped sampling clamp(t,0,1) directly; if the window's identity
// setting were off by anything at all, every existing view would shift color
// the moment this was added, and nobody would connect the change to a knob
// they never touched.
func TestPaletteWindowDefaultsToTheOldCoordinate(t *testing.T) {
	for i := 0; i <= 256; i++ {
		v := float32(i) / 256
		if got := paletteCoord(v, 1, 0); math.Abs(float64(got-v)) > 1e-6 {
			t.Fatalf("paletteCoord(%v, 1, 0) = %v; span 1 shift 0 must be the identity", v, got)
		}
	}
}

// The coordinate is a texture lookup, so it must land on the map for every
// input the knobs and the modulation can produce. Off the map is not merely
// wrong, it is invisible: the sampler clamps and the figure quietly goes flat
// at one end instead of reporting anything.
func TestPaletteCoordStaysOnTheMap(t *testing.T) {
	for _, span := range []float32{0.05, 0.2, 1, 4, 20} {
		for _, shift := range []float32{-1, -0.37, 0, 0.37, 1, 7.5, -7.5} {
			for i := 0; i <= 32; i++ {
				v := float32(i) / 32
				got := paletteCoord(v, span, shift)
				if got < 0 || got > 1 {
					t.Fatalf("paletteCoord(%v, %v, %v) = %v, off the 0..1 map", v, span, shift, got)
				}
			}
		}
	}
}

// The whole argument for reflecting rather than wrapping. A wrap would put a
// step discontinuity in the color field — turbo's two ends are different
// colors — which draws as a hard seam across the figure and, under
// modulation, a seam that MOVES. The fold has to be continuous everywhere,
// including exactly at the turn, so the test walks straight through several
// folds and holds the step down to what the window itself justifies.
func TestPaletteCoordHasNoSeam(t *testing.T) {
	const span, step = 1, 1.0 / 4096
	prev := paletteCoord(0.5, span, -3)
	for x := -3.0; x <= 3.0; x += step {
		got := paletteCoord(0.5, span, float32(x))
		if d := math.Abs(float64(got - prev)); d > 4*step {
			t.Fatalf("the coordinate jumped %v between shift %v and the step before it; a fold must not step", d, x)
		}
		prev = got
	}
}

// The knob runs −1..1 because that is exactly one period of the fold, which
// is what makes clamping the modulated value unnecessary and wrapping it
// invisible. If the two ever disagreed, a shift driven past the end would
// either jump colors or stall.
func TestTheFoldRepeatsOverTheKnobRange(t *testing.T) {
	for _, span := range []float32{0.2, 1, 3} {
		for i := 0; i <= 16; i++ {
			v := float32(i) / 16
			for _, shift := range []float32{-0.9, -0.25, 0, 0.25, 0.9} {
				a := paletteCoord(v, span, shift)
				b := paletteCoord(v, span, shift+paletteFoldPeriod)
				c := paletteCoord(v, span, shift-paletteFoldPeriod)
				if math.Abs(float64(a-b)) > 1e-5 || math.Abs(float64(a-c)) > 1e-5 {
					t.Fatalf("shift %v gave %v but %v/%v a period away; the range is not one period", shift, a, b, c)
				}
			}
		}
	}
}

// Clamping the modulated shift would cost the sweep; the range is only
// harmless because its ends are not dead. At either end of the knob the map
// comes back exactly REVERSED — a different, fully usable picture — rather
// than the flat single color a clamp against the colormap would have given.
func TestTheEndsOfTheKnobAreTheReversedMap(t *testing.T) {
	for i := 0; i <= 32; i++ {
		v := float32(i) / 32
		for _, shift := range []float32{-1, 1} {
			got := paletteCoord(v, 1, shift)
			if math.Abs(float64(got-(1-v))) > 1e-6 {
				t.Errorf("paletteCoord(%v, 1, %v) = %v, want the reversed map %v", v, shift, got, 1-v)
			}
		}
	}
}

// The wrap is what applyViewModulation uses in place of its clamp, so it has
// to bring back anything the modulation can hand it — level ±4 against a
// range of 2 reaches ±8 before the base value is even counted.
func TestWrapPaletteShiftBringsAnythingBack(t *testing.T) {
	for _, x := range []float32{-9, -8, -2.5, -1, -0.5, 0, 0.5, 1, 2.5, 8, 9} {
		w := wrapPaletteShift(x)
		if w < -1 || w >= 1 {
			t.Errorf("wrapPaletteShift(%v) = %v, outside [−1, 1)", x, w)
		}
		if d := math.Mod(math.Abs(float64(x-w)), paletteFoldPeriod); d > 1e-5 && math.Abs(d-paletteFoldPeriod) > 1e-5 {
			t.Errorf("wrapPaletteShift(%v) = %v moved it by %v, not a whole number of periods", x, w, x-w)
		}
	}
}

// The Go arithmetic is only worth testing if it is the SAME arithmetic the
// shader runs. The fragment stage computes abs(pt - 2.0*floor(pt*0.5 + 0.5))
// and no test can execute GLSL, so the literal expression is spelled out here
// and required to agree — if the shader is ever rewritten into a form that
// differs, this is what says so.
func TestFoldMatchesTheShaderExpression(t *testing.T) {
	glsl := func(pt float64) float64 { return math.Abs(pt - 2.0*math.Floor(pt*0.5+0.5)) }
	for x := -6.0; x <= 6.0; x += 1.0 / 512 {
		if got, want := float64(foldPalette01(float32(x))), glsl(x); math.Abs(got-want) > 1e-5 {
			t.Fatalf("foldPalette01(%v) = %v, the shader's expression gives %v", x, got, want)
		}
	}
}

// A narrow window is the setting a sweep is actually used at, so it has to
// behave: the figure must occupy a slice the size of the period, and moving
// the shift must move that slice rather than resize it.
func TestANarrowWindowSlidesWithoutResizing(t *testing.T) {
	const span = 0.2
	for _, shift := range []float32{0, 0.15, 0.3, 0.45, 0.6} {
		lo, hi := float32(1), float32(0)
		for i := 0; i <= 64; i++ {
			c := paletteCoord(float32(i)/64, span, shift)
			if c < lo {
				lo = c
			}
			if c > hi {
				hi = c
			}
		}
		if d := math.Abs(float64(hi - lo - span)); d > 1e-5 {
			t.Errorf("shift %v covered %v..%v, a span of %v; the period says %v", shift, lo, hi, hi-lo, span)
		}
		if math.Abs(float64(lo-shift)) > 1e-5 {
			t.Errorf("shift %v put the window at %v; it should start where the shift says", shift, lo)
		}
	}
}
