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
