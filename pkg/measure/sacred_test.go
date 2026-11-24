package measure

import (
	"math"
	"testing"

	"github.com/0magnet/chaosrack/pkg/sacred"
)

// The real vesica, as the page draws it — normalized, in pieces — measured
// with nothing told about it: its centers are found, its circles meet
// exactly, and the chord between their meeting points is √3 of the line of
// centers. What Frank Chester points out, measured rather than stated.
func TestMeasuringTheVesica(t *testing.T) {
	l := sacred.Form(sacred.Vesica, 1, 0, 2)
	var s Scene
	for _, e := range l.E {
		a, b := l.P[e[0]], l.P[e[1]]
		s.Segs = append(s.Segs, [2]P{P(a), P(b)})
	}
	s.Prepare()
	if len(s.Marks) != 2 {
		t.Fatalf("%d centers found, want the vesica's 2", len(s.Marks))
	}
	var top, bot P
	for _, m := range s.meets {
		if m[1] > top[1] {
			top = m
		}
		if m[1] < bot[1] {
			bot = m
		}
	}
	unit := Dist(s.Marks[0], s.Marks[1])
	ratio := Dist(top, bot) / unit
	if math.Abs(ratio-math.Sqrt(3)) > 1e-9 {
		t.Errorf("chord / centers = %v, want √3", ratio)
	}
	if name, ok := Recognize(ratio); !ok || name != "√3" {
		t.Errorf("named %q", name)
	}
}
