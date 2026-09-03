package attractor

import (
	"math"
	"testing"
	"time"
)

// clockAt is a fixed instant, so a failure reads the same every run.
func clockAt(h, m, s int) time.Time {
	return time.Date(2026, 8, 22, h, m, s, 0, time.UTC)
}

// THE CLOCK HAS TO RUN THE RIGHT WAY ROUND.
//
// This is the one error in the whole drawing that looks almost correct: a
// clock with the sign flipped still has a face, still has ticks, still has
// three hands sweeping smoothly — it just runs backwards, and at a glance from
// across a room nothing about it is obviously wrong. Screenshots would not
// catch it; a person watching for a minute might not.
//
// So the four cardinal positions are pinned by where they land on screen, in
// the display's own coordinates where +x is right and +y is up.
func TestHandsPointWhereTheHourSays(t *testing.T) {
	// The minute hand's length; the tip is divided by it so the expectations
	// below are plain unit directions.
	const minuteHand = 0.76
	cases := []struct {
		h, m         int
		wantX, wantY float64
		where        string
	}{
		{12, 0, 0, 1, "straight up (twelve)"},
		{3, 15, 1, 0, "right (quarter past)"},
		{6, 30, 0, -1, "straight down (half past)"},
		{9, 45, -1, 0, "left (quarter to)"},
	}
	for _, c := range cases {
		// The minute hand is the second-to-last stroke of the tour.
		p := clockPolyline(clockAt(c.h, c.m, 0))
		tip := p[len(p)-3]
		gotX, gotY := tip.x/minuteHand, tip.y/minuteHand
		if math.Abs(gotX-c.wantX) > 1e-9 || math.Abs(gotY-c.wantY) > 1e-9 {
			t.Errorf("at :%02d the minute hand points (%.3f,%.3f), want %s (%.0f,%.0f) — "+
				"a sign flip here draws a clock that runs backwards and still looks like a clock",
				c.m, gotX, gotY, c.where, c.wantX, c.wantY)
		}
	}
}

// NO SEGMENT OF THE TOUR IS A STRAY LINE.
//
// This is the property the drawing was rebuilt around, and it is worth pinning
// because the failure is silent and cumulative: the first version flew from
// each tick's outer end to the next tick's inner end, and the twelve chords
// that produced were brighter than the dial they were supposed to decorate.
// Nothing errored. It simply looked wrong, and only looking would have told.
//
// Every segment must be one of two things — a short step along the rim, or a
// RADIAL stroke, both ends on the same spoke. A tick dip is radial, a hand is
// radial, a hand's retrace is radial. A chord across the face is neither, which
// is exactly what makes this test able to see one.
func TestEverySegmentIsDialOrHand(t *testing.T) {
	// One rim step at 144 segments, plus slack for the two short steps a tick
	// mark cuts either side of itself when it lands between samples.
	const rimStep = 2 * math.Pi * 0.95 / 144
	for _, tm := range []time.Time{clockAt(0, 0, 0), clockAt(10, 9, 8), clockAt(23, 59, 59), clockAt(6, 30, 30)} {
		p := clockPolyline(tm)
		for i := 1; i < len(p); i++ {
			a, b := p[i-1], p[i]
			if math.Hypot(b.x-a.x, b.y-a.y) <= rimStep*1.5 {
				continue // a step along the rim
			}
			// Radial: the two ends share a spoke. Cross product of the two
			// position vectors is zero when they are collinear through the
			// origin, which is what "same spoke" means.
			if math.Abs(a.x*b.y-a.y*b.x) < 1e-9 {
				continue
			}
			t.Errorf("at %s, segment %d of %d runs from (%.3f,%.3f) to (%.3f,%.3f): "+
				"too long for the rim and not radial, so it is a stray line drawn across the face",
				tm.Format("15:04:05"), i, len(p), a.x, a.y, b.x, b.y)
			break
		}
	}
}

// The rim must close where the second hand is, because that is the whole trick
// that removed the false fourth hand at twelve o'clock. If someone later moves
// the rim's start back to twelve, this says so.
func TestRimClosesOnTheSecondHand(t *testing.T) {
	for _, s := range []int{0, 7, 15, 38, 59} {
		p := clockPolyline(clockAt(4, 20, s))
		// The tour is: rim … rim-close, center, minute tip, center, hour tip.
		closePt := p[len(p)-5]
		center := p[len(p)-4]
		if math.Hypot(center.x, center.y) > 1e-12 {
			t.Fatalf("the point after the rim is (%.3f,%.3f), not the center — the tour's "+
				"shape changed and this test no longer measures what it claims",
				center.x, center.y)
		}
		frac := float64(s) / 60
		wantX := 0.95 * math.Cos(math.Pi/2-2*math.Pi*frac)
		wantY := 0.95 * math.Sin(math.Pi/2-2*math.Pi*frac)
		if math.Abs(closePt.x-wantX) > 1e-9 || math.Abs(closePt.y-wantY) > 1e-9 {
			t.Errorf("at :%02d the rim closes at (%.3f,%.3f), want the second hand's tip "+
				"(%.3f,%.3f) — closing anywhere else draws a spare radial line to the center",
				s, closePt.x, closePt.y, wantX, wantY)
		}
	}
}

// All twelve hour marks have to be there, at the right depth.
//
// The marks are merged into the rim by sorting, so a mistake in the ordering
// drops or duplicates one rather than misplacing it visibly — an eleven-hour
// dial is not something the eye counts.
func TestTwelveTicksAtTheRightDepths(t *testing.T) {
	p := clockPolyline(clockAt(1, 23, 45))
	quarters, hours := 0, 0
	for _, pt := range p {
		switch r := math.Hypot(pt.x, pt.y); {
		case math.Abs(r-0.74) < 1e-9:
			quarters++
		case math.Abs(r-0.84) < 1e-9:
			hours++
		}
	}
	if quarters != 4 {
		t.Errorf("%d marks at the quarter depth, want 4 (twelve, three, six, nine)", quarters)
	}
	if hours != 8 {
		t.Errorf("%d marks at the hour depth, want 8 (the hours that are not quarters)", hours)
	}
}

// The hour hand creeps rather than jumping, or the clock looks stopped for
// fifty-nine minutes out of every sixty.
func TestTheHourHandCreeps(t *testing.T) {
	a := clockPolyline(clockAt(4, 0, 0))
	b := clockPolyline(clockAt(4, 30, 0))
	ha, hb := a[len(a)-1], b[len(b)-1]
	if math.Hypot(hb.x-ha.x, hb.y-ha.y) < 1e-6 {
		t.Error("the hour hand is in the same place at 4:00 and 4:30 — it is stepping " +
			"on the hour instead of creeping with the minutes")
	}
	// Half an hour is half of one hour mark: 15 degrees.
	angA := math.Atan2(ha.y, ha.x)
	angB := math.Atan2(hb.y, hb.x)
	moved := math.Mod(angA-angB+2*math.Pi, 2*math.Pi) // clockwise is decreasing
	if want := 15 * math.Pi / 180; math.Abs(moved-want) > 1e-9 {
		t.Errorf("the hour hand moved %.3f° in half an hour, want 15°", moved*180/math.Pi)
	}
}
