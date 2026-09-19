package attractor

import "math"

// Knob skirts: where the engraved labels go.
//
// A rotary switch's positions are printed on a SKIRT — the flange below the
// grip that carries the pointer and the numbers. Woodson & Conover put the
// requirement plainly (Human Engineering Guide for Equipment Designers, 2nd
// ed., §2-104): a skirt exists to "provide a place for a pointer or for
// numbers or labels", and "a properly designed skirt prevents the fingers
// from covering up the pointer or the numbers engraved on the skirt."
//
// chaosrack's label rings ARE skirts, and they were sized by a number typed
// at each of twenty-six call sites — 46, 50, 43, 45, 40, 38, 36, 31 — chosen
// by eye, one dial at a time. Measured against the rule: 201 of 261 labels
// sat on top of the grip they belong to, and 26 of 83 rings had labels
// overlapping each other. There was no rule to get wrong, which is why it
// was wrong nearly everywhere.
//
// So the radius is derived here instead, from what actually has to fit: the
// thing the skirt must clear, and the size of the labels themselves. Two
// constraints, no judgement, and it holds for a two-position switch and for
// the scope's fifteen-position timebase alike.

// knobSweepDeg is the pointer's total travel, centered on straight up.
//
// Here rather than beside the knob drawing because it is the number the
// skirt geometry is built on: where a label sits is a function of the
// sweep, and a second copy of it would be a ring whose labels do not
// line up with the pointer that selects them.
const knobSweepDeg = 270.0

// skirtLabel is one engraved label: the size of its box, and where on the
// ring it sits. Degrees are measured from straight up, clockwise, matching
// the pointer's own travel.
type skirtLabel struct {
	W, H, Deg float64
}

// skirtRadialHalf is how far a label reaches along the radius — the half
// extent of its box in the direction the radius points.
//
// Not the half diagonal, which is what a first guess reaches for: a wide
// short label at the top of the ring reaches up by half its HEIGHT, and
// charging it half its diagonal would push every ring out to suit a label
// that is nowhere near the part of the ring it is being measured against.
func skirtRadialHalf(l skirtLabel) float64 {
	r := l.Deg * math.Pi / 180
	return (math.Abs(math.Sin(r))*l.W + math.Abs(math.Cos(r))*l.H) / 2
}

// skirtRadius is the radius the label CENTERS must sit at.
//
// clear is the radius of everything inside the skirt that it has to stay off
// — the grip, or the outer edge of a ring already placed on the same knob,
// since concentric controls carry concentric skirts. gap is the daylight
// left between them.
//
// Two constraints, and the answer is whichever is larger:
//
//	the skirt clears what is inside it, for every label; and
//	no two neighboring labels overlap.
//
// A ring with one label has only the first, and a ring with none has
// neither — both are answered by the clearance alone rather than by a
// special case.
func skirtRadius(clear, gap float64, labs []skirtLabel) float64 {
	r := clear + gap
	for _, l := range labs {
		if v := clear + gap + skirtRadialHalf(l); v > r {
			r = v
		}
	}
	for i := 1; i < len(labs); i++ {
		if v := skirtPairRadius(labs[i-1], labs[i]); v > r {
			r = v
		}
	}
	return r
}

// skirtPairRadius is the smallest radius at which two neighboring labels
// come apart.
//
// Two axis-aligned boxes miss each other as soon as they are separated
// ENOUGH ON EITHER AXIS — they do not have to clear on both. So the answer
// is the smaller of the two radii that would do it, which is what keeps a
// column of short labels from being pushed out as far as a row of wide ones
// would need.
func skirtPairRadius(a, b skirtLabel) float64 {
	ra := a.Deg * math.Pi / 180
	rb := b.Deg * math.Pi / 180
	// Centers sit at (R·sin, −R·cos), so the separation grows linearly with
	// R and these are the per-unit-radius components of it.
	dx := math.Abs(math.Sin(rb) - math.Sin(ra))
	dy := math.Abs(math.Cos(ra) - math.Cos(rb))
	need := math.Inf(1)
	if dx > 1e-9 {
		need = (a.W + b.W) / 2 / dx
	}
	if dy > 1e-9 {
		if v := (a.H + b.H) / 2 / dy; v < need {
			need = v
		}
	}
	if math.IsInf(need, 1) {
		// Two labels at the same angle: no radius separates them. Nothing
		// useful to ask for, and returning infinity would blow up the ring.
		return 0
	}
	return need
}

// skirtOuter is how far the ring reaches in total — the radius its labels
// are centered on plus the furthest any of them sticks out past that.
//
// What the next skirt out has to clear, and half of what the dial box has to
// be to avoid clipping the labels it contains.
func skirtOuter(radius float64, labs []skirtLabel) float64 {
	out := radius
	for _, l := range labs {
		if v := radius + skirtRadialHalf(l); v > out {
			out = v
		}
	}
	return out
}

// skirtAngles is where n positions sit on a sweep, in the pointer's own
// travel: evenly spaced across it, centered on straight up. One position
// sits at the top rather than at the start of the sweep, because a single
// position is not a range.
func skirtAngles(n int, sweepDeg float64) []float64 {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []float64{0}
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = -sweepDeg/2 + sweepDeg*float64(i)/float64(n-1)
	}
	return out
}
