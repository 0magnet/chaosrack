package attractor

import "github.com/0magnet/chaosrack/pkg/colorspace"

// The color knob's arithmetic, untagged so it tests natively. The knob is two
// rings over hidden sliders: hue in degrees, 0..360, and a 0..100 Level axis
// from black through the pure hue to white. The swatch it drives speaks hex,
// and colorspace counts hue in turns; the conversions between them are here.

// knobHSV reads a swatch color as the rings see it: hue in degrees.
func knobHSV(hex string) (h, s, v float64) {
	th, ts, tv := colorspace.ToHSV(colorspace.ParseHex(hex))
	return float64(th) * 360, float64(ts), float64(tv)
}

// knobHex is the swatch color for a hue in degrees.
func knobHex(h, s, v float64) string {
	return colorspace.Hex(colorspace.FromHSV(float32(h/360), float32(s), float32(v)))
}

// levelToSV maps the 0..100 Level axis to HSV saturation/value: 0 = black,
// 50 = pure saturated hue, 100 = white.
func levelToSV(l float64) (s, v float64) {
	if l <= 50 {
		return 1, l / 50
	}
	return 1 - (l-50)/50, 1
}

// svToLevel is the inverse used when an arbitrary swatch color is picked; it is
// approximate for muted colors (the swatch, not the knob, is authoritative).
func svToLevel(s, v float64) float64 {
	if v < 0.999 {
		return v * 50
	}
	return 50 + (1-s)*50
}
