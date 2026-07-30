package attractor

import (
	"strings"
	"testing"
)

// ledDecimals regression table — includes the two shipped bugs its own
// comment records: float32 round-off inflating the count (0.1 fine-step on a
// 0.1 coarse step must be 2 places, not 10) and the absolute-tolerance bug
// that made tiny steps (Aizawa's dt) show 0 places.
func TestLEDDecimals(t *testing.T) {
	defer func(f float64) { fineRatio = f }(fineRatio)
	fineRatio = 0.1
	cases := []struct {
		step float64
		want int
	}{
		{1, 1},      // fine 0.1
		{0.1, 2},    // fine 0.01 — float32 round-off must not inflate
		{0.05, 3},   // fine 0.005
		{0.01, 3},   // fine 0.001
		{0.0002, 5}, // Aizawa-dt class: tiny steps must NOT collapse to 0
		{1000, 0},   // trail-sized steps: integer display
		{0, 0},
	}
	for _, c := range cases {
		if got := ledDecimals(c.step); got != c.want {
			t.Errorf("ledDecimals(%v) with fine 0.1 = %d, want %d", c.step, got, c.want)
		}
	}
}

// formatLED fixed-width contract: zero-padded integer part, fixed decimals,
// sign slot when signed — the LED-clipping class of bug ("only goes to 2048"
// was a 5-digit value in a 4-digit habit).
func TestFormatLEDWidth(t *testing.T) {
	cases := []struct {
		v      float64
		intDig int
		dec    int
		signed bool
		want   string
	}{
		{5, 1, 1, true, "+5.0"},
		{-3, 1, 1, true, "-3.0"},
		{20, 2, 1, true, "+20.0"},
		{0, 2, 1, true, "+00.0"},
		{20480, 5, 1, false, "20480.0"},
		{110, 5, 1, false, "00110.0"},
		{2.5, 2, 3, false, "02.500"},
		{20000, 6, 0, false, "020000"},
	}
	for _, c := range cases {
		if got := formatLED(c.v, c.intDig, c.dec, c.signed); got != c.want {
			t.Errorf("formatLED(%v,%d,%d,%v) = %q, want %q", c.v, c.intDig, c.dec, c.signed, got, c.want)
		}
	}
}

func TestLEDIntDigits(t *testing.T) {
	cases := []struct {
		min, max float64
		want     int
	}{
		{-95, 95, 2},
		{1000, 500000, 6},
		{27.5, 28160, 5},
		{0.01, 100, 3},
		{-8, 8, 1},
		{0, 0, 1}, // floor of one digit
	}
	for _, c := range cases {
		if got := ledIntDigits(c.min, c.max); got != c.want {
			t.Errorf("ledIntDigits(%v,%v) = %d, want %d", c.min, c.max, got, c.want)
		}
	}
}

// formatParamValue trims trailing zeros so coarse values read cleanly but
// keeps fine-knob precision visible.
func TestFormatParamValue(t *testing.T) {
	if got := formatParamValue(28, 0); got != "28" {
		t.Errorf("formatParamValue(28,0) = %q, want 28", got)
	}
	if got := formatParamValue(2.07, 2); got != "2.07" {
		t.Errorf("formatParamValue(2.07,2) = %q", got)
	}
	if got := formatParamValue(28.00000, 2); strings.Contains(got, ".00000") {
		t.Errorf("formatParamValue must trim trailing zeros, got %q", got)
	}
}
