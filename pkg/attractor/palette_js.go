//go:build js && wasm

package attractor

import (
	"image/color"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The spectrogram's colormaps, available to the trace.
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
// WHY A TEXTURE, and not a uniform array of stops. GLSL ES 1.0 guarantees
// only 16 fragment uniform VECTORS; this shader already declares more than
// that (the three gradient colors, six extent bounds, the split plane, the
// gradient controls), so it already depends on better-than-minimum hardware
// and adding thirty-two vec3 stops would lean on that much harder. A 256×1
// texture costs one sampler, samples in one fetch, interpolates in hardware,
// and reproduces the colormap at full resolution instead of at whatever stop
// count the uniform budget allowed.

// paletteFirst is the uGradientColors value of the first colormap. 1..4 are
// the original palettes (mono, two-color, three-color, rainbow), so the maps
// start above them and the shader tells the two kinds apart by this one
// comparison.
const paletteFirst = 5

// paletteFns are the colormaps, in spectColNames order, so a palette's
// position on the trace knob matches its position on the spectrogram's.
// Keeping them in one order is the whole point of reusing them: "the third
// one" has to mean the same thing on both knobs.
var paletteFns = []func(float64) color.Color{
	sg.ValueToPixelHeat,
	sg.ValueToPixelBlue,
	sg.ValueToPixelGrayscale,
	sg.ValueToPixelTurbo,
	sg.ValueToPixelViridis,
	sg.ValueToPixelMagma,
}

// paletteColorAt samples a colormap, clamping first.
//
// The clamp is here rather than assumed of the library: four of the six go
// through its valueToPixelTable, which clamps, but ValueToPixelGrayscale is
// uint8(255.0*value) with nothing in front of it, so an out-of-range value
// converts to a uint8 that is not the color at either end — 2.0 comes back
// darker than 1.0. Nothing here calls it out of range today; this is so that
// staying in range is a property of this file rather than a thing to remember.
func paletteColorAt(idx int, v float64) color.Color {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return paletteFns[idx](v)
}

// paletteTexels is the width of the colormap texture. 256 because that is the
// size of the library's own tables — sampling more would invent detail that is
// not in the colormap, and sampling less would throw some of it away.
const paletteTexels = 256

// paletteUnit is the texture unit the colormap is bound to. NOT unit 0:
// textured_js.go binds the spectrogram, terminal and desk textures there, and
// a colormap that shares a unit with them comes and goes depending on what
// else was drawn that frame.
const paletteUnit = 1

var (
	paletteTexture js.Value
	paletteBuilt   = -1 // index of the colormap currently in the texture
	paletteBytes   = make([]byte, paletteTexels*4)
	paletteJS      js.Value
)

// paletteIndex maps a uGradientColors value to a colormap index, and reports
// whether it names one at all.
func paletteIndex(gradientColors int) (int, bool) {
	i := gradientColors - paletteFirst
	if i < 0 || i >= len(paletteFns) {
		return 0, false
	}
	return i, true
}

// ensurePaletteTexture uploads the colormap for the current palette, if it is
// not already the one in the texture.
//
// Rebuilt on CHANGE rather than per frame: a colormap is a constant, 256
// calls through the library plus an upload is not per-frame work, and doing
// it every frame is how a static table turns into a stall.
func ensurePaletteTexture(gradientColors int) bool {
	idx, ok := paletteIndex(gradientColors)
	if !ok {
		return false
	}
	if paletteTexture.IsUndefined() {
		paletteTexture = gl.Call("createTexture")
		paletteJS = js.Global().Get("Uint8Array").New(len(paletteBytes))
	}
	if paletteBuilt != idx {
		for i := 0; i < paletteTexels; i++ {
			c := paletteColorAt(idx, float64(i)/float64(paletteTexels-1))
			// RGBA returns 16-bit premultiplied values; >>8 takes the high
			// byte, so each is already 0..255 by construction — the colormap
			// tables are opaque 8-bit entries widened on the way out.
			r, g, b, _ := c.RGBA()
			paletteBytes[i*4+0] = byte(r >> 8) //nolint:gosec
			paletteBytes[i*4+1] = byte(g >> 8) //nolint:gosec
			paletteBytes[i*4+2] = byte(b >> 8) //nolint:gosec
			paletteBytes[i*4+3] = 255
		}
		js.CopyBytesToJS(paletteJS, paletteBytes)
		gl.Call("activeTexture", gl.Get("TEXTURE0").Int()+paletteUnit)
		gl.Call("bindTexture", gl.Get("TEXTURE_2D"), paletteTexture)
		gl.Call("texImage2D", gl.Get("TEXTURE_2D"), 0, gl.Get("RGBA"),
			paletteTexels, 1, 0, gl.Get("RGBA"), gl.Get("UNSIGNED_BYTE"), paletteJS)
		// CLAMP_TO_EDGE and LINEAR: the ends of a colormap are the ends, so a
		// value at 0 or 1 must take the first or last color rather than wrap
		// to the other end of the ramp, and the interpolation between texels
		// is what makes 256 entries look continuous on a wide figure.
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MIN_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_MAG_FILTER"), gl.Get("LINEAR"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_S"), gl.Get("CLAMP_TO_EDGE"))
		gl.Call("texParameteri", gl.Get("TEXTURE_2D"), gl.Get("TEXTURE_WRAP_T"), gl.Get("CLAMP_TO_EDGE"))
		paletteBuilt = idx
		gl.Call("activeTexture", gl.Get("TEXTURE0"))
		return true
	}
	// Re-bind every frame it is used. Another draw may have left a different
	// texture on this unit, and a colormap that is only bound once is a
	// colormap that works until something else touches the unit.
	gl.Call("activeTexture", gl.Get("TEXTURE0").Int()+paletteUnit)
	gl.Call("bindTexture", gl.Get("TEXTURE_2D"), paletteTexture)
	gl.Call("activeTexture", gl.Get("TEXTURE0"))
	return true
}

// ── The points/line continuum ────────────────────────────────────────────
//
// A trace was either a line or a set of points, and those are the two ends of
// something continuous rather than two things. The interesting middle — a
// dotted line, beads close enough to read as a curve — was not reachable, and
// it is the part that makes a dense figure legible: points are easier on the
// eye where the trail folds over itself many times, a solid line is easier
// where the shape matters, and a busy figure usually wants somewhere between.
//
// THE KNOB IS A POINT COUNT, not a dash duty. Those sound like the same
// control and are not. Duty asks "what fraction of the line is drawn", which
// at every setting is still a dashed LINE — turning it down thins the trace
// rather than breaking it into anything, which is not what the eye wants from
// "points". Count asks "how many points does the line break into", which is
// the thing being looked at: fewer points, further apart, until they are
// beads; more points until they touch.
//
// Continuity then falls out of the arithmetic instead of being a special
// case. Each point is drawn one vertex long, so N points cover N vertices of
// the trail's V, and the drawn fraction is N/V — at N == V every vertex is
// drawn and the trace is exactly the solid line it always was. There is no
// separate "solid" setting to keep in step; the top of the knob IS solid.

var (
	// pointCount is how many points the trail breaks into. 0 means "as many
	// as there are vertices", i.e. the solid line, and is the default so an
	// existing view is unchanged until the knob is turned.
	pointCount float32

	// dashDuty and dashCount are what the shader reads, derived from
	// pointCount and the vertex count actually drawn. Kept as the shader's
	// own terms because that is what the fragment stage can cheaply test.
	dashDuty  float32 = 1
	dashCount float32 = 1
)

// updateDashFromPointCount converts the point-count knob into the duty and
// cycle count the shader uses, against the number of vertices actually drawn
// this frame.
//
// Against the DRAWN count, not the buffer's: the trail-length modulation
// shortens the drawn tail, and a point count measured against the whole
// buffer would silently change the spacing whenever that moved.
func updateDashFromPointCount(drawn int) {
	if pointCount < 1 || drawn <= 0 || pointCount >= float32(drawn) {
		dashDuty, dashCount = 1, 1 // solid: every vertex drawn
		return
	}
	dashCount = pointCount
	// One vertex per point. Below that the point falls between samples and
	// blinks as the trail advances; above it, points grow into dashes and the
	// knob stops meaning what it says.
	dashDuty = pointCount / float32(drawn)
}
