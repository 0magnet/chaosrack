package dynamics

import (
	"math"
	"testing"
)

// The point of a swarm on a chaotic flow: seeded as one dot, it spreads over
// the attractor. A swarm that stays a dot shows nothing a trail does not.
func TestSwarmSpreadsOverTheAttractor(t *testing.T) {
	dt, f, ok := FlowFor("lorenz")
	if !ok {
		t.Fatal("no lorenz flow")
	}
	ic := InitCondFor("lorenz")
	var s Swarm
	s.Ensure("lorenz", 2000, dt, f, [3]float64{float64(ic[0]), float64(ic[1]), float64(ic[2])})
	if got := s.Spread(); got > 3*SwarmSeedSpread {
		t.Fatalf("a fresh swarm should be one dot, spread %.2g", got)
	}
	for range 600 { // 6 time units at Lorenz's dt
		s.Step(f, dt, 2, false)
	}
	if got := s.Spread(); got < 0.15 {
		t.Errorf("after 6 time units the swarm is still a clump, spread %.3f", got)
	}
}

// Changing the count keeps the swarm where it is, and the count is exact.
func TestSwarmResizesInPlace(t *testing.T) {
	dt, f, _ := FlowFor("lorenz")
	var s Swarm
	s.Ensure("lorenz", 100, dt, f, [3]float64{1, 1, 1})
	first := s.P[0]
	s.Ensure("lorenz", 250, dt, f, [3]float64{1, 1, 1})
	if len(s.P) != 250 || s.P[0] != first {
		t.Fatalf("grew to %d, first particle %v → %v", len(s.P), first, s.P[0])
	}
	s.Ensure("lorenz", 40, dt, f, [3]float64{1, 1, 1})
	if len(s.P) != 40 {
		t.Fatalf("shrank to %d", len(s.P))
	}
	s.Ensure("rossler", 40, dt, f, [3]float64{1, 1, 1})
	if s.key != "rossler" {
		t.Fatal("a new model did not reseed")
	}
}

// A particle that escapes is put back near the swarm, never left as NaN.
func TestSwarmEscapedParticlesRejoin(t *testing.T) {
	dt, f, _ := FlowFor("lorenz")
	var s Swarm
	s.Ensure("lorenz", 50, dt, f, [3]float64{1, 1, 1})
	s.P[7] = [3]float64{math.NaN(), 0, 0}
	s.P[9] = [3]float64{1e12, 0, 0}
	s.Step(f, dt, 1, false)
	for i, p := range s.P {
		if !Bounded(p[0], p[1], p[2]) || math.Abs(p[0]) > 1e3 {
			t.Fatalf("particle %d is %v", i, p)
		}
	}
}
