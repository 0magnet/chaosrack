package colormap

import "math"

// The window a figure views a colormap through: pt = t*span + shift, folded
// back into the map at its ends.
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

// FoldPeriod is the period of the reflection in coordinate units: out
// along the map and back again is two crossings, after which the picture
// repeats exactly. It is why the knob's ±1 range gives up nothing — that is
// one whole period, so every distinct mapping is reachable inside it — and it
// is why a modulated shift can be WRAPPED rather than clamped.
const FoldPeriod = 2

// WrapShift folds a shift back into [−1, 1).
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
func WrapShift(x float32) float32 {
	return x - FoldPeriod*float32(math.Floor(float64(x)/FoldPeriod+0.5))
}

// Fold is the reflection: any real coordinate brought into 0..1 by
// turning back at each end. The triangle wave is the absolute value of the
// sawtooth over the same period, so it is written that way — one period
// constant, one place to be wrong.
func Fold(x float32) float32 {
	w := WrapShift(x)
	if w < 0 {
		return -w
	}
	return w
}

// Coord is the CPU statement of the texture coordinate the fragment
// shader computes for a colormap: the window, then the fold.
//
// It exists to be tested. The shader's copy is three lines of GLSL that no
// unit test can reach — there is no GL context in `go test` — so the
// arithmetic is written once here, pinned by the tests next door, and the
// GLSL is kept to the same expression. The pair is only trustworthy if one
// half of it is nailed down.
func Coord(t, span, shift float32) float32 {
	return Fold(t*span + shift)
}
