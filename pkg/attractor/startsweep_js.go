//go:build js && wasm

package attractor

// The Grid's start sweep: one trajectory of the current flow per cell, each
// started a distance ε from the model's own, so the grid shows sensitive
// dependence on initial conditions — the defining property of chaos — live.
// With the Grid's Overlay on, the cells are drawn one over another, and a
// grid of two is what the Twin switch was: the model and a copy started ε
// away, coming apart.
//
// A flow integrates, so a sweep of one of its PARAMETERS is not offered
// (sweepNumericOK): one integrator would be smeared across the cells. The
// start is the sweep a flow can have, because each cell here integrates its
// own state. All of them use the SAME generic stepper (dynamics.FlowFor4), so
// their separation reflects the dynamics, never an integrator mismatch.
//
// The λ measurement lives in pkg/analysis and lyaplive_js.go, on a probe pair
// of its own, and its readout is the model bank's λ cell. tick is only where
// it is advanced: generateForMode reaches tick every frame for every mode
// that has a trajectory at all, BEFORE any of the switch-conditional
// branches, so it is the one per-frame hook a measurement can hang off — and
// the modes it does not reach (the spectrogram surfaces, the recurrence plot,
// the audio scopes) are exactly the modes with no exponent to measure.

import (
	"math"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// startSweep is the start sweep's trajectories, one per cell.
type startSweep struct {
	seeded string       // mode the trajectories were seeded for
	eps    []float64    // the ε each was seeded with, by cell
	states [][4]float64 // each cell's trajectory
	buf    []float32    // vertex scratch for every cell but the first (vertBuf holds it)
}

var starts startSweep

// The span of ε a start sweep covers, as powers of ten: FROM and TO at 0 and
// 1 of it, except that 0 itself is ε = 0, the model's own trajectory.
const (
	startEpsLo = -9
	startEpsHi = -1
)

func (s *startSweep) invalidate() { s.seeded = "" }

// startSweepable reports whether mode can have a start sweep: whether it is
// a flow the generic stepper integrates. The bifurcation explorer and the
// Poincaré section are excluded by name, as generateForMode excludes them.
func startSweepable(mode string) bool {
	if mode == "bifurcation" || mode == "poincare" {
		return false
	}
	_, ok := dynamics.FlowFor4(mode)
	return ok
}

// startSweepOn reports whether this frame draws mode's cells as a start
// sweep.
func startSweepOn(mode string) bool {
	return viewSplit() && (grid.sweepTarget() == "#start" || grid.sweepTarget2() == "#start") && startSweepable(mode)
}

// startEps is the ε cell i of n starts at: its place on the axis the start
// is swept along, between FROM and TO.
func startEps(i, n int) float64 {
	across, down := sweepAxisFracs(i, n)
	frac := across
	if grid.sweepTarget() != "#start" {
		frac = down
	}
	t := grid.sweepLo + (grid.sweepHi-grid.sweepLo)*frac
	if t <= 0 {
		return 0
	}
	return math.Pow(10, startEpsLo+(startEpsHi-startEpsLo)*float64(t))
}

// flowStep advances one state by a single Euler sub-step.
func flowStep(sys dynamics.FlowSys4, s *[4]float64, dt float64) {
	dx, dy, dz, dw := sys.F(s[0], s[1], s[2], s[3])
	s[0] += dt * dx
	s[1] += dt * dy
	s[2] += dt * dz
	s[3] += dt * dw
}

func flowDiverged(s [4]float64) bool {
	const lim = 1e4
	return !(s[0] > -lim && s[0] < lim && s[1] > -lim && s[1] < lim &&
		s[2] > -lim && s[2] < lim && s[3] > -lim && s[3] < lim)
}

// seed starts every cell's trajectory together from the model's initial
// condition, moved ε along x: started together, so what the cells show
// apart is the dynamics and not when each began.
func (s *startSweep) seed(mode string, sys dynamics.FlowSys4, eps []float64) {
	ic := dynamics.InitCondFor(mode)
	s.eps = append(s.eps[:0], eps...)
	s.states = s.states[:0]
	for _, e := range eps {
		s.states = append(s.states, [4]float64{float64(ic[0]) + e, float64(ic[1]), float64(ic[2]), sys.W()})
	}
	s.seeded = mode
}

// tick draws the trajectory of the cell being drawn (viewPass). Returns
// false when the normal generator should run instead: no start sweep, or not
// a flow.
func (s *startSweep) tick(mode string) bool {
	// The λ probe advances once a frame, however many cells are drawn.
	if viewPass <= 0 {
		lyapLive.tick(mode)
	}
	if !startSweepOn(mode) {
		return false
	}
	sys, _ := dynamics.FlowFor4(mode)
	n := grid.n()
	i := max(viewPass, 0)
	if i == 0 {
		eps := make([]float64, n)
		for c := range n {
			eps[c] = startEps(c, n)
		}
		if s.seeded != mode || !sameEps(eps, s.eps) {
			s.seed(mode, sys, eps)
		}
	}
	if i >= len(s.states) {
		return false
	}
	budget := frameBudgetCompiled
	if sys.Interpreted {
		budget = frameBudgetInterpreted
	}
	// Every cell's trajectory and the λ probe pair share the frame budget.
	sub := effSubSteps(sim.speedSteps, sim.steps, budget/max(n, 2))
	dt := sys.Dt() * float64(sim.speedScale)
	scale := sys.Scale
	invN := float32(1) / float32(sim.steps-1)

	out := sim.vertBuf[:sim.steps*4]
	if i > 0 {
		if len(s.buf) < sim.steps*4 {
			s.buf = make([]float32, cap(sim.vertBuf))
		}
		out = s.buf[:sim.steps*4]
	}
	st := &s.states[i]
	for k := range sim.steps {
		for range sub {
			flowStep(sys, st, dt)
			if flowDiverged(*st) {
				ic := dynamics.InitCondFor(mode)
				*st = [4]float64{float64(ic[0]), float64(ic[1]), float64(ic[2]), sys.W()}
			}
		}
		j := k * 4
		out[j] = float32(st[0]) * scale
		out[j+1] = float32(st[1]) * scale
		out[j+2] = float32(st[2]) * scale
		out[j+3] = float32(k) * invN
	}
	if i == 0 {
		// The app-wide integrator state follows the first cell, so the
		// permalink, Model Out SCAN and the end of the sweep continue
		// seamlessly.
		sim.x, sim.y, sim.z = float32(st[0]), float32(st[1]), float32(st[2])
		sim.x64, sim.y64, sim.z64 = st[0], st[1], st[2]
		sys.SetW(st[3])
	}
	gpu.uploadVerticesOnly(out, gpu.drawMode, sim.steps)
	return true
}

// sameEps reports whether two cells' ε lists are the same.
func sameEps(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
