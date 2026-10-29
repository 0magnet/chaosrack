package colormap

import (
	"image/color"
	"math"
)

// Diverge is the one map here that is not the spectrogram library's: a
// diverging map, for a value that is SIGNED rather than an amount.
//
// Correlation, balance and stereo position run from one side through a neutral
// middle to the other — out of phase, unrelated, identical; hard left, center,
// hard right. Every other map here reads such a value as more and less of one
// thing: turbo puts "unrelated" at green, an arbitrary step on the way to red,
// and heat and magma put "out of phase" at black, which on this display is
// nothing drawn at all. A diverging map gives the two directions opposite hues
// and the middle no hue, so which way, and how far, are both read at a glance.
//
// The middle is a GRAY, not the white most diverging maps put there. Against
// a black background white is the brightest thing on the screen, and the
// middle is the least interesting reading — uncorrelated is the ambient state
// of a wide mix, not the news. Lit but neutral says "nothing to see" without
// vanishing. The ends are held to about the same lightness, so neither
// direction looks louder than the other.
//
// Mixed in linear light, so the walk from each end to the gray is an even fade
// of saturation rather than the muddy dip a straight sRGB blend makes.
func Diverge(v float64) color.Color {
	v = clamp01(v)
	lo, hi, t := divergeLow, divergeMid, v*2
	if v > 0.5 {
		lo, hi, t = divergeMid, divergeHigh, v*2-1
	}
	var out [3]uint8
	for i := range out {
		l := toLinear(lo[i])*(1-t) + toLinear(hi[i])*t
		out[i] = toSRGB(l)
	}
	return color.RGBA{out[0], out[1], out[2], 255}
}

// The three stops: blue, gray, red-orange (sRGB).
var (
	divergeLow  = [3]uint8{0x3a, 0x7b, 0xff}
	divergeMid  = [3]uint8{0x9a, 0x9a, 0x9a}
	divergeHigh = [3]uint8{0xff, 0x5a, 0x36}
)

// toLinear is an sRGB channel in linear light, 0..1.
func toLinear(c uint8) float64 {
	s := float64(c) / 255
	if s <= 0.04045 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

// toSRGB is a linear-light channel back in sRGB, rounded.
func toSRGB(l float64) uint8 {
	l = clamp01(l)
	s := l * 12.92
	if l > 0.0031308 {
		s = 1.055*math.Pow(l, 1/2.4) - 0.055
	}
	return uint8(math.Round(s * 255)) //nolint:gosec // s is 0..1 by the clamp above
}
