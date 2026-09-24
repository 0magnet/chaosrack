// Package colormap turns a value into a color, the one way the whole rack
// does it: the MAP ring's mixes and hue sweep, the spectrogram's published
// colormaps, and the window the trace views them through.
//
// The gradient had four palettes: one color, two, three, and a raw HSV
// rainbow. The rainbow is the only one that spans a spectrum and it is the
// worst of the four at it — HSV is perceptually uneven, so equal steps in the
// gradient parameter are visibly unequal steps in color, with a wide flat
// green and a narrow sharp cyan. A figure colored by frequency through it
// reads as bands wherever the colormap happens to move fast, and those bands
// are an artifact of the color space rather than anything in the sound.
//
// The spectrogram next door already solved this. heat, blue, grayscale,
// turbo, viridis and magma are the same six the spectrogram knob offers, and
// turbo and viridis in particular exist precisely because a hue ramp is a bad
// way to show a scalar. Reusing them means a value that paints one color in
// the spectrogram paints the SAME color on the trail — the two displays
// become readable against each other rather than each having its own private
// language.
//
// They are the library's own tables, sampled through its own ValueToPixel
// functions, not a re-implementation. A second copy of turbo would be a
// second set of colors to keep in step, and the first time upstream adjusted
// one the two displays would quietly disagree.
//
// Untagged so the arithmetic tests natively. The trace draws these on the GPU,
// where no test can reach, so the CPU statement here is the half of each pair
// that is pinned down, and the shader is kept to the same expressions.
package colormap

import (
	"image/color"

	"github.com/0magnet/chaosrack/pkg/colorspace"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// First is the uGradientColors value of the first colormap. 1..4 are
// the original palettes (mono, two-color, three-color, rainbow), so the maps
// start above them and the shader tells the two kinds apart by this one
// comparison.
const First = 5

// maps are the colormaps, in spectColNames order, so a palette's
// position on the trace knob matches its position on the spectrogram's.
// Keeping them in one order is the whole point of reusing them: "the third
// one" has to mean the same thing on both knobs.
var maps = []func(float64) color.Color{
	sg.ValueToPixelHeat,
	sg.ValueToPixelBlue,
	sg.ValueToPixelGrayscale,
	sg.ValueToPixelTurbo,
	sg.ValueToPixelViridis,
	sg.ValueToPixelMagma,
}

// Len is how many colormaps there are.
func Len() int { return len(maps) }

// At samples a colormap, clamping first.
//
// The clamp is here rather than assumed of the library: four of the six go
// through its valueToPixelTable, which clamps, but ValueToPixelGrayscale is
// uint8(255.0*value) with nothing in front of it, so an out-of-range value
// converts to a uint8 that is not the color at either end — 2.0 comes back
// darker than 1.0. Nothing here calls it out of range today; this is so that
// staying in range is a property of this file rather than a thing to remember.
func At(idx int, v float64) color.Color {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return maps[idx](v)
}

// Index maps a uGradientColors value to a colormap index, and reports
// whether it names one at all.
func Index(gradientColors int) (int, bool) {
	i := gradientColors - First
	if i < 0 || i >= len(maps) {
		return 0, false
	}
	return i, true
}

// Texels is the width of the colormap texture. 256 because that is the size
// of the library's own tables — sampling more would invent detail that is not
// in the colormap, and sampling less would throw some of it away.
const Texels = 256

// Fill writes colormap idx into dst as Texels opaque RGBA texels, the form a
// GL texture upload takes.
func Fill(dst []byte, idx int) {
	for i := 0; i < Texels; i++ {
		c := At(idx, float64(i)/float64(Texels-1))
		// RGBA returns 16-bit premultiplied values; >>8 takes the high
		// byte, so each is already 0..255 by construction — the colormap
		// tables are opaque 8-bit entries widened on the way out.
		r, g, b, _ := c.RGBA()
		dst[i*4+0] = byte(r >> 8) //nolint:gosec
		dst[i*4+1] = byte(g >> 8) //nolint:gosec
		dst[i*4+2] = byte(b >> 8) //nolint:gosec
		dst[i*4+3] = 255
	}
}

// Map is the MAP ring's setting: which mapping, and what it mixes.
type Map struct {
	Colors         int // the uGradientColors value: 2, 3 and 4 mix and sweep, First and up are colormaps
	Base, Mid, Top [3]float32
	Freq           float32 // hue sweep cycles over the range
}

// At is the MAP ring applied to a 0..1 value: the one function that
// turns a value into a color anywhere in the rack.
//
// Every position is a genuine mapping, which is what the ring now holds:
// 2 and 3 mix the Palette module's own swatches, 4 sweeps hue, and 5 and up are
// the published colormaps. The mono position is gone from here — it was the
// absence of a source, not a mapping, and it lives on the src ring as OFF.
//
// This is the CPU twin of the branch in the fragment shader, and the two have
// to agree: the shader paints the trace and this paints the spectrogram, and
// the whole point of one ring is that a value looks the same on both. The
// window (period and shift) is deliberately NOT applied here, and neither is
// the hue sweep's drifting phase: a spectrogram column is painted once and
// scrolls, so a moving window would give equal magnitudes different colors.
func (m Map) At(v float64) color.Color {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	if idx, ok := Index(m.Colors); ok {
		return At(idx, v)
	}
	mix := func(a, b [3]float32, t float64) color.Color {
		f := func(x, y float32) uint8 {
			return uint8(255 * clamp01(float64(x)+(float64(y)-float64(x))*t)) //nolint:gosec
		}
		return color.RGBA{R: f(a[0], b[0]), G: f(a[1], b[1]), B: f(a[2], b[2]), A: 255}
	}
	switch m.Colors {
	case 3:
		if v < 0.5 {
			return mix(m.Base, m.Mid, v*2)
		}
		return mix(m.Mid, m.Top, (v-0.5)*2)
	case 4:
		c := colorspace.FromHSV(float32(v)*m.Freq, 1, 1)
		return color.RGBA{R: uint8(255 * c[0]), G: uint8(255 * c[1]), B: uint8(255 * c[2]), A: 255} //nolint:gosec
	default: // 2-color, and anything unexpected
		return mix(m.Base, m.Top, v)
	}
}

func clamp01(v float64) float64 {
	return min(max(v, 0), 1)
}
