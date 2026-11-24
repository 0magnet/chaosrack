package measure

import (
	"math"
	"testing"
)

// The vesica, as a figure tells it: two unit circles 1 apart, the line of
// centers. Where the circles meet is solved exactly, and the chord between
// the two meeting points is √3 — Chester's number.
func TestTheVesicaChordIsRootThree(t *testing.T) {
	s := Scene{
		Segs:    polyCircle(P{-0.5, 0, 0}),
		Circles: []Circle{{P{-0.5, 0, 0}, 1}, {P{0.5, 0, 0}, 1}},
		Marks:   []P{{-0.5, 0, 0}, {0.5, 0, 0}},
	}
	s.Segs = append(s.Segs, polyCircle(P{0.5, 0, 0})...)
	s.Prepare()
	face := func(p P) (float64, float64, bool) { return 100 * p[0], -100 * p[1], true }
	top, ok1 := s.Snap(1, -85, face, 12)
	bot, ok2 := s.Snap(-1, 87, face, 12)
	if !ok1 || !ok2 || top.Kind != Meet || bot.Kind != Meet {
		t.Fatalf("snapped %v %v (ok %v %v), want the two exact meeting points", top, bot, ok1, ok2)
	}
	chord := Dist(top.P, bot.P)
	if math.Abs(chord-math.Sqrt(3)) > 1e-12 {
		t.Errorf("chord %v, want √3", chord)
	}
	if name, ok := Recognize(chord); !ok || name != "√3" {
		t.Errorf("%v named %q", chord, name)
	}
	// A center is a mark, and wins over the meeting points' corner kin.
	c, ok := s.Snap(-49, 2, face, 12)
	if !ok || c.Kind != Mark || c.P != (P{-0.5, 0, 0}) {
		t.Errorf("snapped %v, want the left center", c)
	}
}

// Segments crossing on screen but passing at different depths do not meet.
func TestCrossingsMustMeetInSpace(t *testing.T) {
	s := Scene{Segs: [][2]P{{{-1, 0, 0}, {1, 0, 0}}, {{0, -1, 1}, {0, 1, 1}}}}
	s.Prepare()
	flat := func(p P) (float64, float64, bool) { return 100 * p[0], 100 * p[1], true }
	if got, ok := s.Snap(0, 0, flat, 10); ok && got.Kind == Crossing {
		t.Errorf("a crossing at %v, where the segments are 1 apart in depth", got.P)
	}
	s.Segs[1] = [2]P{{0, -1, 0}, {0, 1, 0}}
	s.Prepare()
	if got, ok := s.Snap(1, 1, flat, 10); !ok || got.Kind != Crossing || got.P.norm() > 1e-12 {
		t.Errorf("snapped %v %v, want the crossing at the origin", got, ok)
	}
}

// A smooth curve drawn in pieces has no corners; a polygon has its own.
func TestCornersAreTurnsNotJoints(t *testing.T) {
	if n := len(corners(polyCircle(P{}), 1e-9)); n != 0 {
		t.Errorf("a circle has %d corners", n)
	}
	var sq [][2]P
	pts := []P{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}}
	for i := range pts {
		sq = append(sq, [2]P{pts[i], pts[(i+1)%4]})
	}
	if n := len(corners(sq, 1e-9)); n != 4 {
		t.Errorf("a square has %d corners", n)
	}
}

func TestAngle(t *testing.T) {
	if a := Angle(P{1, 0, 0}, P{}, P{0, 1, 0}); math.Abs(a-90) > 1e-12 {
		t.Errorf("%v°", a)
	}
	// The pentagon's interior angle.
	c := func(k int) P {
		a := math.Pi/2 + 2*math.Pi*float64(k)/5
		return P{math.Cos(a), math.Sin(a), 0}
	}
	a := Angle(c(0), c(1), c(2))
	if name, ok := RecognizeAngle(a); !ok || name != "108° = 3π/5" {
		t.Errorf("%v° named %q", a, name)
	}
}

func TestRecognize(t *testing.T) {
	phi := (1 + math.Sqrt(5)) / 2
	for x, want := range map[float64]string{
		math.Sqrt(3):       "√3",
		math.Sqrt(3) / 2:   "√3/2",
		phi:                "φ",
		1 / (phi * phi):    "1/φ²",
		2:                  "2",
		0.5:                "1/2",
		math.Sqrt(2) * 1.0: "√2",
	} {
		if got, ok := Recognize(x * (1 + 3e-4)); !ok || got != want {
			t.Errorf("%v named %q, want %q", x, got, want)
		}
	}
	if got, ok := Recognize(1.2345); ok {
		t.Errorf("1.2345 named %q; it has no simple name", got)
	}
}

func polyCircle(c P) [][2]P {
	const n = 96 // pieces, as Sacred Geometry draws a circle
	const r = 1
	var out [][2]P
	at := func(k int) P {
		a := 2 * math.Pi * float64(k) / float64(n)
		return P{c[0] + r*math.Cos(a), c[1] + r*math.Sin(a), c[2]}
	}
	for k := range n {
		out = append(out, [2]P{at(k), at(k + 1)})
	}
	return out
}

// With nothing said about it, a figure's circles and lines are read off its
// segments: the vesica drawn as two 96-gons and a line still meets exactly.
func TestDetectFindsCirclesAndLines(t *testing.T) {
	segs := append(polyCircle(P{-0.5, 0, 0}), polyCircle(P{0.5, 0, 0})...)
	segs = append(segs, [2]P{{-1.5, 0, 0}, {1.5, 0, 0}})
	s := Scene{Segs: segs}
	s.Prepare()
	if len(s.Circles) != 2 || len(s.Lines) != 1 {
		t.Fatalf("%d circles, %d lines; want 2 and 1", len(s.Circles), len(s.Lines))
	}
	if len(s.Marks) != 2 {
		t.Errorf("%d centers, want 2", len(s.Marks))
	}
	// The circles meet at (0, ±√3/2); each meets the line twice.
	want := []P{{0, math.Sqrt(3) / 2, 0}, {0, -math.Sqrt(3) / 2, 0}}
	for _, w := range want {
		found := false
		for _, m := range s.meets {
			if m.near(w, 1e-9) {
				found = true
			}
		}
		if !found {
			t.Errorf("no exact meeting point at %v among %v", w, s.meets)
		}
	}
}

// An arc's missing part adds no meeting points.
func TestArcsMeetOnlyWhereDrawn(t *testing.T) {
	full := polyCircle(P{-0.5, 0, 0})
	half := polyCircle(P{0.5, 0, 0})[:48] // the upper half only
	s := Scene{Segs: append(full, half...)}
	s.Prepare()
	for _, m := range s.meets {
		if m[1] < 0 {
			t.Errorf("a meeting point at %v, on the missing half", m)
		}
	}
}
