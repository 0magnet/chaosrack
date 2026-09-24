package colormap

import (
	"image/color"
	"testing"
)

// Index is what the shader branch and the texture build both key off,
// so its boundaries have to be exact: one below the first map must not resolve,
// and one past the last must not either.
func TestPaletteIndexBoundaries(t *testing.T) {
	for _, gc := range []int{0, 1, 2, 3, 4} {
		if _, ok := Index(gc); ok {
			t.Errorf("uGradientColors %d resolved to a colormap; 1..4 are the mix palettes", gc)
		}
	}
	for i := range maps {
		gc := First + i
		got, ok := Index(gc)
		if !ok || got != i {
			t.Errorf("uGradientColors %d gave (%d,%v), want (%d,true)", gc, got, ok, i)
		}
	}
	if _, ok := Index(First + len(maps)); ok {
		t.Error("one past the last colormap resolved; the texture build would index out of range")
	}
}

// Every colormap has to actually vary across its range. A constant one is not
// a colormap, and it fails silently — the figure just comes out one color,
// which looks like the gradient being broken rather than the palette.
func TestEveryColormapVariesAcrossItsRange(t *testing.T) {
	for i, fn := range maps {
		lo, hi := fn(0), fn(1)
		lr, lg, lb, _ := lo.RGBA()
		hr, hg, hb, _ := hi.RGBA()
		if lr == hr && lg == hg && lb == hb {
			t.Errorf("colormap %d gives the same color at 0 and 1", i)
		}
	}
}

// Out-of-range input must clamp. Not every colormap in the library does it
// for itself — ValueToPixelGrayscale is uint8(255.0*value) with no guard, so
// 2.0 converts to a uint8 that is darker than the color at 1.0 rather than
// equal to it — so the wrapper this file builds the texture through has to.
func TestPaletteColorAtClampsAtTheEnds(t *testing.T) {
	for i := range maps {
		ur, ug, ub, _ := At(i, -1).RGBA()
		lr, lg, lb, _ := At(i, 0).RGBA()
		or, og, ob, _ := At(i, 2).RGBA()
		hr, hg, hb, _ := At(i, 1).RGBA()
		if ur != lr || ug != lg || ub != lb {
			t.Errorf("colormap %d at -1 is not its color at 0", i)
		}
		if or != hr || og != hg || ob != hb {
			t.Errorf("colormap %d at 2 is not its color at 1", i)
		}
	}
}

// The hue position of the map ring sweeps the spectrum on the spectrogram as
// it does on the trace. It painted the spectrogram solid red while the trace
// was a rainbow: this took its hue from a 0..1 value and handed it to an HSV
// conversion that counted in degrees, so every value landed in the first
// sixtieth of the red sector.
func TestTheHueMapSweepsTheSpectrumOnTheSpectrogram(t *testing.T) {
	m := Map{Colors: 4, Freq: 1}
	for _, c := range []struct {
		v       float64
		r, g, b uint8
	}{
		{0, 255, 0, 0},
		{1.0 / 3, 0, 255, 0},
		{0.5, 0, 255, 255},
		{2.0 / 3, 0, 0, 255},
	} {
		got := color.RGBAModel.Convert(m.At(c.v)).(color.RGBA)
		near := func(a, b uint8) bool { return int(a)+2 >= int(b) && int(b)+2 >= int(a) }
		if !near(got.R, c.r) || !near(got.G, c.g) || !near(got.B, c.b) {
			t.Errorf("Map{hue}.At(%.3f) = %d,%d,%d, want %d,%d,%d", c.v, got.R, got.G, got.B, c.r, c.g, c.b)
		}
	}
}
