package attractor

import (
	"math"
	"testing"
)

// The iterate flavor's whole claim is that a typed map is the SAME kind of
// object as a built-in one. Henon is the cross-check that can prove it: it is
// registered in mapdata.go as compiled Go, and it is two lines of arithmetic a
// user can type. If the two disagree, one of them is wrong.

// typedMap compiles expression strings the way the panel does — parse, refuse
// what a map cannot express, bind the knob pointers — and returns the step.
func typedMap(t *testing.T, eq [3]string, params map[string]*float32) mapStep {
	t.Helper()
	var exprs [3]*Expr
	var ptrs [3][]*float32
	for i, s := range eq {
		e, err := ParseExpr(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		if why := iterateBlocker(e); why != "" {
			t.Fatalf("%q refused: %s", s, why)
		}
		exprs[i] = e
		ptrs[i] = make([]*float32, len(e.Params))
		for k, p := range e.Params {
			ptr, ok := params[p]
			if !ok {
				t.Fatalf("%q uses parameter %q, which the test did not supply", s, p)
			}
			ptrs[i][k] = ptr
		}
	}
	return newIterateStep(exprs, ptrs)
}

// typedHenon is the built-in map as a user would type it. The parameters are
// the same float32 vars the built-in reads, so a knob edit would move both and
// the comparison is of the EQUATIONS, not of two different coefficient sets.
func typedHenon(t *testing.T) mapStep {
	t.Helper()
	return typedMap(t, [3]string{"1 - a*x*x + y", "b*x", ""},
		map[string]*float32{"a": &henonA, "b": &henonB})
}

// Iterate for iterate, as far as a chaotic map allows. Exact agreement is not
// asserted: the two evaluate the same arithmetic in the same association order
// and start out identical to the last bit, but Go may contract a*x + y into an
// FMA on some architectures and not others, and Henon amplifies a 1e-16
// disagreement by e^0.42 per iterate — O(1) within ninety of them. So this
// checks the window where any real transcription error is already enormous
// (a wrong sign or a wrong operand is O(1) within two or three iterates) and
// leaves the long run to the attractor-level checks below.
func TestTypedHenonMatchesBuiltinIterates(t *testing.T) {
	typed := typedHenon(t)
	ref, ic, ok := MapStep("henon")
	if !ok {
		t.Fatal("henon is not registered")
	}
	a, b := ic, ic
	for i := 0; i < 25; i++ {
		a[0], a[1], a[2] = ref(a[0], a[1], a[2])
		b[0], b[1], b[2] = typed(b[0], b[1], b[2])
		for k := range a {
			if math.Abs(a[k]-b[k]) > 1e-9 {
				t.Fatalf("iterate %d coordinate %d: built-in %.17g, typed %.17g", i+1, k, a[k], b[k])
			}
		}
	}
}

// The same attractor, reached from a different starting point: a typed map
// must land on the set the built-in draws, not merely track it for a while.
func TestTypedHenonDrawsTheSameAttractor(t *testing.T) {
	typed := typedHenon(t)
	ref, ic, _ := MapStep("henon")

	extent := func(step mapStep, from [3]float64) [4]float64 {
		p := from
		for i := 0; i < 20000; i++ { // transient
			p[0], p[1], p[2] = step(p[0], p[1], p[2])
		}
		box := [4]float64{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
		for i := 0; i < 200000; i++ {
			p[0], p[1], p[2] = step(p[0], p[1], p[2])
			box[0], box[1] = math.Min(box[0], p[0]), math.Max(box[1], p[0])
			box[2], box[3] = math.Min(box[2], p[1]), math.Max(box[3], p[1])
		}
		return box
	}
	// The typed one starts where a typed map seeds, which is not Henon's own
	// initial condition — a different point in the same basin.
	want := extent(ref, ic)
	got := extent(typed, customMapIC)
	for k := range want {
		if math.Abs(want[k]-got[k]) > 1e-3 {
			t.Errorf("attractor extent %d: built-in %.6f, typed %.6f", k, want[k], got[k])
		}
	}
}

// Simultaneous assignment. x' = y, y' = x is a swap; if the new x were fed
// into y the pair would collapse onto x instead, which is a different map and
// the mistake this is here to catch.
func TestIterateAssignsFromThePreviousState(t *testing.T) {
	step := typedMap(t, [3]string{"y", "x", ""}, nil)
	x, y, z := step(2, 3, 0)
	if x != 3 || y != 2 || z != 0 {
		t.Fatalf("(2,3) → (%v,%v,%v), want (3,2,0) — the assignments are not simultaneous", x, y, z)
	}
}

// The knobs have to move the running map, exactly as they move a running flow.
func TestIterateReadsParametersLive(t *testing.T) {
	a := float32(2)
	step := typedMap(t, [3]string{"a*x", "", ""}, map[string]*float32{"a": &a})
	if x, _, _ := step(1, 0, 0); x != 2 {
		t.Fatalf("a=2: x' = %v, want 2", x)
	}
	a = 5
	if x, _, _ := step(1, 0, 0); x != 5 {
		t.Fatalf("after the knob moved to 5: x' = %v, want 5 (the step cached the old value)", x)
	}
}

// What a map cannot express has to be refused rather than silently evaluated
// as zero, which would draw a system nobody typed.
func TestIterateRefusesTimeAndHiddenState(t *testing.T) {
	for _, c := range []struct{ eq, want string }{
		{"1 - a*x*x + y", ""},
		{"sin(x) + cos(z)", ""},
		{"x + t", "t"},
		{"x*y - w", "w"},
	} {
		e, err := ParseExpr(c.eq)
		if err != nil {
			t.Fatalf("parse %q: %v", c.eq, err)
		}
		why := iterateBlocker(e)
		if (why == "") != (c.want == "") {
			t.Errorf("%q: blocker %q, wanted %q", c.eq, why, c.want)
		}
		if c.want != "" && why != "" && why[:1] != c.want {
			t.Errorf("%q: blocked for the wrong reason: %s", c.eq, why)
		}
	}
}

// A typed map is a MAP to everything that asks — which is what keeps the
// consumers that integrate a flow (Model Out FLOW, the ring beam, the Poincare
// section) away from a system that has no dt to integrate with, and what gets
// the Lyapunov readout onto its per-iterate branch. Registering an iterate
// system as a flow would have all of them stepping x += dt·f, which at
// dt = 0.005 is a slow crawl to a fixed point rather than Henon's fractal.
func TestTypedIterateIsAMapAndNotAFlow(t *testing.T) {
	setCustomMap(typedHenon(t))
	t.Cleanup(clearCustomMap)

	if !IsMap(customModeKey) {
		t.Fatal("a registered iterate system does not report as a map")
	}
	if HasFlow(customModeKey) {
		t.Error("an iterate system is registered as a flow — everything downstream would integrate it with a dt it does not have")
	}
	if _, ok := flowFor4(customModeKey); ok {
		t.Error("flowFor4 hands out an iterate system; Model Out FLOW and the Poincare section would run a system that does not exist")
	}
	for _, k := range MapKeys() {
		if k == customModeKey {
			t.Error("the typed map is in MapKeys — it has no catalog entry and nothing could select it there")
		}
	}

	// The measurement itself: per iterate, and Henon's own exponent.
	got := LyapunovFor(customModeKey)
	if !got.PerStep {
		t.Error("a typed map's exponent is reported per unit time; a map has no time")
	}
	if got.Verdict != "chaotic" {
		t.Errorf("verdict %q, want chaotic", got.Verdict)
	}
	want := LyapunovForMap("henon")
	if math.Abs(got.Lambda-want.Lambda) > 0.02 {
		t.Errorf("λ = %.4f/iterate, built-in henon = %.4f", got.Lambda, want.Lambda)
	}

	clearCustomMap()
	_, _, stillThere := MapStep(customModeKey)
	if IsMap(customModeKey) || stillThere {
		t.Error("withdrawing the typed map left it behind; the flow flavor would still look like a map")
	}
}

// Seeding runs a transient so the drawn cloud is the attractor rather than the
// path to it, and gives up rather than seeding an escaped orbit — a typed map
// can be made to diverge with one keystroke.
func TestTypedMapSeedingDiscardsTransientAndSurvivesEscape(t *testing.T) {
	t.Cleanup(func() { mapState, mapSeeded, mapOrbitsN = nil, "", 0 })
	ic := customMapIC

	seedMapState(customModeKey, mapSys{step: typedHenon(t), ic: ic, orbits: 1})
	got := mapState[0]
	if got == ic {
		t.Error("the transient was not run: the orbit is still at the initial condition")
	}
	// Henon's attractor is inside |x| < 1.3, |y| < 0.4; the seed at (0.1, 0.5)
	// is not, so being inside it means the transient actually landed.
	if math.Abs(got[0]) > 1.3 || math.Abs(got[1]) > 0.4 {
		t.Errorf("after the transient the orbit is at %v, which is not on the attractor", got)
	}

	// x' = 2x, y' = 2y — one keystroke from a bounded map to an escaping one.
	// The seeder must hand back a finite state rather than the infinities that
	// would upload as NaNs and blank the whole figure.
	seedMapState(customModeKey, mapSys{step: typedMap(t, [3]string{"2*x", "2*y", ""}, nil), ic: ic, orbits: 1})
	if p := mapState[0]; !finite3(p[0], p[1], p[2]) {
		t.Fatalf("an escaping typed map seeded to %v", p)
	}
	if mapState[0] != ic {
		t.Errorf("an escaping typed map seeded to %v, want a reset to the initial condition %v", mapState[0], ic)
	}
}
