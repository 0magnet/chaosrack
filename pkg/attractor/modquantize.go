package attractor

// Quantized audio modulation for the COUNT parameters — latitude lines,
// subdivisions, window lengths, embedding delays, dial positions.
//
// Deliberately UNTAGGED, for the same reason ledformat.go is: nothing here
// touches the DOM or GL, and every question it answers ("which whole number is
// the sound asking for", "is that a real move or a boundary wobble") is a table
// test. The js/wasm layer above it (audiomod_js.go) only supplies the paramDef
// and decides what to do with the answer.
//
// The old rule was that integers are not modulatable at all, and it confused a
// value with the grid it is READ on. Nothing stops the value moving
// continuously; what cannot be drawn is half a latitude line. So the modulated
// value is computed as a float exactly as it is for a continuous parameter and
// snapped to the parameter's own Step on the way out — audio pushing a line
// count between 8 and 14, landing on whole numbers, which is what was wanted
// and was impossible.

import "math"

// clampF lives here rather than in audiomod_js.go, where it used to, because
// an untagged file cannot call into a js-tagged one and the quantizer needs it.
// It is pure arithmetic and belongs on this side of the line anyway.
func clampF(x, lo, hi float32) float32 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// snapToStep puts v on the parameter's own grid and clamps it to [min,max].
//
// The grid is anchored at MIN, not at zero, because that is the grid the
// slider itself uses: an <input type=range> steps from its min, so a value off
// that lattice is one the user cannot dial and the LED cannot round-trip.
// Most of the count parameters happen to have a min that is a multiple of
// their step (spect-ovl 5..95 by 5, rec-win 20..2000 by 20, fvf-fmax 500..12000
// by 50) so anchoring at zero would agree with them by luck; the first one
// added whose min is not — say 3..99 by 2 — would silently be offered even
// values its own slider never produces.
//
// step <= 0 means "no grid" and passes the value through clamped, so a caller
// that reaches here with a continuous parameter degrades to the float path
// rather than dividing by zero.
func snapToStep(v, min, max, step float32) float32 {
	if step <= 0 {
		return clampF(v, min, max)
	}
	n := math.Round(float64(v-min) / float64(step))
	return clampF(min+float32(n)*step, min, max)
}

// modStepDeadband is how far PAST the halfway point a modulated value must
// travel before the count it drives is allowed to move, as a fraction of one
// Step. A Schmitt trigger, in other words, with switching points at
// ±(0.5+modStepDeadband)·Step around the value currently held.
//
// Whether this was worth having at all was measured rather than assumed,
// simulating the real signal path (afSmooth's 0.6 attack / 0.12 release over a
// jittery band energy) at 60 fps for 10 s and counting how often the quantized
// value moved. TestQuantizedCountsDoNotChatterOnASteadyTone keeps that
// measurement in the tree. What it found:
//
//   - The per-frame strobe that was the obvious worry — 10, 11, 10, 11 on
//     successive frames — essentially does not happen. Immediate reversals ran
//     0..7 per 600 frames in every configuration tried, with or without a
//     deadband, because afSmooth's slow release has already low-passed the
//     feature long before it reaches here. Rounding alone would have been
//     defensible on that evidence.
//   - What DOES happen is chatter at the rate of the music, or of the room
//     noise. Sphere latitude modulated at depth 0.05 by a STEADY tone (no beat,
//     jitter only) changed the line count 127 times in 10 s — thirteen mesh
//     rebuilds a second from a sound that is not doing anything — because the
//     value sat near a boundary and the jitter walked it back and forth across.
//     A ±0.15·Step deadband takes that to 2.
//   - And it is discriminating rather than merely damping.
//     TestTheDeadbandKeepsModulationThatIsReallyThere runs the other half: a
//     full-depth sweep over a 2 Hz beat keeps 516 of the 527 changes plain
//     rounding makes, so what is being removed really is the chatter and not
//     the music.
//
// The exploratory harness this was tuned in, which is not in the tree, swept
// the excursion as well: an excursion of ~0.3 of a step (a modulation that
// never actually spans a whole line) went from 40 changes to 0, while an
// excursion of ~1 step kept all 40 of its changes. That is the shape wanted —
// movement that does not exist is discarded, movement that does is not.
//
// 0.15 rather than something larger: the same harness put 0.35 at eating real
// modulation — the 1-step excursion collapsing from 40 changes to 1, and
// spect-ovl at depth 0.1 from 47 to 1, which is a knob that has stopped
// working rather than one that has stopped flickering.
const modStepDeadband = 0.15

// quantizeHeld returns the grid value a modulated float should read as this
// frame, given the one it read LAST frame.
//
// held/hasHeld is the whole of the hysteresis: without a previous value there
// is nothing to be sticky about and it is a plain nearest-snap. With one, the
// held value stands until v has traveled more than (0.5+modStepDeadband)·Step
// away from it, and only then does the value snap to whatever v is nearest —
// which is the neighboring notch in the ordinary case, but may be several
// notches away if the base slider was dragged, so the trigger cannot lag
// behind a deliberate move.
//
// A held value left over from a route that was switched off and later switched
// back on is deliberately NOT discarded. It is at worst one deadband away from
// where the value belongs, which is the same tolerance the trigger grants
// anyway, and the first sample outside that window resnaps it. Pruning would
// buy exactness that no frame can observe.
func quantizeHeld(v, held float32, hasHeld bool, min, max, step float32) float32 {
	if !hasHeld || step <= 0 {
		return snapToStep(v, min, max, step)
	}
	d := v - held
	if d < 0 {
		d = -d
	}
	if d <= (0.5+modStepDeadband)*step {
		return held
	}
	return snapToStep(v, min, max, step)
}
