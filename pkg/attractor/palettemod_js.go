//go:build js && wasm

package attractor

import "math"

// Moving the figure THROUGH a colormap.
//
// The colormaps arrived alongside a gradient source that can already be driven
// from sound, and that is only half of a color instrument. The source decides
// WHAT the gradient value is — brightness, band energy, a short-time spectrum
// laid along the trail — and the palette turns that value into a color. What
// nothing could move was the mapping itself: with turbo selected, the low end
// of the figure was dark blue and the high end dark red, this frame and every
// frame, whatever was playing. The rainbow never had that problem, because it
// has a period and a phase and the sound can already sweep it. This is the
// colormaps' equivalent of that phase.
//
// TWO NUMBERS, ONLY ONE OF THEM NEW. A window onto a colormap needs a width
// and a position: pt = t*span + shift. The width is already on the panel. The
// "period" knob that set the rainbow's hue cycles is asking exactly "how many
// times is the palette crossed across the figure", and that question has the
// same meaning whether the palette is a hue circle or turbo — so it is reused
// (it stops being dimmed outside the rainbow) rather than answered twice. Two
// knobs for one quantity is two values to keep in step, and the first time
// they disagreed one of them would be wrong.
//
// The width is not an optional extra, which is the reason it was worth
// reusing rather than pinning at 1. At span 1 the figure already covers the
// map end to end, so shifting it does not sweep anything — it FOLDS: the map
// runs off its end somewhere in the middle of the figure and comes back down
// the way it went up. That is a real effect and not the one asked for. A
// sweep needs the figure to occupy a slice of the map (period well below 1)
// that the shift then slides along, which is what the shift knob's tooltip
// points at.
//
// ── Off the ends: reflect, not wrap and not clamp ────────────────────────
//
// A window that can be moved can be moved off the map, and the three
// available answers are not close.
//
// WRAP (fract) is what the hue sweep does, and it is right there for the
// taking — but hue is a CIRCLE: the end of the spectrum is the beginning. A
// colormap is a line. turbo runs dark blue to dark red, viridis dark purple
// to yellow, magma black to near-white; joining either one's end to its own
// start puts a step discontinuity in the color field, which draws as a hard
// edge cutting across the figure wherever the coordinate crosses. It is worse
// under modulation than at rest, too: the seam MOVES with the sound, and a
// moving edge is the most conspicuous thing on a screen, so the eye ends up
// reading the artifact rather than the sweep.
//
// CLAMP has no seam and no sweep. The audio features are non-negative, so a
// routed shift pushes in one direction and spends most of its travel pinned
// against an end, where every fragment takes the same color and the gradient
// stops saying anything at all. A control that switches itself off over most
// of its range is not a control.
//
// REFLECT has neither failure. The coordinate folds back at each end, so the
// color field stays continuous — the fold is a maximum, not a step — and the
// sweep never stalls however far the modulation drives it. The price is
// monotonicity: past a fold, two stretches of the figure share a color and
// the palette visibly turns around. That is a shape the eye accepts as part
// of the picture, where a seam reads as the picture being broken. It is also
// what the hardware would have done — this is GL_MIRRORED_REPEAT — computed
// in the shader instead of set on the sampler only because the texture's
// CLAMP_TO_EDGE is what keeps the ENDS of the map exact for every palette
// that never leaves 0..1, and that is the default case.
//
// ── Only the colormaps ───────────────────────────────────────────────────
//
// The window is applied inside the colormap branch of the fragment shader and
// nowhere else. That is a decision, not an omission.
//
// mono has no t in it at all, so the knob would be dead. The two- and
// three-color mixes carry their stops on the panel as color swatches, so
// "push the mix toward its end color" is something the end swatch already
// says directly, and says better. The rainbow is the one that would actively
// break: its hue is t*freq + phase with the phase already advancing every
// frame, so a second additive offset would be a knob fighting an animation
// over the same term — and since the offset lands BEFORE the freq multiply,
// one knob setting would mean a different hue jump at every period. That is
// the muddle a shared offset buys.
//
// Reusing uGradientPhase for the colormaps was the other way to avoid a new
// uniform, and it fails for the mirrored reason: the phase drifts on its own,
// which on a hue circle is a rainbow flowing and on a colormap is the fold
// wandering across the figure forever with nothing in the sound driving it.

// gradientShift is the palette window's position: −1..1, and 0 is the picture
// the colormaps have drawn since they landed. Its home is a viewModTargets
// entry, which is what earns it audio routing, a patchbay column, a MIDI CC
// and a permalink without any of those four needing to know it exists.
var gradientShift float32

// paletteFoldPeriod is the period of the reflection in coordinate units: out
// along the map and back again is two crossings, after which the picture
// repeats exactly. It is why the knob's ±1 range gives up nothing — that is
// one whole period, so every distinct mapping is reachable inside it — and it
// is why a modulated shift can be WRAPPED rather than clamped.
const paletteFoldPeriod = 2

// wrapPaletteShift folds a shift back into [−1, 1).
//
// Audio modulation clamps every other view target to its knob range, and for
// this one clamping would be the same mistake the palette itself rejects
// above: the sweep would jam at an end and sit there for the loud half of the
// music. Wrapping instead is not merely tolerable here, it is INVISIBLE — the
// wrap's period and the fold's period are the same 2 by construction, so a
// shift that steps from just under 1 to just over it comes back as just over
// −1 and paints the same colors it was about to paint. There is no jump to
// hide because the two arithmetics agree; that is why the fold below is
// written in terms of this function rather than repeating the constant.
func wrapPaletteShift(x float32) float32 {
	return x - paletteFoldPeriod*float32(math.Floor(float64(x)/paletteFoldPeriod+0.5))
}

// foldPalette01 is the reflection: any real coordinate brought into 0..1 by
// turning back at each end. The triangle wave is the absolute value of the
// sawtooth over the same period, so it is written that way — one period
// constant, one place to be wrong.
func foldPalette01(x float32) float32 {
	w := wrapPaletteShift(x)
	if w < 0 {
		return -w
	}
	return w
}

// paletteCoord is the CPU statement of the texture coordinate the fragment
// shader computes for a colormap: the window, then the fold.
//
// It exists to be tested. The shader's copy is three lines of GLSL that no
// unit test can reach — there is no GL context in `go test` — so the
// arithmetic is written once here, pinned by the tests next door, and the
// GLSL is kept to the same expression. The pair is only trustworthy if one
// half of it is nailed down.
func paletteCoord(t, span, shift float32) float32 {
	return foldPalette01(t*span + shift)
}
