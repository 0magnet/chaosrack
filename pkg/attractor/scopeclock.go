package attractor

import (
	"math"
	"sort"
	"time"
)

// A clock, drawn the way a scope draws anything: as one beam tour.
//
// The face, the twelve ticks and the three hands are a SINGLE path, walked in
// order, because that is what an X/Y display does — it has one beam and no way
// to lift it. A jump between the end of one stroke and the start of the next is
// therefore DRAWN, and the first version of this was mostly jumps: the beam
// finished each tick at the rim and flew to the next tick's inner end, twelve
// long chords that made a ring of bright triangles louder than the dial itself.
//
// So the tour is ordered until nothing is left to hide. The beam walks the rim
// once and dips inward wherever a tick falls; it closes the rim AT THE SECOND
// HAND'S ANGLE and runs straight down that hand to the center; and the minute
// hand's return retraces the minute hand. Every segment in the finished tour is
// either dial or hand — which is not a compromise on the instrument but the way
// a scope clock was actually programmed, because the transits were what you
// spent your effort getting rid of.
//
// IT IS RESAMPLED BY ARC LENGTH rather than given a fixed number of points per
// stroke. The strokes are wildly different lengths — the face is a full circle,
// a tick is a few hundredths — so points-per-stroke would put most of the beam
// on the ticks and leave the circle a dotted line. Even spacing along the path
// is what makes the trace read as one continuous brightness, which is also how
// a real scope behaves: the beam moves at a rate, not at a number of samples.

// clockPt is a point on the beam's path, in the display's own [-1,1] square.
type clockPt struct{ x, y float64 }

// clockPolyline builds the whole tour for a given instant.
//
// Angles run clockwise from twelve o'clock, which is π/2 in the maths and the
// reason for the minus: a clock is one of the few things that is drawn the
// other way round from a unit circle, and getting it wrong is a clock that runs
// backwards while looking almost right.
func clockPolyline(now time.Time) []clockPt {
	var p []clockPt
	at := func(frac, r float64) clockPt {
		a := math.Pi/2 - 2*math.Pi*frac
		return clockPt{r * math.Cos(a), r * math.Sin(a)}
	}

	// Fractional throughout: the hour hand creeps with the minutes and the
	// minute hand with the seconds, because a clock whose hands jump is a clock
	// that looks stopped between the jumps.
	sec := float64(now.Second()) + float64(now.Nanosecond())/1e9
	minute := float64(now.Minute()) + sec/60
	hour := float64(now.Hour()%12) + minute/60
	secFrac := sec / 60

	// The rim, its ticks cut into it, walked from the second hand's angle round
	// to the same angle.
	//
	// Starting there is what removes the last stray line. A rim that began at
	// twelve had to end at twelve, and getting from there to the center for the
	// hands drew a radial stroke at twelve o'clock that looked exactly like a
	// fourth hand — permanently pointing at noon, on a clock, which is the one
	// artifact this drawing could least afford. Beginning at the second hand
	// instead means the rim closes exactly where the second hand's tip is, and
	// the run to the center IS the second hand.
	const face = 0.95
	const faceSegs = 144

	// The rim samples and the twelve hour marks, merged and walked in the order
	// the beam meets them. They have to be merged rather than interleaved by
	// index: the walk no longer starts at twelve, so an hour mark generally
	// falls BETWEEN two rim samples instead of on one.
	type rimNode struct {
		f    float64 // position round the dial, 0 = twelve o'clock
		d    float64 // distance traveled from the start, in turns
		tick float64 // inner radius of a tick here, 0 for a plain rim sample
	}
	turnsFrom := func(f float64) float64 {
		d := math.Mod(f-secFrac, 1)
		if d < 0 {
			d++
		}
		return d
	}
	nodes := make([]rimNode, 0, faceSegs+13)
	for i := 0; i < faceSegs; i++ {
		f := secFrac + float64(i)/faceSegs
		nodes = append(nodes, rimNode{f: f, d: float64(i) / faceSegs})
	}
	for h := 0; h < 12; h++ {
		f := float64(h) / 12
		// Longer at the quarters — the hierarchy a real dial has, and what makes
		// the orientation readable at a glance.
		in := 0.84
		if h%3 == 0 {
			in = 0.74
		}
		nodes = append(nodes, rimNode{f: f, d: turnsFrom(f), tick: in})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].d < nodes[j].d })
	for _, n := range nodes {
		p = append(p, at(n.f, face))
		if n.tick > 0 {
			p = append(p, at(n.f, n.tick), at(n.f, face))
		}
	}
	p = append(p, at(secFrac, face)) // close the rim, on the second hand

	// The hands, in the one order that leaves no spare segment: down the second
	// hand to the center, out and back along the minute hand, out along the
	// hour hand. The minute hand's return retraces itself, so the only lines on
	// screen are the three hands.
	//
	// The second hand reaches the rim, as it does on a real dial — and here that
	// is load-bearing rather than decorative: a shorter one would leave a gap
	// between the rim and the tip that the beam would have to cross.
	p = append(p, clockPt{0, 0})
	p = append(p, at(minute/60, 0.76), clockPt{0, 0})
	p = append(p, at(hour/12, 0.5))
	return p
}
