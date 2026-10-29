package attractor

import (
	"math"
	"testing"
)

func TestFeatLitIsTheNearestSegment(t *testing.T) {
	for _, c := range []struct {
		v    float32
		want int
	}{
		{0, 0}, {-1, 0}, {float32(math.NaN()), 0}, {0.024, 0}, {0.026, 1},
		{0.5, 10}, {1, featSegs}, {7, featSegs},
	} {
		if got := featLit(c.v); got != c.want {
			t.Errorf("featLit(%v) = %d, want %d", c.v, got, c.want)
		}
	}
}

// The canvas colors the segments where the stylesheet's gradient does: green
// to 70%, amber to 90%, red above.
func TestLadderTintsSplitAsTheStylesheetDoes(t *testing.T) {
	var n [3]int
	for k := range featSegs {
		n[ladderTint(k)]++
	}
	if n != [3]int{14, 4, 2} {
		t.Fatalf("green, amber, red = %v, want [14 4 2]", n)
	}
	if ladderTint(13) != 0 || ladderTint(14) != 1 || ladderTint(18) != 2 {
		t.Fatal("a boundary segment has the wrong color")
	}
}
