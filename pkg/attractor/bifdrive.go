package attractor

// Driving the bifurcation diagram from the live audio envelope.
//
// WHY THE DIAGRAM ITSELF DOES NOT MOVE.
//
// The obvious reading of "drive the swept parameter from the audio" is to
// replace the sweep: instead of stepping the parameter left to right, set it
// from the envelope each frame and plot what comes out. That produces a
// picture, and the picture is a lie.
//
// A bifurcation diagram means what it means because x IS the parameter and the
// parameter is monotonic in x. Every column is a separate integration, run to
// its own asymptote at its own parameter value, and the branches you read off
// it — a period-2 orbit splitting to period-4, a periodic window inside a
// chaotic band — are structure in (parameter, value). Take the monotonicity
// away and none of that survives. The same x gets revisited seconds apart with
// different transients behind it, columns arrive out of order, and the
// horizontal axis has quietly become a time series of the loudness with a
// parameter's label still on it. A diagram whose x jumps around is not a
// bifurcation diagram, and the fact that it would look busy and musical is
// exactly what makes it dishonest.
//
// The two ways out, and why this file picks the second:
//
//  1. Plot against TIME, with the parameter as color. Honest, and a different
//     picture: a time series of the attractor's asymptotic values, colored by
//     what the music was doing. It throws away the thing the view exists for.
//     A period-doubling cascade is a SHAPE in the parameter-value plane — a
//     fig tree — and replotting it against time scatters it into a band with
//     no structure left to see. The mode would keep its name and lose its
//     subject.
//
//  2. Keep the parameter on x, let the sweep compute the diagram exactly as
//     before, and let the audio move a CURSOR along it.
//
// Two it is. The diagram stays a real bifurcation diagram, computed by a
// monotonic sweep, and the audio answers the one question the diagram cannot
// answer on its own: where on it are we now. The cursor claims only that — at
// this instant the envelope puts the system at this parameter — and the branch
// structure it lands on is the structure that was actually measured there.
// Nothing on screen is a claim about a measurement that was not made.
//
// It also turns out to be the more musical of the two. Watching a cursor cross
// from a single branch into a fan of them, and into the chaotic band beyond,
// is watching the transition happen on the beat; a scatter that merely got
// wider would not have told you which transition it was.

// bifCols is the sweep resolution: how many separate integrations the diagram
// is made of, and so how many distinct parameter values its x axis can hold.
//
// Untagged, with the cursor arithmetic rather than with the renderer, because
// the cursor has to land on the column the sweep actually computed — that is
// a property of this number, and testing it without a browser means having the
// number without one.
const bifCols = 360

// bifAudioSpan returns the low end and the width of the parameter window the
// envelope is mapped onto: a fraction of the parameter's full range, centered
// on wherever the knob is.
//
// The window is SHIFTED to fit inside the range rather than the value being
// clamped to it. Clamping is the obvious implementation and it is wrong in a
// way you would see immediately: with the knob near either end of the range,
// half the envelope's travel would map outside the axis, so the cursor would
// sit pinned at the end for half the music and only move during the other
// half. A shifted window keeps the whole envelope mapped onto the whole span,
// which is what a depth control is for.
func bifAudioSpan(depth, center, min, max float32) (lo, span float32) {
	full := max - min
	if full <= 0 {
		return min, 0
	}
	if depth < 0 {
		depth = 0
	} else if depth > 1 {
		depth = 1
	}
	span = full * depth
	lo = center - span/2
	if lo < min {
		lo = min
	}
	if lo+span > max {
		lo = max - span
	}
	return lo, span
}

// bifAudioValue maps a 0..1 envelope onto a parameter value inside that
// window. Silence is the low end and a full-scale envelope is the high end,
// which is the direction that reads right: louder pushes the system further
// along the axis, into the cascade rather than out of it.
func bifAudioValue(env, depth, center, min, max float32) float32 {
	lo, span := bifAudioSpan(depth, center, min, max)
	if env < 0 {
		env = 0
	} else if env > 1 {
		env = 1
	}
	return lo + span*env
}

// bifFrac is a parameter value's position along the axis, 0..1 — the same
// mapping the sweep uses to place its columns, so the cursor lands where the
// column for that value was drawn and not half a column off it.
func bifFrac(v, min, max float32) float32 {
	full := max - min
	if full <= 0 {
		return 0
	}
	f := (v - min) / full
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// bifColumnFor is the column of the diagram a parameter value falls in.
// Rounded, not truncated: the sweep places column j at exactly
// min + (max-min)·j/(cols-1), so the nearest column is the one whose points
// were computed closest to this value, and truncating would bias the whole
// cursor half a column low.
func bifColumnFor(v, min, max float32, cols int) int {
	if cols < 2 {
		return 0
	}
	j := int(bifFrac(v, min, max)*float32(cols-1) + 0.5)
	if j < 0 {
		return 0
	}
	if j > cols-1 {
		return cols - 1
	}
	return j
}
