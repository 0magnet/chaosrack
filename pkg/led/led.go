// Package led decides what a seven-segment readout shows: how many digits,
// how many decimals, and where the sign sits.
//
// It is untagged so it tests natively. The panel only puts the strings on
// the page; everything that chooses them is here, and this is where the real
// bugs were — LEDs clipping a five-digit value, and decimal counts inflated by
// float32 round-off or collapsed to zero for a tiny step.
package led

import (
	"math"
	"strconv"
	"strings"
)

// Digits is the number of integer digits in |v|, at least 1.
func Digits(v float64) int {
	v = math.Abs(v)
	if v < 1 {
		return 1
	}
	return int(math.Floor(math.Log10(v))) + 1
}

// IntDigits is the number of integer digits the widest value in [min, max]
// needs, at least 1.
func IntDigits(lo, hi float64) int {
	d := Digits(hi)
	if x := Digits(lo); x > d {
		d = x
	}
	return d
}

// Decimals is the number of fractional digits an LED shows for a control
// whose coarse step is step: the finest the fine knob resolves, step×fine, so
// the display's precision matches the smallest possible adjustment.
func Decimals(step, fine float64) int {
	fs := math.Abs(step * fine)
	if fs == 0 {
		return 0
	}
	// Smallest d for which fs rounds cleanly to d places — tolerant of the
	// float32 round-off in step (~1e-7 relative), so 0.1*0.1 counts as 2 places,
	// not 10, and 1000*0.1 counts as 0. The tolerance is RELATIVE (with a small
	// floor) so a tiny step like 1e-5 isn't mistaken for "0 places" — a fixed
	// 1e-4 absolute tolerance made small-step params (e.g. Aizawa's dt) show 0.
	for d := range 9 {
		scaled := fs * math.Pow(10, float64(d))
		// And at least one digit: a step below the tolerance's floor rounds
		// "cleanly" to zero places, which is no places at all.
		if r := math.Round(scaled); r != 0 && math.Abs(scaled-r) <= 1e-6*math.Max(1, scaled) {
			return d
		}
	}
	return 8
}

// StepDecimals is how many decimal places a step itself implies. It uses the
// shortest round-tripping form of the float32, so single-precision noise
// (0.01 stored as 0.00999999977…) doesn't inflate the count.
func StepDecimals(step float32) int {
	s := strconv.FormatFloat(float64(step), 'g', -1, 32)
	if e := strings.IndexAny(s, "eE"); e >= 0 {
		mant := s[:e]
		exp, _ := strconv.Atoi(s[e+1:]) //nolint:errcheck // FormatFloat wrote it; it parses
		d := 0
		if dot := strings.IndexByte(mant, '.'); dot >= 0 {
			d = len(mant) - dot - 1
		}
		d -= exp // a negative exponent adds decimal places
		if d < 0 {
			d = 0
		}
		return d
	}
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return 0
	}
	return len(s) - dot - 1
}

// Format renders val for a readout: the sign in a FIXED leftmost slot (Blank
// or -)
// and the integer part zero-padded to intDig, so neither digits nor sign
// shift as the value changes, as on a real LED counter. dec fixes the
// fraction width. intDig should come from IntDigits.
func Format(val float64, intDig, dec int, signed bool) string {
	s := strconv.FormatFloat(math.Abs(val), 'f', dec, 64)
	ip, fp := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		ip, fp = s[:dot], s[dot:]
	}
	for len(ip) < intDig {
		ip = "0" + ip
	}
	s = ip + fp
	if signed {
		if val < 0 {
			return "-" + s
		}
		return Blank + s
	}
	return s
}

// Blank is the sign slot of a positive value: DSEG7's blank digit, as wide as
// a digit with no segment lit. A seven-segment display has no plus, and the
// font has none either; asked for one, the browser drew it from the fallback
// face, whose line metrics are not DSEG7's, and the digits jumped a pixel
// whenever the value crossed zero.
const Blank = "!"

// Parse reads a readout's text as a number, its Blank sign slot included.
func Parse(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(strings.Replace(s, Blank, "", 1)), 64)
}
