package dynamics

import (
	"math"
	"testing"
)

// These tests exist because of what they replaced.
//
// Checking that a control did what it says used to mean a browser: open a tab
// over CDP, set the knob, screenshot the canvas and compare pixels. That
// answered slowly, needed a GPU and a window manager, and — measured — was not
// even reliable, because the audio displays draw whatever the microphone is
// doing and two screenshots of live music differ whether or not the knob did
// anything. The systems integrate perfectly well on a host. The only reason
// this could not be asked here was that the table of ranges sat in a file
// tagged js && wasm.

// traj is a short integration, enough to tell two attractors apart without
// making the suite slow.
func traj(t *testing.T, mode string) [][3]float64 {
	t.Helper()
	return Trajectory(mode, TrajectoryOptions{Transient: 10, Duration: 60, MaxPoints: 1200})
}

func extent(p [][3]float64) (dx, dy, dz float64) {
	if len(p) == 0 {
		return 0, 0, 0
	}
	lo, hi := p[0], p[0]
	for _, q := range p {
		for i := range 3 {
			lo[i] = math.Min(lo[i], q[i])
			hi[i] = math.Max(hi[i], q[i])
		}
	}
	return hi[0] - lo[0], hi[1] - lo[1], hi[2] - lo[2]
}

// differs reports whether two paths are meaningfully different rather than the
// same path recomputed.
func differs(a, b [][3]float64) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		for k := range 3 {
			if math.Abs(a[i][k]-b[i][k]) > 1e-9 {
				return true
			}
		}
	}
	return false
}

// A KNOB THAT DOES NOTHING IS THE BUG THIS CATCHES. Every row in the registry
// claims to be a constant of its system; a row wired to the wrong variable, or
// to a variable the deriv stopped reading, still builds a knob, still moves,
// still shows a number, and changes nothing on screen.
//
// ONE STEP FROM THE DEFAULT, not the ends of the range. Turning each knob to
// both extremes was the obvious test and it was wrong twice over: several of
// these systems only exist in a narrow band — Rabinovich–Fabrikant escapes
// everywhere except near alpha = 1.1 and returns NO POINTS AT ALL — so the
// comparison was between two empty paths, which are equal, and the test
// reported a dead knob for a knob that is wired. A single step is also what a
// knob actually does when it is clicked, so a step that changes nothing is
// exactly the defect worth naming.
func TestEveryParameterMovesItsSystem(t *testing.T) {
	t.Cleanup(ResetParams)
	for _, mode := range ParamModes() {
		if !HasFlow(mode) {
			continue // a table for something Trajectory cannot integrate
		}
		for _, p := range Params(mode) {
			ResetParams()
			base := traj(t, mode)
			if len(base) == 0 {
				t.Errorf("%s draws nothing at its own defaults", mode)
				break
			}
			// One click up, or down when the default is already at the top.
			to := p.Def + p.Step
			if to > p.Max {
				to = p.Def - p.Step
			}
			if to < p.Min || to > p.Max {
				continue // a range narrower than its own step: nothing to test
			}
			ResetParams()
			if err := SetParam(p.ID, to); err != nil {
				t.Errorf("%s: %v", p.ID, err)
				continue
			}
			if moved := traj(t, mode); !differs(base, moved) {
				t.Errorf("%s (%s): one step from %g to %g leaves the trajectory identical — the knob is not wired to the system",
					p.ID, mode, p.Def, to)
			}
		}
	}
	ResetParams()
}

// The defaults in the table have to be the values the systems actually start
// at, or the panel opens showing numbers the model is not running.
func TestEveryParameterDefaultIsTheValueInTheSystem(t *testing.T) {
	ResetParams()
	for _, mode := range ParamModes() {
		for _, p := range Params(mode) {
			if *p.Value != p.Def {
				t.Errorf("%s: the registry says the default is %g, the variable holds %g",
					p.ID, p.Def, *p.Value)
			}
			if p.Def < p.Min || p.Def > p.Max {
				t.Errorf("%s: default %g is outside its own range %g..%g", p.ID, p.Def, p.Min, p.Max)
			}
		}
	}
}

// Lorenz is the one system here whose behavior is published, so it is the one
// that can be checked against something other than this program's own opinion.
// Below rho = 24.74 the origin's companion fixed points are stable and the
// trajectory spirals into one: there is no attractor to draw, and the extent
// collapses. Above it, there is. Anything that broke the wiring between the
// knob and the integrator would have to break it in a way that preserved this.
func TestLorenzLosesItsAttractorBelowTheCriticalRho(t *testing.T) {
	t.Cleanup(ResetParams)
	measure := func(rho float32) float64 {
		ResetParams()
		if err := SetParam("lorenz-r", rho); err != nil {
			t.Fatalf("lorenz-r=%g: %v", rho, err)
		}
		dx, dy, dz := extent(traj(t, "lorenz"))
		return math.Max(dx, math.Max(dy, dz))
	}
	if sub := measure(14); sub > 1 {
		t.Errorf("at rho=14 the extent is %.3f; below the critical rho it should collapse to a point", sub)
	}
	if over := measure(28); over < 10 {
		t.Errorf("at rho=28 the extent is %.3f; the butterfly should be tens of units across", over)
	}
}

// SetParam refuses rather than clamps, because a caller that asks for a value
// outside the range and is silently given a different one gets a passing
// measurement of a system it did not ask for.
func TestSetParamRefusesWhatAKnobCouldNotProduce(t *testing.T) {
	t.Cleanup(ResetParams)
	ResetParams()
	before := LorenzR
	if err := SetParam("lorenz-r", 600); err == nil {
		t.Error("rho=600 was accepted; the knob stops at 60")
	}
	if LorenzR != before {
		t.Errorf("a refused set still wrote: rho is %g, was %g", LorenzR, before)
	}
	if err := SetParam("no-such-knob", 1); err == nil {
		t.Error("an unknown id was accepted")
	}
}

// The registry and the panel address parameters by the same ids, so an id that
// appears twice is two knobs writing to one name — the permalink, the MIDI map
// and Reset All all address by id and would disagree about which one they mean.
func TestParameterIDsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, mode := range ParamModes() {
		for _, p := range Params(mode) {
			if was, dup := seen[p.ID]; dup {
				t.Errorf("%s is declared by both %s and %s", p.ID, was, mode)
			}
			seen[p.ID] = mode
		}
	}
}
