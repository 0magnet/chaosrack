package rackhost

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// trail is a flow integrated as the page integrates it: each frame a fresh
// run of Trail points, continuing from where the last one ended, so the
// figure is redrawn by the motion along it rather than shown once.
type trail struct {
	key     string
	x, y, z float64
	pts     [][3]float64
}

// trailBudget is the most integration steps one frame does. The page's is
// 4,000,000 Euler steps on a GPU's schedule; this one runs RK4 on the CPU
// between terminal frames, and a long Trail is cut short here rather than
// the frame held up: the run is as long as the budget allows.
const trailBudget = 200_000

// speedSteps is the Speed knob as the page applies it: a log10 setting, 0 for
// one step a point, whole sub-steps above 1, a shorter step below.
func speedSteps(logVal float64) (sub int, scale float64) {
	speed := math.Pow(10, logVal)
	if speed >= 1 {
		return int(speed + 0.5), 1
	}
	return 1, speed
}

// advance runs the flow key on for n points of the trail at speed (the Speed
// knob's setting). It reports false for a flow the rack cannot step, which
// is drawn from its fixed trajectory instead.
func (t *trail) advance(key string, n int, speed float64) bool {
	dt, f, ok := dynamics.FlowFor(key)
	if !ok || dt <= 0 {
		return false
	}
	if key != t.key {
		t.key = key
		t.seed()
	}
	sub, scale := speedSteps(speed)
	if n < 2 {
		n = 2
	}
	// The budget is spent on sub-steps first, as the page's effSubSteps does,
	// and then on the run's length.
	sub = max(min(sub, trailBudget/n), 1)
	n = min(n, trailBudget/sub)
	h := dt * scale
	if cap(t.pts) < n {
		t.pts = make([][3]float64, n)
	}
	t.pts = t.pts[:n]
	for i := range n {
		for range sub {
			t.x, t.y, t.z = dynamics.RK4(f, h, t.x, t.y, t.z)
		}
		if gone(t.x, t.y, t.z) {
			// A constant that throws the flow off its attractor: start it
			// again, as the page reseeds, rather than draw a line to nowhere.
			t.seed()
		}
		t.pts[i] = [3]float64{t.x, t.y, t.z}
	}
	return true
}

func (t *trail) seed() {
	ic := dynamics.InitCondFor(t.key)
	t.x, t.y, t.z = float64(ic[0]), float64(ic[1]), float64(ic[2])
}

// gone is the page's test for a flow that has left: not a number, or past
// 1e4 on any axis.
func gone(x, y, z float64) bool {
	const lim = 1e4
	return math.IsNaN(x+y+z) || math.Abs(x) > lim || math.Abs(y) > lim || math.Abs(z) > lim
}
