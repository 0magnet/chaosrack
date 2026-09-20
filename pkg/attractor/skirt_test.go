package attractor

import (
	"math"
	"testing"
)

// The rule the whole file exists for: the fingers must not cover the
// numbers. Measured on the real panel before this, 201 of 261 labels sat on
// their own grip — so the test is not that the radius is plausible, it is
// that NO label overlaps the grip, for any ring the panel actually builds.
func TestNoLabelEverSitsOnTheGrip(t *testing.T) {
	const grip = 19.0 // a 38px knob, the panel's usual
	const gap = 3.0
	for _, n := range []int{1, 2, 3, 4, 5, 6, 9, 12, 15, 16} {
		labs := ringOf(n, 26, 9)
		r := skirtRadius(grip, gap, labs)
		for i, l := range labs {
			inner := r - skirtRadialHalf(l)
			if inner < grip+gap-1e-9 {
				t.Errorf("%d labels: label %d reaches in to %.2f, inside the grip+gap at %.2f",
					n, i, inner, grip+gap)
			}
		}
	}
}

// And no two neighbors overlap — the other half of what was measured
// broken, 26 of 83 rings.
func TestNeighboringLabelsComeApart(t *testing.T) {
	const grip, gap = 19.0, 3.0
	for _, n := range []int{2, 3, 4, 5, 6, 9, 12, 15, 16} {
		labs := ringOf(n, 26, 9)
		r := skirtRadius(grip, gap, labs)
		for i := 1; i < len(labs); i++ {
			if overlaps(r, labs[i-1], labs[i]) {
				t.Errorf("%d labels: %d and %d overlap at radius %.2f", n, i-1, i, r)
			}
		}
	}
}

// A long legend needs a bigger skirt than a short one. If it did not, the
// scope's "500 ms" ring would be sized for a ring of "1"s.
func TestALongerLegendNeedsABiggerSkirt(t *testing.T) {
	const grip, gap = 19.0, 3.0
	short := skirtRadius(grip, gap, ringOf(15, 10, 9))
	long := skirtRadius(grip, gap, ringOf(15, 42, 9))
	if long <= short {
		t.Errorf("wide labels gave radius %.2f, narrow ones %.2f — the legend width is being ignored",
			long, short)
	}
}

// Two boxes come apart as soon as they are separated on EITHER axis. A ring
// of short labels stacked vertically must not be pushed out as far as the
// horizontal separation alone would demand, or every tall thin ring becomes
// enormous for no reason.
func TestSeparationOnOneAxisIsEnough(t *testing.T) {
	// Two labels near the bottom of the sweep, far apart vertically and
	// close horizontally.
	a := skirtLabel{W: 40, H: 8, Deg: 100}
	b := skirtLabel{W: 40, H: 8, Deg: 135}
	r := skirtPairRadius(a, b)
	if overlaps(r, a, b) {
		t.Errorf("the pair still overlaps at the radius returned, %.2f", r)
	}
	// The horizontal-only answer would be much larger; check we did better.
	ra, rb := a.Deg*math.Pi/180, b.Deg*math.Pi/180
	xOnly := (a.W + b.W) / 2 / math.Abs(math.Sin(rb)-math.Sin(ra))
	if r >= xOnly {
		t.Errorf("radius %.2f is no better than the horizontal-only %.2f", r, xOnly)
	}
}

// Degenerate rings must not blow up. Two labels at the same angle cannot be
// separated by any radius, and an infinite answer would size the dial to
// infinity rather than merely look wrong.
func TestADegenerateRingDoesNotBlowUp(t *testing.T) {
	same := skirtLabel{W: 20, H: 8, Deg: 45}
	if got := skirtPairRadius(same, same); got != 0 || math.IsInf(got, 0) {
		t.Errorf("two labels at one angle gave %v, want 0", got)
	}
	for _, labs := range [][]skirtLabel{nil, {}, {{W: 20, H: 8, Deg: 0}}} {
		r := skirtRadius(19, 3, labs)
		if math.IsInf(r, 0) || math.IsNaN(r) || r < 19 {
			t.Errorf("%d labels gave radius %v, want a finite radius clearing the grip", len(labs), r)
		}
	}
}

// A skirt clears whatever is inside it, which for a concentric control is
// the ring already placed — so two skirts on one knob nest instead of
// crossing. This is what the 43-and-31 pairs at the call sites were doing by
// hand, badly.
func TestConcentricSkirtsNest(t *testing.T) {
	const gap = 3.0
	inner := ringOf(4, 22, 9)
	outer := ringOf(6, 30, 9)

	rIn := skirtRadius(9, gap, inner)     // clears the small inner knob
	edge := skirtOuter(rIn, inner)        // ... and reaches this far
	rOut := skirtRadius(edge, gap, outer) // the next skirt clears THAT

	for i, l := range outer {
		if in := rOut - skirtRadialHalf(l); in < edge+gap-1e-9 {
			t.Errorf("outer label %d reaches in to %.2f, into the inner ring ending at %.2f",
				i, in, edge)
		}
	}
	if rOut <= rIn {
		t.Errorf("the outer skirt (%.2f) is not outside the inner one (%.2f)", rOut, rIn)
	}
}

// The dial box has to contain the labels, or they are clipped by the very
// element that exists to hold them.
func TestTheRingReportsWhatItActuallyReaches(t *testing.T) {
	labs := ringOf(9, 30, 9)
	r := skirtRadius(19, 3, labs)
	out := skirtOuter(r, labs)
	for i, l := range labs {
		if reach := r + skirtRadialHalf(l); reach > out+1e-9 {
			t.Errorf("label %d reaches %.2f, past the reported outer edge %.2f", i, reach, out)
		}
	}
	if out < r {
		t.Errorf("outer edge %.2f is inside the label centers at %.2f", out, r)
	}
}

// The positions are the pointer's own travel: evenly spaced, centered on
// straight up, spanning the sweep. A single position sits at the top,
// because one position is not a range.
func TestPositionsSpanThePointersTravel(t *testing.T) {
	if got := skirtAngles(1, 270); len(got) != 1 || got[0] != 0 {
		t.Errorf("one position at %v, want straight up", got)
	}
	got := skirtAngles(5, 270)
	if len(got) != 5 {
		t.Fatalf("got %d angles, want 5", len(got))
	}
	if got[0] != -135 || got[4] != 135 {
		t.Errorf("the sweep runs %v..%v, want -135..135", got[0], got[4])
	}
	for i := 1; i < len(got); i++ {
		if d := got[i] - got[i-1]; math.Abs(d-67.5) > 1e-9 {
			t.Errorf("step %d is %v degrees, want an even 67.5", i, d)
		}
	}
	if len(skirtAngles(0, 270)) != 0 {
		t.Error("no positions should give no angles")
	}
}

// ── helpers ──

// ringOf is n labels of the given box size, spread over the pointer sweep.
func ringOf(n int, w, h float64) []skirtLabel {
	out := make([]skirtLabel, n)
	for i, deg := range skirtAngles(n, knobSweepDeg) {
		out[i] = skirtLabel{W: w, H: h, Deg: deg}
	}
	return out
}

// overlaps reports whether two labels' boxes intersect at this radius.
func overlaps(r float64, a, b skirtLabel) bool {
	ax, ay := r*math.Sin(a.Deg*math.Pi/180), -r*math.Cos(a.Deg*math.Pi/180)
	bx, by := r*math.Sin(b.Deg*math.Pi/180), -r*math.Cos(b.Deg*math.Pi/180)
	return math.Abs(ax-bx) < (a.W+b.W)/2-1e-9 && math.Abs(ay-by) < (a.H+b.H)/2-1e-9
}

// The gap is a parameter because it is a design choice, not a constant of
// the geometry — and a wider gap must produce a wider skirt, or it is not
// doing anything.
func TestAWiderGapPushesTheSkirtOut(t *testing.T) {
	labs := ringOf(6, 24, 10)
	tight := skirtRadius(19, 1, labs)
	loose := skirtRadius(19, 8, labs)
	if loose <= tight {
		t.Errorf("gap 8 gave radius %.2f, gap 1 gave %.2f — the gap is ignored", loose, tight)
	}
	if got := loose - tight; math.Abs(got-7) > 1e-9 {
		t.Errorf("seven more pixels of gap moved the skirt %.2f, want 7", got)
	}
}

// The sweep is a parameter too: a switch with less travel packs its
// positions into a narrower arc, which is a harder crowding problem, not an
// easier one.
func TestANarrowerSweepCrowdsThePositions(t *testing.T) {
	wide := skirtAngles(5, 270)
	narrow := skirtAngles(5, 90)
	if narrow[0] != -45 || narrow[4] != 45 {
		t.Errorf("a 90 degree sweep runs %v..%v, want -45..45", narrow[0], narrow[4])
	}
	spanW := wide[4] - wide[0]
	spanN := narrow[4] - narrow[0]
	if spanN >= spanW {
		t.Errorf("the narrow sweep spans %v, the wide one %v", spanN, spanW)
	}
	// Crowded positions need a bigger radius to come apart on.
	mk := func(angles []float64) []skirtLabel {
		out := make([]skirtLabel, len(angles))
		for i, d := range angles {
			out[i] = skirtLabel{W: 30, H: 9, Deg: d}
		}
		return out
	}
	if rN, rW := skirtRadius(19, 3, mk(narrow)), skirtRadius(19, 3, mk(wide)); rN <= rW {
		t.Errorf("the crowded ring came out at %.2f, no bigger than the roomy one at %.2f", rN, rW)
	}
}

// A taller label reaches further along the radius where the ring is near
// vertical, which is the case a half-diagonal would get wrong.
func TestATallerLabelReachesFurtherAtTheTop(t *testing.T) {
	short := skirtRadialHalf(skirtLabel{W: 30, H: 8, Deg: 0})
	tall := skirtRadialHalf(skirtLabel{W: 30, H: 20, Deg: 0})
	if tall <= short {
		t.Errorf("at the top a 20px-tall label reaches %.2f and an 8px one %.2f", tall, short)
	}
	// And at the side it is the WIDTH that decides, not the height.
	if got := skirtRadialHalf(skirtLabel{W: 30, H: 20, Deg: 90}); math.Abs(got-15) > 1e-9 {
		t.Errorf("at the side the reach is %.2f, want half the width, 15", got)
	}
}

// A ring that already fits is left exactly alone — the fit step must not
// shrink a knob that had no problem.
func TestAFittingRingIsNotTouched(t *testing.T) {
	labs := ringOf(4, 14, 9)
	r := skirtRadius(19, 3, labs)
	room := skirtOuter(r, labs) + 5 // more room than it needs
	g, s := skirtFit(19, 3, room, labs)
	if g != 19 || s != 1 {
		t.Errorf("a ring with room to spare came back grip %.2f scale %.2f, want 19 and 1", g, s)
	}
}

// An unmeasured cell must not shrink anything. A zero maxOuter is "I do not
// know yet", not "no room at all" — the second reading would take every knob
// on a panel that has not been laid out down to its floor.
func TestAnUnmeasuredCellShrinksNothing(t *testing.T) {
	labs := ringOf(6, 30, 9)
	for _, room := range []float64{0, -1} {
		if g, s := skirtFit(19, 3, room, labs); g != 19 || s != 1 {
			t.Errorf("maxOuter %v gave grip %.2f scale %.2f, want the natural 19 and 1", room, g, s)
		}
	}
}

// The grip is spent before the legend is. A ring that fits once the knob is
// a little smaller must not also shrink the type — the type is what the
// silkscreen is for, and §2-103 gives knob size a range precisely so it can
// be the part that gives.
func TestTheGripGivesBeforeTheLegendDoes(t *testing.T) {
	labs := ringOf(5, 26, 9)
	natural := skirtOuter(skirtRadius(19, 3, labs), labs)
	// Just short of what it wants: reachable by shrinking the grip alone.
	g, s := skirtFit(19, 3, natural-3, labs)
	if s != 1 {
		t.Errorf("the legend was scaled to %.2f when a smaller grip would have done", s)
	}
	if g >= 19 {
		t.Errorf("the grip did not shrink: %.2f", g)
	}
	sc := skirtScaleLabels(labs, s)
	if got := skirtOuter(skirtRadius(g, 3, sc), sc); got > natural-3 {
		t.Errorf("it still reaches %.2f, past the %.2f it was given", got, natural-3)
	}
}

// When the grip is spent the legend shrinks, and the result actually fits.
func TestATightCellShrinksTheLegendAndFits(t *testing.T) {
	labs := ringOf(6, 34, 9)
	// Tighter than a smaller grip alone can reach, but not impossible: the
	// legend has to give as well. (Below about 41 for this ring both levers
	// are spent and it overhangs, which is what TestTheLeversHaveFloors
	// covers.)
	room := 45.0
	g, s := skirtFit(19, 3, room, labs)
	if s >= 1 {
		t.Errorf("the legend was not scaled: %.2f", s)
	}
	if g > 19*skirtMinGripFrac+1e-9 {
		t.Errorf("the legend shrank before the grip was spent (grip %.2f)", g)
	}
	sc := skirtScaleLabels(labs, s)
	if got := skirtOuter(skirtRadius(g, 3, sc), sc); got > room {
		t.Errorf("after fitting it still reaches %.2f, past %.2f", got, room)
	}
}

// Neither lever may run away: a cell far too small still leaves a knob you
// can grip and type you can read, overhanging rather than vanishing.
func TestTheLeversHaveFloors(t *testing.T) {
	g, s := skirtFit(19, 3, 1, ringOf(9, 40, 9))
	if g < 19*skirtMinGripFrac-1e-9 {
		t.Errorf("the grip went below its floor: %.2f", g)
	}
	if s < skirtMinLabelScale-1e-9 {
		t.Errorf("the legend went below its floor: %.2f", s)
	}
}

// Scaling a ring scales the boxes and leaves the angles alone — a legend set
// in smaller type is at the same position on the dial, not a different one.
func TestScalingALegendKeepsItsPosition(t *testing.T) {
	labs := ringOf(4, 20, 10)
	got := skirtScaleLabels(labs, 0.5)
	for i := range labs {
		if got[i].Deg != labs[i].Deg {
			t.Errorf("label %d moved from %v to %v", i, labs[i].Deg, got[i].Deg)
		}
		if got[i].W != labs[i].W*0.5 || got[i].H != labs[i].H*0.5 {
			t.Errorf("label %d scaled to %vx%v", i, got[i].W, got[i].H)
		}
	}
}

// The grip is a parameter because the panel has knobs of more than one size —
// a concentric stack's outer ring is the grip its skirt must clear. A bigger
// grip in the same cell has less room left for the ring, so it must give up
// more of itself than a small one would.
func TestABiggerGripGivesUpMoreRoom(t *testing.T) {
	labs := ringOf(5, 26, 9)
	room := skirtOuter(skirtRadius(19, 3, labs), labs) - 2

	small, _ := skirtFit(19, 3, room, labs)
	big, _ := skirtFit(30, 3, room, labs)
	if small >= 19 {
		t.Errorf("the 19px grip did not shrink at all: %.2f", small)
	}
	if big >= 30 {
		t.Errorf("the 30px grip did not shrink at all: %.2f", big)
	}
	if 30-big <= 19-small {
		t.Errorf("the big grip gave up %.2f and the small one %.2f", 30-big, 19-small)
	}
	// The small grip had enough to give and actually fits. The big one is at
	// its floor here and still overhangs slightly, which is the documented
	// outcome when both levers are spent — see TestTheLeversHaveFloors.
	if got := skirtOuter(skirtRadius(small, 3, labs), labs); got > room+1e-9 {
		t.Errorf("the small grip still reaches %.2f, past %.2f", got, room)
	}
	if big > 30*skirtMinGripFrac+1e-9 {
		t.Errorf("the big grip stopped at %.2f without reaching its floor", big)
	}
}

// The gap is a parameter for the same reason skirtRadius takes one: it is a
// design choice. A wider gap between grip and legend spends room the ring
// needed, so fitting the same ring in the same cell costs the grip more.
func TestAWiderGapCostsTheGripMore(t *testing.T) {
	labs := ringOf(5, 26, 9)
	room := skirtOuter(skirtRadius(19, 1, labs), labs)

	tight, _ := skirtFit(19, 1, room, labs)
	loose, _ := skirtFit(19, 8, room, labs)
	if tight != 19 {
		t.Errorf("at the gap it was measured with, the grip shrank to %.2f", tight)
	}
	if loose >= 19 {
		t.Errorf("seven more pixels of gap cost the grip nothing: %.2f", loose)
	}
}
