package attractor

import (
	"math"
	"sort"
	"testing"
)

// The self-steering Lorenz (docs/self-steering-lorenz.md): a Lorenz whose ρ
// follows its own z through a HEAD z route. Past a depth of about 0.33 it
// stops switching wings at random and pulses instead — seconds of quiet
// spiraling in one wing, then a break — and how it does depends on the delay
// in the loop as much as on the depth. These are the doc's numbers, held.

// selfSteer runs the Lorenz with ρ routed from head z at depth, the route's
// smoothing advanced filter 60 Hz frames a frame, for secs seconds of 60 Hz
// frames, and returns the times of its wing switches after the first ten
// seconds. Head z is normalized about the attractor's middle (z≈24) across its
// reach (26), standing in for the page's centering and fit.
func selfSteer(depth, filter float32, secs int) []float64 {
	const fps, steps, dt = 60, 10, 0.005
	x, y, z := 1.0, 1.0, 1.0
	sm := float32(0.5)
	side := 1.0
	var at []float64
	for frame := range fps * secs {
		sm = modelModSmooth(sm, modelModNorm(float32(z-24), 26), filter)
		rho := float64(ModTarget(28, depth, 2*sm-1, 1, 60, 0.1, 0, false))
		for range steps {
			dx, dy, dz := 10*(y-x), x*(rho-z)-y, x*y-8.0/3*z
			x, y, z = x+dx*dt, y+dy*dt, z+dz*dt
		}
		if s := math.Copysign(1, x); s != side {
			side = s
			if frame > fps*10 {
				at = append(at, float64(frame)/fps)
			}
		}
	}
	return at
}

// quietSpells are the gaps of a second or more between switches, and their
// median.
func quietSpells(at []float64) (n int, median float64) {
	var gaps []float64
	for i := 1; i < len(at); i++ {
		if g := at[i] - at[i-1]; g >= 1 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return 0, 0
	}
	sort.Float64s(gaps)
	return len(gaps), gaps[len(gaps)/2]
}

func TestASelfSteeringLorenzPulses(t *testing.T) {
	_, plain := quietSpells(selfSteer(0, 1, 300))
	_, pulse := quietSpells(selfSteer(0.38, 1, 300))
	if !(plain < 2) {
		t.Errorf("the plain Lorenz's quiet spells have a median of %.1fs, want under 2", plain)
	}
	if !(pulse > 3.5) {
		t.Errorf("at depth 0.38 the quiet spells have a median of %.1fs, want a pulse of more than 3.5", pulse)
	}
	// The delay in the loop: a route that follows z twice as fast catches
	// every excursion and holds the orbit in one wing for good.
	if at := selfSteer(0.38, 2, 300); len(at) != 0 {
		t.Errorf("with the feedback twice as fast it switched wings %d times, want it held in one", len(at))
	}
}
