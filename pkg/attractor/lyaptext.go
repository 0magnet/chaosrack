package attractor

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/analysis"
)

// The Lyapunov readout's text, shared by the page (analysis_js.go) and a
// native rack.

// LyapunovTexts is a measurement as its two displays show it: the exponent
// with its unit, and the verdict.
func LyapunovTexts(r analysis.LyapunovResult) (val, verdict string) {
	if r.Verdict == "n/a" {
		// Not a dynamical system. Saying so is the honest readout; printing
		// 0.0000 beside a dodecahedron would be a category error with a
		// decimal point.
		return "--.--", "n/a"
	}
	if !r.OK {
		return "--.--", verdictText(r.Verdict)
	}
	unit := "/t"
	if r.PerStep {
		unit = "/n" // per iterate: a map has no dt, so per-time is meaningless
	}
	return formatLyap(r.Lambda) + unit, verdictText(r.Verdict)
}

// formatLyap renders the exponent signed, to three decimals, or two past ten,
// so with its unit ("/t", "/n") it fills a full display and no more.
func formatLyap(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "--.--"
	}
	dec := 3
	if math.Abs(v) >= 10 {
		dec = 2
	}
	s := strconv.FormatFloat(v, 'f', dec, 64)
	if v >= 0 {
		s = "+" + s
	}
	return s
}

// verdictText is a verdict as its display shows it: eight characters, so
// "converging" says what the exponent says of it, that the orbit is settling.
func verdictText(v string) string {
	if v == "converging" {
		return "settling"
	}
	return v
}
