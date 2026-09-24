//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"testing"
)

// The trace palettes and the spectrogram's must stay the same six, in the same
// order. The whole value of reusing them is that "the third one" means the same
// thing on both knobs; if one list grows and the other does not, the knobs
// silently disagree and the colors stop being comparable between the displays.
func TestPaletteListMatchesTheSpectrogramsList(t *testing.T) {
	if colormap.Len() != len(spectColNames) {
		t.Fatalf("%d trace colormaps against %d spectrogram ones: %v", colormap.Len(), len(spectColNames), spectColNames)
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
