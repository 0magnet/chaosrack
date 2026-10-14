package attractor

import (
	"math"
	"strconv"
)

// The Stereo Embedding and the XY scope's correlation readout, shared by the
// page and a native rack.

// stereoCorrelation is the Pearson correlation of the two channels over the
// window — the number a goniometer's correlation meter shows. +1 is mono
// (the diagonal), 0 is uncorrelated (a round cloud), −1 is a polarity flip
// (the other diagonal, and the thing that vanishes when someone sums the mix
// to mono).
//
// The means are subtracted rather than assumed zero. Audio is nominally
// AC-coupled and in practice is not: a cheap ADC's DC offset, or a window
// short enough to sit inside one cycle of something very low, both move the
// mean, and a raw Σlr/√(Σl²Σr²) then reports the offset's correlation instead
// of the signal's — which is +1 for any two channels that happen to share a
// rail.
//
// ok is false when there is nothing to correlate: silence, or one dead
// channel, where the denominator is zero and any answer would be invented. The
// floor is a variance of 1e-9, an RMS around −90 dBFS, which is below the
// noise floor of anything real and above exact digital zero.
func stereoCorrelation(l, r []float32) (float32, bool) {
	n := len(l)
	if n != len(r) || n < 2 {
		return 0, false
	}
	var sl, sr float64
	for i := range n {
		sl += float64(l[i])
		sr += float64(r[i])
	}
	ml, mr := sl/float64(n), sr/float64(n)
	var sll, srr, slr float64
	for i := range n {
		a := float64(l[i]) - ml
		b := float64(r[i]) - mr
		sll += a * a
		srr += b * b
		slr += a * b
	}
	fn := float64(n)
	if sll/fn < 1e-9 || srr/fn < 1e-9 {
		return 0, false
	}
	c := slr / math.Sqrt(sll*srr)
	// Rounding can put an exactly-identical pair a hair outside the range, and
	// a readout of "r+1.00" that came from 1.0000000002 is fine while a
	// downstream acos of it would not be.
	if c > 1 {
		c = 1
	} else if c < -1 {
		c = -1
	}
	return float32(c), true
}

// stereoReadout is the readout text for a measured state: six characters at
// most, the size of its display (appendReadout).
// matching the Takens mode's "τ32 m4" — the cell is a third of a module wide.
func stereoReadout(monoSrc, ok bool, corr float32) string {
	if monoSrc {
		return "mono"
	}
	if !ok {
		return "r --"
	}
	sign := "+"
	if corr < 0 {
		sign = "-"
		corr = -corr
	}
	return "r" + sign + strconv.FormatFloat(float64(corr), 'f', 2, 32)
}

// StereoReadout is the readout for the pair l and r: their correlation, or
// dashes when there is nothing to correlate.
func StereoReadout(l, r []float32) string {
	corr, ok := stereoCorrelation(l, r)
	return stereoReadout(false, ok, corr)
}
