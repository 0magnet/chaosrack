//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/colormap"
	"github.com/0magnet/chaosrack/pkg/glctx"
	"image/color"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// The colormaps on the GPU: pkg/colormap decides the colors, and this uploads
// them as the texture the fragment shader samples.
//
// WHY A TEXTURE, and not a uniform array of stops. GLSL ES 1.0 guarantees
// only 16 fragment uniform VECTORS; this shader already declares more than
// that (the three gradient colors, six extent bounds, the split plane, the
// gradient controls), so it already depends on better-than-minimum hardware
// and adding thirty-two vec3 stops would lean on that much harder. A 256×1
// texture costs one sampler, samples in one fetch, interpolates in hardware,
// and reproduces the colormap at full resolution instead of at whatever stop
// count the uniform budget allowed.

// paletteUnit is the texture unit the colormap is bound to. NOT unit 0:
// textured_js.go binds the spectrogram, terminal and desk textures there, and
// a colormap that shares a unit with them comes and goes depending on what
// else was drawn that frame.
const paletteUnit = 1

// paletteTex is the colormap texture on the GPU.
type paletteTex struct {
	texture js.Value
	built   int // index of the colormap currently in the texture
	js      js.Value
}

var pal = paletteTex{
	built: -1,
}

var (
	paletteBytes = make([]byte, colormap.Texels*4)
)

// ensurePaletteTexture uploads the colormap for the current palette, if it is
// not already the one in the texture.
//
// Rebuilt on CHANGE rather than per frame: a colormap is a constant, 256
// calls through the library plus an upload is not per-frame work, and doing
// it every frame is how a static table turns into a stall.
func (p *paletteTex) ensurePaletteTexture(gradientColors int) bool {
	idx, ok := colormap.Index(gradientColors)
	if !ok {
		return false
	}
	if p.texture.IsUndefined() {
		p.texture = glctx.GL.Call("createTexture")
		p.js = js.Global().Get("Uint8Array").New(len(paletteBytes))
	}
	if p.built != idx {
		colormap.Fill(paletteBytes, idx)
		js.CopyBytesToJS(p.js, paletteBytes)
		glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+paletteUnit)
		glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), p.texture)
		glctx.GL.Call("texImage2D", glctx.GL.Get("TEXTURE_2D"), 0, glctx.GL.Get("RGBA"),
			colormap.Texels, 1, 0, glctx.GL.Get("RGBA"), glctx.GL.Get("UNSIGNED_BYTE"), p.js)
		// CLAMP_TO_EDGE and LINEAR: the ends of a colormap are the ends, so a
		// value at 0 or 1 must take the first or last color rather than wrap
		// to the other end of the ramp, and the interpolation between texels
		// is what makes 256 entries look continuous on a wide figure.
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MIN_FILTER"), glctx.GL.Get("LINEAR"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_MAG_FILTER"), glctx.GL.Get("LINEAR"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_S"), glctx.GL.Get("CLAMP_TO_EDGE"))
		glctx.GL.Call("texParameteri", glctx.GL.Get("TEXTURE_2D"), glctx.GL.Get("TEXTURE_WRAP_T"), glctx.GL.Get("CLAMP_TO_EDGE"))
		p.built = idx
		glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
		return true
	}
	// Re-bind every frame it is used. Another draw may have left a different
	// texture on this unit, and a colormap that is only bound once is a
	// colormap that works until something else touches the unit.
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0").Int()+paletteUnit)
	glctx.GL.Call("bindTexture", glctx.GL.Get("TEXTURE_2D"), p.texture)
	glctx.GL.Call("activeTexture", glctx.GL.Get("TEXTURE0"))
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

// gradientColorsUniform is what uGradientColors is set to: the map ring, unless
// the source is OFF, in which case the shader's monochrome branch.
//
// The derivation lives here rather than in the knob because OFF only silences
// the displays that HAVE a source to turn off — the geometry, which reads the
// src ring to decide what the color follows. A display with one intrinsic
// value never consulted that knob: the spectrogram's value is magnitude, the
// RTA's is level, the transfer function's is coherence. Those read the map ring
// directly and go on doing so, which is why gradientColors itself stays the map
// and only this one call site folds OFF in.
func gradientColorsUniform() int {
	if style.gradientSource == GradientSourceOff {
		return 1
	}
	return style.gradientColors
}

// modeUsesGradientSource reports whether the model on screen reads the SRC ring
// at all.
//
// The geometry does: an attractor, an embedding, the waterfall surface — the
// color follows a coordinate, an age, or the sound, and which one is a choice.
// A display built from one quantity does not: there is nothing to choose. The
// panel has never said which is which, so the src knob sat there looking live
// in modes that ignore it.
func modeUsesGradientSource(mode string) bool {
	switch mode {
	case "spectrogram", "rta", "xfer":
		return false
	}
	return true
}

// mapColorAt is the MAP ring applied to a 0..1 value, as the panel has it set:
// colormap.Map.At over the ring's position, the Palette module's swatches and
// the hue sweep's period.
func mapColorAt(v float64) color.Color {
	return colormap.Map{
		Colors: style.gradientColors, Base: style.baseColor, Mid: style.midColor, Top: style.topColor,
		Freq: style.gradientFreq,
	}.At(v)
}

// spectrogramPixel is the color a raw magnitude paints on the spectrogram.
//
// The magnitude is normalized by the library — the same log/linear scale and
// the same MIN/MAX window the spectrogram's own knobs set — and then colored by
// the shared map ring. Splitting it there is what lets one ring paint the
// spectrogram and the trace alike: the normalization is the spectrogram's
// business, the mapping is everybody's.
//
// ── WHY THE PERIOD AND SHIFT WINDOW IS NOT APPLIED ───────────────────────
//
// On the trace the window is a lens: the figure is redrawn every frame, so
// narrowing the period and sweeping the shift moves color ALONG the figure and
// the whole of it moves together. A spectrogram column is written once and
// scrolls; it is never repainted. A moving window would leave every column
// wearing whatever the window happened to be when it was drawn, so two columns
// of equal magnitude would be different colors and the color would stop meaning
// the magnitude. The hue sweep's animated phase is left out for exactly the
// same reason, which is why the case above reads gradientFreq but not
// gradientPhase.
func spectrogramPixel(mag float64) color.Color {
	return mapColorAt(sg.Normalize(spectMagnitude(mag), spectMagMin(), spectMagMax()))
}
