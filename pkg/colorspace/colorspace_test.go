package colorspace

import (
	"math"
	"testing"
)

func near(a, b [3]float32) bool {
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-5 {
			return false
		}
	}
	return true
}

// A turn of hue passes through the six primaries and secondaries at sixths.
// Counting in degrees instead is the bug this package exists to rule out: 1/3
// of a turn read as 1/3 of a degree is red, not green.
func TestHueIsInTurns(t *testing.T) {
	for _, c := range []struct {
		h    float32
		want [3]float32
	}{
		{0, [3]float32{1, 0, 0}},
		{1.0 / 6, [3]float32{1, 1, 0}},
		{2.0 / 6, [3]float32{0, 1, 0}},
		{3.0 / 6, [3]float32{0, 1, 1}},
		{4.0 / 6, [3]float32{0, 0, 1}},
		{5.0 / 6, [3]float32{1, 0, 1}},
	} {
		if got := FromHSV(c.h, 1, 1); !near(got, c.want) {
			t.Errorf("FromHSV(%.3f, 1, 1) = %v, want %v", c.h, got, c.want)
		}
	}
}

func TestHueWraps(t *testing.T) {
	for _, h := range []float32{0.2, 0.7} {
		if a, b := FromHSV(h, 0.6, 0.8), FromHSV(h+3, 0.6, 0.8); !near(a, b) {
			t.Errorf("hue %.1f and %.1f differ: %v, %v", h, h+3, a, b)
		}
	}
}

func TestToHSVInvertsFromHSV(t *testing.T) {
	for h := float32(0); h < 1; h += 0.05 {
		for _, s := range []float32{0.25, 1} {
			for _, v := range []float32{0.3, 1} {
				gh, gs, gv := ToHSV(FromHSV(h, s, v))
				if math.Abs(float64(gh-h)) > 1e-4 || math.Abs(float64(gs-s)) > 1e-4 || math.Abs(float64(gv-v)) > 1e-4 {
					t.Errorf("ToHSV(FromHSV(%.2f, %.2f, %.2f)) = %.4f, %.4f, %.4f", h, s, v, gh, gs, gv)
				}
			}
		}
	}
	if h, s, v := ToHSV([3]float32{0.5, 0.5, 0.5}); h != 0 || s != 0 || v != 0.5 {
		t.Errorf("a gray = %v, %v, %v, want hue 0, saturation 0, value 0.5", h, s, v)
	}
}

func TestHexRoundTrips(t *testing.T) {
	for _, s := range []string{"#000000", "#ffffff", "#f2b84b", "#2f6f8f", "#0a0b0c"} {
		if got := Hex(ParseHex(s)); got != s {
			t.Errorf("Hex(ParseHex(%q)) = %q", s, got)
		}
	}
	if got := ParseHex(""); got != [3]float32{1, 1, 1} {
		t.Errorf("an unset swatch = %v, want white", got)
	}
	if got := Hex([3]float32{-1, 0.5, 2}); got != "#0080ff" {
		t.Errorf("Hex clamps: got %q, want #0080ff", got)
	}
}
