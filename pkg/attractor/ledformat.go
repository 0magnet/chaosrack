package attractor

// LED / numeric formatting — pure logic, deliberately UNTAGGED so it is
// unit-testable natively (see ledformat_test.go). The js/wasm layer only
// touches DOM (sizeLEDField, wheelNudge); everything that DECIDES digits,
// decimals, and rendering lives here. This module has shipped real bugs
// (LED clipping, decimal-count misderivation) that native table tests catch.

import (
	"math"
	"strconv"
	"strings"
)

// fineRatio is the Fine× multiplier (UI-set); it participates in decimal
// derivation (ledDecimals), so it lives with the pure formatting layer.
var fineRatio = 0.1

// decimalsForStep returns the number of decimal places needed to represent a step value.
// formatParamValue renders a param value for its numeric box. It shows up to
// three decimals beyond the coarse-step precision (dec) so a fine-knob nudge
// — which moves by coarseStep·fineRatio, fineRatio as small as 0.001 — is
// visible, then strips trailing zeros so coarse values still read cleanly
// (28, 2.07, 0.95 rather than 28.00000).
func formatParamValue(val float64, dec int) string {
	prec := dec + 3
	if prec > 8 {
		prec = 8
	}
	s := strconv.FormatFloat(val, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" || s == "-0" {
		s = "0"
	}
	return s
}

// decimalsForStep returns how many decimal places a step implies. It uses the
// shortest round-tripping representation of the float32 so single-precision
// noise (e.g. 0.01 stored as 0.00999999977…) doesn't inflate the count.
// intDigits returns the number of integer digits in |v| (min 1).
func intDigits(v float64) int {
	v = math.Abs(v)
	if v < 1 {
		return 1
	}
	return int(math.Floor(math.Log10(v))) + 1
}

// ledDecimals is the fixed number of fractional digits an LED field shows: the
// finest the fine knob resolves for a control of coarse `step` (step*fineRatio),
// so the displayed precision matches the smallest possible adjustment. Computed
// in float64 and trimmed (a fixed 10-place render then strip trailing zeros) so
// float32 round-off in step*fineRatio doesn't spuriously inflate the count.
func ledDecimals(step float64) int {
	fs := math.Abs(step * fineRatio)
	if fs == 0 {
		return 0
	}
	// Smallest d for which fs rounds cleanly to d places — tolerant of the
	// float32 round-off in step (~1e-7 relative), so 0.1*0.1 counts as 2 places,
	// not 10, and 1000*0.1 counts as 0. The tolerance is RELATIVE (with a small
	// floor) so a tiny step like 1e-5 isn't mistaken for "0 places" — a fixed
	// 1e-4 absolute tolerance made small-step params (e.g. Aizawa's dt) show 0.
	for d := 0; d <= 8; d++ {
		scaled := fs * math.Pow(10, float64(d))
		if math.Abs(scaled-math.Round(scaled)) <= 1e-6*math.Max(1, scaled) {
			return d
		}
	}
	return 8
}

// formatLED renders val with EXACTLY dec fractional digits (always the decimal
// point) and, for a signed field, a leading +/- — so on a right-aligned LED the
// digits never shift position as the value or its sign changes.
// ledIntDigits is the number of integer digits the widest value in [min,max]
// needs (at least 1).
func ledIntDigits(min, max float64) int {
	d := intDigits(max)
	if x := intDigits(min); x > d {
		d = x
	}
	if d < 1 {
		d = 1
	}
	return d
}

// formatLED renders a value for a 7-seg LED readout: the sign sits in a FIXED
// leftmost slot (+/-) and the integer part is zero-padded to intDig, so digits
// and the sign never shift as the value changes (like a real LED counter). dec
// fixes the fraction width. intDig should come from ledIntDigits(min,max).
func formatLED(val float64, intDig, dec int, signed bool) string {
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
		return "+" + s
	}
	return s
}

func decimalsForStep(step float32) int { //nolint:unused // built but not wired up yet; kept deliberately
	s := strconv.FormatFloat(float64(step), 'g', -1, 32)
	if e := strings.IndexAny(s, "eE"); e >= 0 {
		mant := s[:e]
		exp, _ := strconv.Atoi(s[e+1:]) //nolint:errcheck // a numeric DOM attribute; zero is the right fallback if it is ever not
		d := 0
		if dot := strings.IndexByte(mant, '.'); dot >= 0 {
			d = len(mant) - dot - 1
		}
		d -= exp // negative exponent increases the decimal places
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
