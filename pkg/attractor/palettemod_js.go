//go:build js && wasm

package attractor

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
// Off the ends the window reflects rather than wrapping or clamping; the
// arithmetic, and why, is pkg/colormap's window.go.
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
