package takens

import "math"

// The polar embedding's radial maps. Here rather than beside the generator
// in pkg/attractor/polar_js.go, so `chaosrack render` bends the delay vector
// exactly as the page does.

// The radius maps, in knob order.
const (
	PolarTanh = iota
	PolarAlgebraic
	PolarLog
	PolarUnit
	PolarCount
)

// PolarRadius maps a delay vector's length to the length it is DRAWN at, as a
// fraction of GAIN. Every map returns a value in [0, 1] for every non-negative
// r and every positive drive, which is the whole property this mode is built
// on and what polarFitExtent relies on.
//
// A negative or non-finite r is answered with 0 rather than propagated. r comes
// from a square root of a sum of squares of audio samples, so it can only be
// either of those if the samples were, and a NaN coordinate multiplied through
// the vertex buffer is a hole in the trail that GL will not tell anyone about.
func PolarRadius(m int, r, drive float32) float32 {
	if !(r > 0) { // false for NaN, and 0 is already the answer for 0
		return 0
	}
	if !(drive > 0) {
		drive = 0.2 // the knob's floor; a zero drive would collapse the figure to a point
	}
	switch m {
	case PolarAlgebraic:
		// drive·r/(1 + drive·r), written as 1 − 1/(1 + drive·r). Below tanh
		// everywhere past the origin, so the same drive keeps more of the
		// mid-range and reaches the surface later.
		//
		// The rearrangement is not cosmetic: the direct form is ∞/∞ for an
		// infinite length and returns NaN, where this one returns the surface,
		// which is the answer. An infinite delay coordinate cannot come from a
		// sample bounded to ±1 — but the maps are also reachable from the audio
		// modulator's arithmetic, and a NaN written into the vertex buffer is a
		// hole in the trail that GL reports to nobody.
		return 1 - 1/(1+drive*r)
	case PolarLog:
		// r ↦ 1 + log₁₀(r)/drive: a DECIBEL radius. Full scale is the surface,
		// and DRIVE is the window in DECADES below it that the sphere spends its
		// radius on — 2 is 40 dB, about the useful range of a level meter, and
		// the knob reaches from 4 dB to 200 dB.
		//
		// The other two curves are linear near the origin, and the origin is
		// where almost all of the audio is: program material sits tens of dB
		// below full scale, so tanh and the algebraic map draw the quiet
		// nine-tenths of a signal in the middle tenth of the sphere and spend
		// the outside on peaks that are hardly ever there. This spends radius on
		// RATIOS instead — every doubling of level is the same step outward,
		// 6 dB of the window wherever it falls — which is how a meter is scaled
		// and how loudness is actually heard.
		//
		// It is NOT log(1+drive·r)/log(1+drive), which was tried first and is no
		// decibel scale at all: normalizing that way divides the slope by
		// log(1+drive), so at the default drive it draws the quiet end SMALLER
		// than tanh does — the opposite of the point, and caught by the test that
		// asks whether the map lifts the quiet end.
		//
		// Everything below the window is the origin rather than a negative
		// radius, and everything at or past full scale is the surface. The clamps
		// are not defensive decoration: a delay vector reaches √3 when its three
		// coordinates peak together (takensCubeDiag), so r > 1 is ordinary here,
		// and the maps are reachable from the audio modulator's arithmetic
		// besides.
		v := float32(1 + math.Log10(float64(r))/float64(drive))
		if v > 1 {
			return 1
		}
		if !(v > 0) { // false for NaN, and for everything under the window
			return 0
		}
		return v
	case PolarUnit:
		// Loudness removed entirely: the surface of the sphere and nothing else.
		return 1
	default: // PolarTanh
		return float32(math.Tanh(float64(drive * r)))
	}
}

// PolarScale is the factor each coordinate of a vector of length r is
// multiplied by. Isotropic by construction — one factor for all three — which
// is what keeps the direction, and therefore the reconstructed geometry,
// exactly as the delay vector had it.
func PolarScale(m int, r, drive float32) float32 {
	if !(r > 0) {
		return 0 // a zero vector has no direction to preserve; it stays at the origin
	}
	return PolarRadius(m, r, drive) / r
}
