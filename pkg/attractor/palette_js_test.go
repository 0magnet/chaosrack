//go:build js && wasm

package attractor

import "testing"

// The trace palettes and the spectrogram's must stay the same six, in the same
// order. The whole value of reusing them is that "the third one" means the same
// thing on both knobs; if one list grows and the other does not, the knobs
// silently disagree and the colors stop being comparable between the displays.
func TestPaletteListMatchesTheSpectrogramsList(t *testing.T) {
	if len(paletteFns) != len(spectColNames) {
		t.Fatalf("%d trace colormaps against %d spectrogram ones: %v", len(paletteFns), len(spectColNames), spectColNames)
	}
}

// paletteIndex is what the shader branch and the texture build both key off,
// so its boundaries have to be exact: one below the first map must not resolve,
// and one past the last must not either.
func TestPaletteIndexBoundaries(t *testing.T) {
	for _, gc := range []int{0, 1, 2, 3, 4} {
		if _, ok := paletteIndex(gc); ok {
			t.Errorf("uGradientColors %d resolved to a colormap; 1..4 are the mix palettes", gc)
		}
	}
	for i := range paletteFns {
		gc := paletteFirst + i
		got, ok := paletteIndex(gc)
		if !ok || got != i {
			t.Errorf("uGradientColors %d gave (%d,%v), want (%d,true)", gc, got, ok, i)
		}
	}
	if _, ok := paletteIndex(paletteFirst + len(paletteFns)); ok {
		t.Error("one past the last colormap resolved; the texture build would index out of range")
	}
}

// Every colormap has to actually vary across its range. A constant one is not
// a colormap, and it fails silently — the figure just comes out one color,
// which looks like the gradient being broken rather than the palette.
func TestEveryColormapVariesAcrossItsRange(t *testing.T) {
	for i, fn := range paletteFns {
		lo, hi := fn(0), fn(1)
		lr, lg, lb, _ := lo.RGBA()
		hr, hg, hb, _ := hi.RGBA()
		if lr == hr && lg == hg && lb == hb {
			t.Errorf("colormap %d (%s) gives the same color at 0 and 1", i, spectColNames[i])
		}
	}
}

// Out-of-range input must clamp. Not every colormap in the library does it
// for itself — ValueToPixelGrayscale is uint8(255.0*value) with no guard, so
// 2.0 converts to a uint8 that is darker than the color at 1.0 rather than
// equal to it — so the wrapper this file builds the texture through has to.
func TestPaletteColorAtClampsAtTheEnds(t *testing.T) {
	for i := range paletteFns {
		ur, ug, ub, _ := paletteColorAt(i, -1).RGBA()
		lr, lg, lb, _ := paletteColorAt(i, 0).RGBA()
		or, og, ob, _ := paletteColorAt(i, 2).RGBA()
		hr, hg, hb, _ := paletteColorAt(i, 1).RGBA()
		if ur != lr || ug != lg || ub != lb {
			t.Errorf("colormap %d (%s) at -1 is not its color at 0", i, spectColNames[i])
		}
		if or != hr || og != hg || ob != hb {
			t.Errorf("colormap %d (%s) at 2 is not its color at 1", i, spectColNames[i])
		}
	}
}

// The knob is a point COUNT, and its default has to be the behavior that
// existed before it: a solid line. Anything else and every existing view
// silently gains gaps.
func TestPointCountDefaultsToASolidLine(t *testing.T) {
	if pointCount != 0 {
		t.Errorf("pointCount defaults to %v; 0 means as many points as vertices, i.e. solid", pointCount)
	}
	updateDashFromPointCount(5000)
	if dashDuty != 1 || dashCount != 1 {
		t.Errorf("the default derived duty/count %v/%v; want 1/1, the solid line", dashDuty, dashCount)
	}
}

// Continuity is arithmetic here, not a special case: one vertex per point
// means asking for as many points as there are vertices draws every one of
// them. If that ever stopped landing exactly on solid, the top of the knob
// would be a nearly-solid line with a visible seam.
func TestAPointForEveryVertexIsTheSolidLine(t *testing.T) {
	for _, n := range []int{2, 100, 4000, 20000} {
		pointCount = float32(n)
		updateDashFromPointCount(n)
		if dashDuty != 1 || dashCount != 1 {
			t.Errorf("%d points over %d vertices gave duty %v count %v; want the solid line", n, n, dashDuty, dashCount)
		}
	}
	pointCount = 0
}

// Fewer points must mean sparser ones. This is the whole content of the knob:
// the drawn fraction has to fall as the count does, or turning it down thins
// the trace instead of breaking it into points.
func TestFewerPointsDrawLessOfTheTrail(t *testing.T) {
	const drawn = 4000
	var prev float32 = 2
	for _, n := range []int{2000, 1000, 400, 100, 20} {
		pointCount = float32(n)
		updateDashFromPointCount(drawn)
		if dashDuty >= prev {
			t.Errorf("%d points drew duty %v, not less than the %v before it", n, dashDuty, prev)
		}
		if dashCount != float32(n) {
			t.Errorf("%d points gave %v dash cycles; the count IS the number of points", n, dashCount)
		}
		prev = dashDuty
	}
	pointCount = 0
}

// Degenerate inputs must fall back to solid rather than to a division by zero
// or a duty that makes the trace vanish. An empty frame is ordinary — the
// trail is empty before the first data arrives.
func TestPointCountDegenerateInputsStaySolid(t *testing.T) {
	for _, c := range []struct {
		pts   float32
		drawn int
	}{{0, 0}, {100, 0}, {100, -5}, {-1, 4000}, {0.5, 4000}} {
		pointCount = c.pts
		updateDashFromPointCount(c.drawn)
		if dashDuty != 1 || dashCount != 1 {
			t.Errorf("pointCount %v over %d vertices gave duty %v count %v; want solid", c.pts, c.drawn, dashDuty, dashCount)
		}
	}
	pointCount = 0
}
