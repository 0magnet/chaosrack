package dynamics

import (
	"math"
	"math/rand/v2"
)

// Swarm is many particles carried through one flow at once, the way
// spectrometry.mp4's attractor engine draws: nothing is traced, and the
// figure is wherever the particles crowd. Seeded as a tight ball on the
// attractor, it shows what a single trail cannot, the ball smearing out along
// the flow until it covers the whole attractor.
//
// RK4 for N particles is N times one particle's cost and no more: the points
// are the same four field evaluations each, so the drawing, not the
// integration, is what a large swarm costs.
type Swarm struct {
	P   [][3]float64
	key string
	rng *rand.Rand
	// ext is the attractor's size, measured when seeded: the scale the seed
	// ball and the escape guard are set against.
	ext float64
}

// SwarmSeedSpread is the seed ball's radius as a share of the attractor's
// size: small enough to start as one bright dot, large enough to spread
// within seconds on a chaotic flow.
const SwarmSeedSpread = 1e-3

// Ensure seeds the swarm for key with n particles if it is not already that
// swarm, and resizes it to n if only the count changed. A resized swarm
// keeps its particles: new ones are copies of old ones, a hair apart, so a
// turned knob grows the cloud where it is instead of restarting it.
func (s *Swarm) Ensure(key string, n int, dt float64, f Deriv, ic [3]float64) {
	n = max(n, 1)
	if s.rng == nil {
		s.rng = rand.New(rand.NewPCG(1, 2)) //nolint:gosec // particle jitter, not security
	}
	if s.key != key || len(s.P) == 0 {
		s.seed(key, n, dt, f, ic)
		return
	}
	switch {
	case n < len(s.P):
		s.P = s.P[:n]
	case n > len(s.P):
		for len(s.P) < n {
			s.P = append(s.P, s.near(s.P[s.rng.IntN(len(s.P))]))
		}
	}
}

// Reseed forgets the swarm, so the next Ensure starts it again as a ball.
func (s *Swarm) Reseed() { s.key = "" }

// seed runs the flow past its transient from ic, measures the attractor, and
// places n particles in a ball around the point reached.
func (s *Swarm) seed(key string, n int, dt float64, f Deriv, ic [3]float64) {
	s.key = key
	x, y, z := ic[0], ic[1], ic[2]
	steps := max(int(50/dt), 2000)
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := range steps {
		x, y, z = RK4(f, dt, x, y, z)
		if !Bounded(x, y, z) {
			x, y, z = ic[0], ic[1], ic[2]
		}
		if i >= steps/2 {
			for a, v := range [3]float64{x, y, z} {
				lo[a], hi[a] = math.Min(lo[a], v), math.Max(hi[a], v)
			}
		}
	}
	s.ext = math.Max(math.Max(hi[0]-lo[0], hi[1]-lo[1]), hi[2]-lo[2])
	if !(s.ext > 0) || math.IsInf(s.ext, 0) {
		s.ext = 1
	}
	at := [3]float64{x, y, z}
	s.P = s.P[:0]
	for range n {
		s.P = append(s.P, s.near(at))
	}
}

// near is a point a seed ball's radius from p, in a random direction.
func (s *Swarm) near(p [3]float64) [3]float64 {
	r := SwarmSeedSpread * s.ext
	return [3]float64{p[0] + r*s.rng.NormFloat64(), p[1] + r*s.rng.NormFloat64(), p[2] + r*s.rng.NormFloat64()}
}

// Step advances every particle by steps steps of dt, by RK4 or, with euler,
// forward Euler. A particle that escapes is put back beside another one
// rather than dropped, so the count stays what was asked for.
func (s *Swarm) Step(f Deriv, dt float64, steps int, euler bool) {
	lim := 1e3 * math.Max(s.ext, 1)
	for i := range s.P {
		x, y, z := s.P[i][0], s.P[i][1], s.P[i][2]
		for range steps {
			if euler {
				dx, dy, dz := f(x, y, z)
				x, y, z = x+dt*dx, y+dt*dy, z+dt*dz
			} else {
				x, y, z = RK4(f, dt, x, y, z)
			}
		}
		// Plain comparisons: math.Abs here was a sizeable share of the frame
		// at twenty thousand particles. NaN fails every one of them.
		if !(x < lim && x > -lim && y < lim && y > -lim && z < lim && z > -lim) {
			s.P[i] = s.near(s.P[s.rng.IntN(len(s.P))])
			continue
		}
		s.P[i] = [3]float64{x, y, z}
	}
}

// Spread is the swarm's root-mean-square distance from its own centroid, as
// a share of the attractor's size: about SwarmSeedSpread just after seeding,
// and a sizeable fraction once the swarm covers the attractor.
func (s *Swarm) Spread() float64 {
	if len(s.P) == 0 {
		return 0
	}
	var c [3]float64
	for _, p := range s.P {
		c[0], c[1], c[2] = c[0]+p[0], c[1]+p[1], c[2]+p[2]
	}
	n := float64(len(s.P))
	c = [3]float64{c[0] / n, c[1] / n, c[2] / n}
	var sum float64
	for _, p := range s.P {
		dx, dy, dz := p[0]-c[0], p[1]-c[1], p[2]-c[2]
		sum += dx*dx + dy*dy + dz*dz
	}
	return math.Sqrt(sum/n) / s.ext
}
