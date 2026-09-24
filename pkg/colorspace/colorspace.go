// Package colorspace converts the rack's colors between the forms its parts
// speak: the #rrggbb of a swatch, the RGB triple a shader takes, and HSV.
//
// Hue is in TURNS, 0..1, everywhere, because that is what the fragment shader
// uses and every CPU drawing has to match the shader. The color knob's copy of
// HSV counted in degrees while the spectrogram's hue map handed it turns, and
// the spectrogram came out solid red under a rainbow trace. One conversion,
// in one unit, and a knob that wants degrees converts at its own dial.
package colorspace

import (
	"math"
	"strconv"
)

// FromHSV is the fragment shader's hsv2rgb, ported exactly:
//
//	K = (1, 2/3, 1/3, 3); p = abs(fract(h + K.xyz)*6 - K.www)
//	rgb = v * mix(K.xxx, clamp(p - K.xxx, 0, 1), s)
//
// h wraps, so any real hue is a hue; s and v are 0..1.
func FromHSV(h, s, v float32) [3]float32 {
	fract := func(x float32) float32 { return x - float32(math.Floor(float64(x))) }
	k := [3]float32{0, 2.0 / 3.0, 1.0 / 3.0}
	var out [3]float32
	for i := range 3 {
		p := float32(math.Abs(float64(fract(h+k[i])*6 - 3)))
		out[i] = v * (1 + s*(clamp01(p-1)-1))
	}
	return out
}

// ToHSV is FromHSV's inverse: h in 0..1, s and v 0..1. A gray has hue 0.
func ToHSV(c [3]float32) (h, s, v float32) {
	r, g, b := c[0], c[1], c[2]
	mx := max(r, g, b)
	d := mx - min(r, g, b)
	v = mx
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = (g - b) / d
		if h < 0 {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, v
}

// ParseHex reads a swatch's #rrggbb. Anything shorter is white, which is what
// an unset swatch has always drawn as.
func ParseHex(hex string) [3]float32 {
	if len(hex) < 7 {
		return [3]float32{1, 1, 1}
	}
	var c [3]float32
	for i := range c {
		n, _ := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8) //nolint:errcheck // a swatch value; zero is the right fallback if it is ever not hex
		c[i] = float32(n) / 255
	}
	return c
}

// Hex writes c as #rrggbb, rounding and clamping each channel.
func Hex(c [3]float32) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, x := range c {
		n := int(math.Round(float64(clamp01(x)) * 255))
		b[1+2*i], b[2+2*i] = digits[n>>4], digits[n&0xf]
	}
	return string(b)
}

func clamp01(x float32) float32 {
	return min(max(x, 0), 1)
}
