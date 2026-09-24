package skirt

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

// SweepDeg is the pointer's total travel, centered on straight up.
//
// Here rather than beside the knob drawing because it is the number the
// skirt geometry is built on: where a label sits is a function of the
// sweep, and a second copy of it would be a ring whose labels do not
// line up with the pointer that selects them.
const SweepDeg = 270.0

// Label is one engraved label: the size of its box, and where on the
// ring it sits. Degrees are measured from straight up, clockwise, matching
// the pointer's own travel.
type Label struct {
	W, H, Deg float64
}

// radialHalf is how far a label reaches along the radius — the half
// extent of its box in the direction the radius points.
//
// Not the half diagonal, which is what a first guess reaches for: a wide
// short label at the top of the ring reaches up by half its HEIGHT, and
// charging it half its diagonal would push every ring out to suit a label
// that is nowhere near the part of the ring it is being measured against.
func radialHalf(l Label) float64 {
	r := l.Deg * math.Pi / 180
	return (math.Abs(math.Sin(r))*l.W + math.Abs(math.Cos(r))*l.H) / 2
}

// Radius is the radius the label CENTERS must sit at.
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
func Radius(clear, gap float64, labs []Label) float64 {
	r := clear + gap
	for _, l := range labs {
		if v := clear + gap + radialHalf(l); v > r {
			r = v
		}
	}
	for i := 1; i < len(labs); i++ {
		if v := pairRadius(labs[i-1], labs[i]); v > r {
			r = v
		}
	}
	return r
}

// pairRadius is the smallest radius at which two neighboring labels
// come apart.
//
// Two axis-aligned boxes miss each other as soon as they are separated
// ENOUGH ON EITHER AXIS — they do not have to clear on both. So the answer
// is the smaller of the two radii that would do it, which is what keeps a
// column of short labels from being pushed out as far as a row of wide ones
// would need.
func pairRadius(a, b Label) float64 {
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

// Outer is how far the ring reaches in total — the radius its labels
// are centered on plus the furthest any of them sticks out past that.
//
// What the next skirt out has to clear, and half of what the dial box has to
// be to avoid clipping the labels it contains.
func Outer(radius float64, labs []Label) float64 {
	out := radius
	for _, l := range labs {
		if v := radius + radialHalf(l); v > out {
			out = v
		}
	}
	return out
}

// Angles is where n positions sit on a sweep, in the pointer's own
// travel: evenly spaced across it, centered on straight up. One position
// sits at the top rather than at the start of the sweep, because a single
// position is not a range.
func Angles(n int, sweepDeg float64) []float64 {
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

// ── Fitting a ring into the cell it has to live in ──────────────────────────
//
// A skirt is sized by its legends: enough radius to clear the grip and to
// keep neighboring labels apart. Nothing in that says the result fits the
// control cell, and for the widest legends it did not — Model Out's
// off/CAM/XY/XZ/YZ ring reached 25px past its cell and into the next
// control's space, and the step/fine ring 23px.
//
// Which lever to pull is a question Woodson & Conover already answer. Knob
// size is given as a RANGE, not a value: "the preferred size is between 0.5
// and 2 inches in diameter, 0.75 inch being normal" (2nd ed. §2-103). So a
// crowded ring may take its room from the grip, because a slightly smaller
// knob is still a correctly sized knob. Only when that is spent does the
// legend itself shrink, and only to the point where it is still a legend.
//
// This matters more as the panel gains rotaries. A rotary switch says what
// every one of its positions is without being touched, which a toggle cannot;
// it is the right control wherever there are more than two settings, and the
// reason not to use one should never be that its legend does not fit.

// MinGripFrac is how far the grip may shrink to make room, as a
// fraction of its natural radius. 0.62 takes the panel's usual 38px knob to
// 23.5px, which at this panel's scale is still inside §2-103's range.
const MinGripFrac = 0.62

// minLabelScale is how far a legend may shrink once the grip is spent.
// Below about three quarters the 8px face type stops being readable at arm's
// length, which is the whole purpose of silkscreening it.
const minLabelScale = 0.75

// ScaleLabels is labs with every box scaled — the legends set in
// smaller type, at the same angles.
func ScaleLabels(labs []Label, s float64) []Label {
	out := make([]Label, len(labs))
	for i, l := range labs {
		out[i] = Label{W: l.W * s, H: l.H * s, Deg: l.Deg}
	}
	return out
}

// Fit is the grip radius and legend scale at which this ring fits
// inside maxOuter.
//
// minGrip is how far the grip may shrink. It is a parameter rather than
// grip*MinGripFrac because not every ring is sitting on a knob: the
// outer ring of a concentric control clears the ring INSIDE it, and there is
// nothing there to take room from. Passing minGrip == grip disables the
// first lever and puts the whole reduction on the legend, which is the
// truthful answer for those.
//
// maxOuter of zero or less means unconstrained, which is the honest answer
// when the cell has not been measured yet: an unmeasured cell must not shrink
// a knob to nothing.
func Fit(grip, minGrip, gap, maxOuter float64, labs []Label) (useGrip, scale float64) {
	fits := func(g, s float64) bool {
		sc := ScaleLabels(labs, s)
		return Outer(Radius(g, gap, sc), sc) <= maxOuter
	}
	if minGrip > grip {
		minGrip = grip
	}
	if maxOuter <= 0 || len(labs) == 0 || fits(grip, 1) {
		return grip, 1
	}
	// First lever: a smaller grip.
	for i := 1; i <= 8; i++ {
		g := grip - (grip-minGrip)*float64(i)/8
		if fits(g, 1) {
			return g, 1
		}
	}
	// Second: smaller legends, with the grip already at its floor.
	for i := 1; i <= 8; i++ {
		s := 1 - (1-minLabelScale)*float64(i)/8
		if fits(minGrip, s) {
			return minGrip, s
		}
	}
	// Both spent. Return the smallest of each: the ring still overhangs, but
	// by as little as this panel is willing to make it, and an overhang that
	// is visible is better than a legend that cannot be read.
	return minGrip, minLabelScale
}
