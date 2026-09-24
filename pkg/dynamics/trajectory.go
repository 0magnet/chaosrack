package dynamics

import (
	"math"
	"sort"
)

// Trajectories of the registered flows, for anything outside the browser that
// wants the curve itself rather than a picture of it — the STL export, and
// the chaos guard's sibling tests.
//
// The vector fields, the timesteps and the initial conditions all come from
// the same registry the renderer integrates, so a path exported here is the
// path the app draws, not a second implementation of the same equations.

// Keys is every system with a registered vector field, sorted. 4-D systems
// are included: what they trace is the (x,y,z) projection, which is what the
// renderer draws.
//
// Sorted rather than in the catalog's order, which is what it used to be. The
// catalog is the MODE SELECTOR — labels, groups, descriptions, the order a
// knob turns through — and a package about vector fields has no business
// knowing about it. Sorting is stable for the same reason and costs nothing;
// a caller that wants them in panel order has the catalog and can say so.
func Keys() []string {
	out := make([]string, 0, len(flowSystems)+len(flowSystems4))
	for k := range flowSystems {
		out = append(out, k)
	}
	for k := range flowSystems4 {
		if _, dup := flowSystems[k]; !dup {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// HasFlow reports whether a mode has a registered vector field. Parametric
// curves and geometry do not.
func HasFlow(mode string) bool {
	if _, ok := flowSystems[mode]; ok {
		return true
	}
	_, ok := flowSystems4[mode]
	return ok
}

// TrajectoryOptions controls how much of a flow is traced.
type TrajectoryOptions struct {
	// Transient is how long to integrate before recording, in the system's
	// own time units. A trajectory recorded from its initial condition starts
	// with a run-in from wherever that point was to the attractor, which is
	// not part of the attractor and looks like a stray whisker on a model.
	Transient float64

	// Duration is how long to record for, in the same units.
	Duration float64

	// MaxPoints caps the returned path, thinning by taking every Nth step.
	// A tube swept along a million points is a mesh no viewer will open — and
	// the app's own STL loader indexes with 16-bit indices, so a model wants
	// to stay under 65535 vertices: 4000 rings of 8 is 32000 of them.
	MaxPoints int
}

// DefaultTrajectory is a reasonable trace of any registered flow: past the
// run-in, long enough to close the figure, thinned to something a mesh can
// carry.
func DefaultTrajectory() TrajectoryOptions {
	return TrajectoryOptions{Transient: 20, Duration: 220, MaxPoints: 4000}
}

// Trajectory integrates a registered flow from its own initial condition and
// returns the path. It returns nil for a mode with no vector field.
func Trajectory(mode string, o TrajectoryOptions) [][3]float64 {
	if s4, ok := flowSystems4[mode]; ok {
		return trajectory4(s4, mode, o)
	}
	sys, ok := flowSystems[mode]
	if !ok {
		return nil
	}
	dt := sys.dt()
	if dt <= 0 {
		return nil
	}
	if o.Duration <= 0 {
		o = DefaultTrajectory()
	}
	if o.MaxPoints < 2 {
		o.MaxPoints = 2
	}

	ic := InitCondFor(mode)
	x, y, z := float64(ic[0]), float64(ic[1]), float64(ic[2])

	// RK4, not the forward Euler the classic render loops use.
	//
	// It is not a stylistic preference: Rabinovich–Fabrikant is stiff with a
	// small basin, and under Euler at its own timestep the trajectory escapes
	// during the transient and this returns nothing at all. The app renders
	// it through the shared RK4 loop for exactly that reason. For the systems
	// that do run Euler in their render loop the attractor is the same set
	// either way — a better integrator does not move it, it just stays on it.
	step := func() { x, y, z = RK4(sys.f, dt, x, y, z) }
	for t := 0.0; t < o.Transient; t += dt {
		step()
		if diverged(x, y, z) {
			return nil
		}
	}

	total := max(int(o.Duration/dt), 2)
	// Round the stride UP, or MaxPoints is not a maximum: flooring lets a
	// trace of 11000 steps capped at 4000 take every 2nd step and return
	// 5500, which is how one attractor came out half again as heavy as the
	// viewer's index pipeline allows.
	every := 1
	if total > o.MaxPoints {
		every = (total + o.MaxPoints - 1) / o.MaxPoints
	}
	out := make([][3]float64, 0, total/every+1)
	for i := range total {
		step()
		if diverged(x, y, z) {
			break
		}
		if i%every == 0 {
			out = append(out, [3]float64{x, y, z})
		}
	}
	return out
}

// diverged reports a trajectory that has left the attractor for good — some
// systems are only conditionally stable, and an exported model of a run to
// infinity is a straight line to nowhere.
func diverged(x, y, z float64) bool {
	const far = 1e6
	if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) {
		return true
	}
	if math.IsInf(x, 0) || math.IsInf(y, 0) || math.IsInf(z, 0) {
		return true
	}
	return math.Abs(x) > far || math.Abs(y) > far || math.Abs(z) > far
}

// trajectory4 traces a 4-D system and returns the (x,y,z) projection — which
// is what the renderer draws and what the fourth coordinate is hidden from.
//
// The hidden state has to be seeded on the attractor rather than at zero: the
// hyper-Rössler DIVERGES from w=0 at the canonical parameters, and the render
// loop only looks healthy there because its divergence guard keeps reseeding.
func trajectory4(s FlowSys4, mode string, o TrajectoryOptions) [][3]float64 {
	dt := s.Dt()
	if dt <= 0 {
		return nil
	}
	if o.Duration <= 0 {
		o = DefaultTrajectory()
	}
	if o.MaxPoints < 2 {
		o.MaxPoints = 2
	}
	ic := InitCondFor(mode)
	st := [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), s.W0}
	step := func() { st = RK4x4(s.F, dt, st) }
	for t := 0.0; t < o.Transient; t += dt {
		step()
		if diverged(st[0], st[1], st[2]) {
			return nil
		}
	}
	total := max(int(o.Duration/dt), 2)
	every := 1
	if total > o.MaxPoints {
		every = (total + o.MaxPoints - 1) / o.MaxPoints
	}
	out := make([][3]float64, 0, total/every+1)
	sc := float64(s.Scale)
	for i := range total {
		step()
		if diverged(st[0], st[1], st[2]) {
			break
		}
		if i%every == 0 {
			out = append(out, [3]float64{st[0] * sc, st[1] * sc, st[2] * sc})
		}
	}
	return out
}

// RK4 advances a 3-D field by one step of classical Runge-Kutta.
func RK4(f Deriv, dt, x, y, z float64) (float64, float64, float64) {
	k1x, k1y, k1z := f(x, y, z)
	k2x, k2y, k2z := f(x+dt/2*k1x, y+dt/2*k1y, z+dt/2*k1z)
	k3x, k3y, k3z := f(x+dt/2*k2x, y+dt/2*k2y, z+dt/2*k2z)
	k4x, k4y, k4z := f(x+dt*k3x, y+dt*k3y, z+dt*k3z)
	return x + dt/6*(k1x+2*k2x+2*k3x+k4x),
		y + dt/6*(k1y+2*k2y+2*k3y+k4y),
		z + dt/6*(k1z+2*k2z+2*k3z+k4z)
}

// RK4x4 is the same step for a 4-D field.
func RK4x4(f Deriv4, dt float64, s [4]float64) [4]float64 {
	add := func(s [4]float64, k [4]float64, h float64) [4]float64 {
		return [4]float64{s[0] + h*k[0], s[1] + h*k[1], s[2] + h*k[2], s[3] + h*k[3]}
	}
	eval := func(s [4]float64) [4]float64 {
		a, b, c, d := f(s[0], s[1], s[2], s[3])
		return [4]float64{a, b, c, d}
	}
	k1 := eval(s)
	k2 := eval(add(s, k1, dt/2))
	k3 := eval(add(s, k2, dt/2))
	k4 := eval(add(s, k3, dt))
	var out [4]float64
	for i := range 4 {
		out[i] = s[i] + dt/6*(k1[i]+2*k2[i]+2*k3[i]+k4[i])
	}
	return out
}
