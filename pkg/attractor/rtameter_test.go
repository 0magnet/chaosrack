package attractor

import (
	"math"
	"testing"
)

// Advancing the ballistics by k frames at once is k of the page's frames:
// the bars and peaks land where k single steps put them.
func TestRTAAdvanceIsFrameRateFree(t *testing.T) {
	one, three := NewRTA(), NewRTA()
	x := make([]float32, RTAFFT)
	for i := range x {
		x[i] = float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/48000))
	}
	for _, m := range []*RTA{one, three} {
		m.Analyze(x, 48000)
	}
	for range 3 {
		one.Advance(1)
	}
	three.Advance(3)
	for i := range one.held {
		if math.Abs(one.held[i]-three.held[i]) > 1e-9 || math.Abs(one.peaks[i]-three.peaks[i]) > 1e-9 {
			t.Fatalf("band %d: three frames hold %.4f peak %.4f; one advance of three, %.4f and %.4f",
				i, one.held[i], one.peaks[i], three.held[i], three.peaks[i])
		}
	}
}
