package attractor

import (
	"math"
	"time"

	"github.com/0magnet/chaosrack/pkg/scope"
)

// ClockTrace hands put the scope clock's face at now, steps points spaced
// evenly along it, as the beam draws it: shared by the page
// (scopeclock_js.go) and a native rack.
func ClockTrace(now time.Time, steps int, put func(i int, x, y float64)) bool {
	path := scope.ClockPolyline(now)
	if len(path) < 2 || steps < 2 {
		return false
	}
	// Cumulative arc length, so the beam can be placed at even distances along
	// the whole path rather than evenly per stroke.
	cum := make([]float64, len(path))
	for i := 1; i < len(path); i++ {
		dx := path[i].X - path[i-1].X
		dy := path[i].Y - path[i-1].Y
		cum[i] = cum[i-1] + math.Hypot(dx, dy)
	}
	total := cum[len(cum)-1]
	if total <= 0 {
		return false
	}
	seg := 1
	for i := range steps {
		want := total * float64(i) / float64(steps-1)
		for seg < len(cum)-1 && cum[seg] < want {
			seg++
		}
		a, b := path[seg-1], path[seg]
		span := cum[seg] - cum[seg-1]
		t := 0.0
		if span > 0 {
			t = (want - cum[seg-1]) / span
		}
		put(i, a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
	}
	return true
}
