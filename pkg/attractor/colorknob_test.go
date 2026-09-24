package attractor

import (
	"math"
	"testing"
)

// The hue ring counts degrees and colorspace counts turns; a slip between the
// two is invisible in the code and obvious on the panel, so pin the ring's
// landmarks to the colors they must be.
func TestTheHueRingIsInDegrees(t *testing.T) {
	for _, c := range []struct {
		h    float64
		want string
	}{
		{0, "#ff0000"}, {60, "#ffff00"}, {120, "#00ff00"}, {180, "#00ffff"},
		{240, "#0000ff"}, {300, "#ff00ff"},
		{360, "#ff0000"}, // the slider's top end is its bottom end
	} {
		if got := knobHex(c.h, 1, 1); got != c.want {
			t.Errorf("knobHex(%v°) = %s, want %s", c.h, got, c.want)
		}
	}
	if h, s, v := knobHSV("#00ffff"); h != 180 || s != 1 || v != 1 {
		t.Errorf("knobHSV(#00ffff) = %v°, %v, %v, want 180°, 1, 1", h, s, v)
	}
}

// Picking a swatch turns the rings to match. For a color the rings dialed, they
// must come back to where they were, give or take the step a hex byte is too
// coarse to hold; a ring that jumped further would be reading the swatch in
// the wrong units. Near black and white a hex byte cannot hold a hue at all
// (level 1 is five steps of 255), so the check keeps to the middle.
func TestPickingASwatchReturnsTheRingsToIt(t *testing.T) {
	for h := 0; h < 360; h += 7 {
		for l := 25; l <= 75; l += 2 {
			s, v := levelToSV(float64(l))
			gh, gs, gv := knobHSV(knobHex(float64(h), s, v))
			dh := math.Abs(math.Round(gh) - float64(h))
			dh = math.Min(dh, 360-dh)
			dl := math.Abs(math.Round(svToLevel(gs, gv)) - float64(l))
			if dh > 1 || dl > 1 {
				t.Errorf("hue %d° level %d came back as %.2f°, level %.2f", h, l, gh, svToLevel(gs, gv))
			}
		}
	}
}
