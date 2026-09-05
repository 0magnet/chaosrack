package attractor

import (
	"math"
	"testing"
)

// The crossing arithmetic, checked against curves whose crossings are known in
// closed form. This is the part of the section feature that can be wrong
// without looking wrong: a section drawn from snapped samples is still a
// plausible-looking scatter, it is just a scatter of the wrong points, and
// nobody reading it can tell. So the accuracy claim in poincare.go is a
// measurement here rather than an assertion there.

// vec3len is the length of a difference, for the error measurements below.
func vec3dist(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// A straight segment crosses a plane at a fraction that can be written down,
// and both interpolators must return exactly it: a straight line has no
// curvature for the cubic to correct, so linear and Hermite have to agree with
// each other and with the algebra.
func TestAStraightSegmentCrossesWhereTheAlgebraSaysItDoes(t *testing.T) {
	pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
	// From z=-1 to z=+3: the plane is a quarter of the way along.
	a := [3]float64{0, 0, -1}
	b := [3]float64{8, 4, 3}
	want := [3]float64{2, 1, 0}

	hit, ok := poincareCross(pl, a, b, [3]float64{}, [3]float64{}, crossRising)
	if !ok {
		t.Fatal("no crossing reported for a segment that plainly crosses")
	}
	if d := vec3dist(hit, want); d > 1e-12 {
		t.Errorf("linear crossing at %v, want %v (off by %g)", hit, want, d)
	}

	// The same segment traversed at constant velocity: the Hermite path has to
	// reproduce the straight line, not bend it.
	v := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	hit, ok = poincareCross(pl, a, b, v, v, crossRising)
	if !ok {
		t.Fatal("no crossing reported with velocities supplied")
	}
	if d := vec3dist(hit, want); d > 1e-12 {
		t.Errorf("Hermite crossing at %v, want %v (off by %g) — a constant-velocity "+
			"segment is a straight line and the cubic must not curve it", hit, want, d)
	}
}

// THE HEADLINE MEASUREMENT. A circle has an exactly known crossing, so the
// three ways of answering "where did it cross" can be put side by side.
//
// The circle is sampled at 64 steps per turn — comparable to what the app's
// flows do per orbit — and the step is placed so the crossing falls at a
// generic phase inside it rather than conveniently near an end.
func TestInterpolatingBeatsSnapping(t *testing.T) {
	const (
		radius = 10.0
		steps  = 64
	)
	h := 2 * math.Pi / steps
	at := func(th float64) [3]float64 {
		return [3]float64{radius * math.Cos(th), 0, radius * math.Sin(th)}
	}
	// d/dθ of the above, times the step: the "velocity per step" the Hermite
	// form wants.
	vel := func(th float64) [3]float64 {
		return [3]float64{-radius * math.Sin(th) * h, 0, radius * math.Cos(th) * h}
	}

	pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
	th0 := -0.3 * h // crossing at θ=0, three tenths of a step in
	a, b := at(th0), at(th0+h)
	want := [3]float64{radius, 0, 0}

	snapErr := vec3dist(poincareSnap(pl, a, b), want)

	linHit, ok := poincareCross(pl, a, b, [3]float64{}, [3]float64{}, crossRising)
	if !ok {
		t.Fatal("linear path found no crossing")
	}
	linErr := vec3dist(linHit, want)

	herHit, ok := poincareCross(pl, a, b, vel(th0), vel(th0+h), crossRising)
	if !ok {
		t.Fatal("Hermite path found no crossing")
	}
	herErr := vec3dist(herHit, want)

	t.Logf("crossing error at %d steps/turn: snap %.3e  linear %.3e  hermite %.3e",
		steps, snapErr, linErr, herErr)

	if linErr >= snapErr {
		t.Errorf("linear interpolation (%.3e) is no better than snapping (%.3e); "+
			"interpolating is the whole point of this file", linErr, snapErr)
	}
	if herErr >= linErr {
		t.Errorf("the cubic (%.3e) is no better than the straight line (%.3e); "+
			"the two extra field evaluations are buying nothing", herErr, linErr)
	}
	// Order-of-magnitude floors, not the measured values: the point is that
	// each stage is a different KIND of accurate, and pinning the digits would
	// make this test fail on an unrelated refactor of the circle above.
	if snapErr/linErr < 10 {
		t.Errorf("snapping is only %.1f× worse than interpolating; the smear this "+
			"feature exists to avoid should be far larger than that", snapErr/linErr)
	}
	if linErr/herErr < 100 {
		t.Errorf("the cubic is only %.1f× better than the straight line", linErr/herErr)
	}
}

// The error orders themselves: halving the step should shrink the linear error
// by ~4 (second order) and the Hermite error by ~16 (fourth order). This is
// what says the two are doing what they are claimed to be doing, rather than
// both being right by luck at one step size.
func TestTheInterpolatorsConvergeAtTheirStatedOrders(t *testing.T) {
	const radius = 10.0
	measure := func(steps float64) (lin, her float64) {
		h := 2 * math.Pi / steps
		at := func(th float64) [3]float64 {
			return [3]float64{radius * math.Cos(th), 0, radius * math.Sin(th)}
		}
		vel := func(th float64) [3]float64 {
			return [3]float64{-radius * math.Sin(th) * h, 0, radius * math.Cos(th) * h}
		}
		pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
		th0 := -0.3 * h
		a, b := at(th0), at(th0+h)
		want := [3]float64{radius, 0, 0}
		lh, _ := poincareCross(pl, a, b, [3]float64{}, [3]float64{}, crossRising)
		hh, _ := poincareCross(pl, a, b, vel(th0), vel(th0+h), crossRising)
		return vec3dist(lh, want), vec3dist(hh, want)
	}
	lin1, her1 := measure(64)
	lin2, her2 := measure(128)
	t.Logf("64→128 steps/turn: linear %.3e→%.3e (%.1f×)  hermite %.3e→%.3e (%.1f×)",
		lin1, lin2, lin1/lin2, her1, her2, her1/her2)
	if r := lin1 / lin2; r < 3 || r > 5 {
		t.Errorf("linear error shrank %.1f× on halving the step; second order wants ~4×", r)
	}
	if r := her1 / her2; r < 8 || r > 40 {
		t.Errorf("Hermite error shrank %.1f× on halving the step; fourth order wants ~16×", r)
	}
}

// Direction is not a filter applied afterwards — it decides whether there is a
// crossing at all, and the return trip through the same plane must be silent
// in one-way mode. See poincare.go on why one-way is the default: keeping both
// superimposes two different sections and stops the return map being a
// function.
func TestOneWayIgnoresTheReturnTrip(t *testing.T) {
	pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
	up := [2][3]float64{{0, 0, -1}, {0, 0, 1}}
	down := [2][3]float64{{0, 0, 1}, {0, 0, -1}}
	zero := [3]float64{}

	if _, ok := poincareCross(pl, up[0], up[1], zero, zero, crossRising); !ok {
		t.Error("rising mode missed a rising crossing")
	}
	if _, ok := poincareCross(pl, down[0], down[1], zero, zero, crossRising); ok {
		t.Error("rising mode reported the downward return trip; the section would be two " +
			"superimposed sheets and the return map would alternate between two rules")
	}
	if _, ok := poincareCross(pl, down[0], down[1], zero, zero, crossFalling); !ok {
		t.Error("falling mode missed a falling crossing")
	}
	if _, ok := poincareCross(pl, up[0], up[1], zero, zero, crossFalling); ok {
		t.Error("falling mode reported a rising crossing")
	}
	for _, seg := range [][2][3]float64{up, down} {
		if _, ok := poincareCross(pl, seg[0], seg[1], zero, zero, crossEither); !ok {
			t.Error("both-ways mode missed a crossing")
		}
	}
}

// A step that stays on one side is not a crossing, however close it comes. The
// near miss is the case worth stating: a trajectory that runs along just under
// the plane must produce nothing, or the section fills with points that are
// not crossings at all.
func TestNoCrossingWithoutASignChange(t *testing.T) {
	pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
	zero := [3]float64{}
	for _, seg := range [][2][3]float64{
		{{0, 0, -1}, {0, 0, -1e-9}},
		{{0, 0, 1e-9}, {0, 0, 1}},
		{{5, 5, -3}, {-5, -5, -3}},
	} {
		if _, ok := poincareCross(pl, seg[0], seg[1], zero, zero, crossEither); ok {
			t.Errorf("%v → %v reported as a crossing; both ends are on the same side", seg[0], seg[1])
		}
	}
}

// A sample landing exactly ON the plane is counted once, not twice. With two
// closed comparisons the step that arrives and the step that leaves both claim
// it, and a doubled point in the section is a doubled point on the return
// map's diagonal — a spurious fixed point, which is precisely the thing a
// reader of a return map is looking for.
func TestASampleOnThePlaneIsCountedOnce(t *testing.T) {
	pl := newPoincarePlane([3]float64{0, 0, 1}, 0)
	zero := [3]float64{}
	arrive := [2][3]float64{{0, 0, -1}, {0, 0, 0}} // g0 < 0, g1 == 0
	depart := [2][3]float64{{0, 0, 0}, {0, 0, 1}}  // g0 == 0, g1 > 0

	_, a := poincareCross(pl, arrive[0], arrive[1], zero, zero, crossRising)
	_, d := poincareCross(pl, depart[0], depart[1], zero, zero, crossRising)
	if a == d {
		t.Errorf("the arriving step and the departing step both report %v for a sample "+
			"sitting exactly on the plane; exactly one of them must", a)
	}
	if !a {
		t.Error("the arriving step is the half-open interval's owner and should be the one that counts it")
	}
}

// The in-plane basis has to be an orthonormal right-handed frame for EVERY
// normal, including the axis-aligned ones the panel offers — the fixed-seed
// version of poincareBasis degenerates exactly there, which is to say on all
// three of its presets.
func TestTheInPlaneBasisIsAnOrthonormalFrame(t *testing.T) {
	normals := [][3]float64{
		{1, 0, 0}, {0, 1, 0}, {0, 0, 1},
		{-1, 0, 0}, {0, -1, 0}, {0, 0, -1},
		{1, 1, 1}, {0.3, -0.9, 0.05}, {1e-9, 1, 1e-9},
	}
	for _, n := range normals {
		pl := newPoincarePlane(n, 0)
		u, v, nn := pl.u, pl.v, pl.n
		dot := func(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
		for name, got := range map[string]float64{
			"u·u": dot(u, u), "v·v": dot(v, v), "n·n": dot(nn, nn),
		} {
			if math.Abs(got-1) > 1e-9 {
				t.Errorf("normal %v: %s = %g, want 1 — the section would be scaled by this", n, name, got)
			}
		}
		for name, got := range map[string]float64{
			"u·v": dot(u, v), "u·n": dot(u, nn), "v·n": dot(v, nn),
		} {
			if math.Abs(got) > 1e-9 {
				t.Errorf("normal %v: %s = %g, want 0 — the section would be sheared by this", n, name, got)
			}
		}
		// u × v == n: a left-handed frame mirrors the section, which is not
		// wrong so much as silently the wrong picture.
		cx := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		if vec3dist(cx, nn) > 1e-9 {
			t.Errorf("normal %v: u×v = %v, want the normal %v — the section is mirrored", n, cx, nn)
		}
	}
}

// The three presets get the bases a hand would draw: a section through the z
// plane is read in (x, y), through x in (y, z), through y in (z, x). Anything
// else is defensible and none of it is what the axis labels on the panel say.
func TestAxisAlignedPlanesReadInTheObviousCoordinates(t *testing.T) {
	cases := []struct {
		n, u, v [3]float64
	}{
		{[3]float64{0, 0, 1}, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}},
		{[3]float64{1, 0, 0}, [3]float64{0, 1, 0}, [3]float64{0, 0, 1}},
		{[3]float64{0, 1, 0}, [3]float64{0, 0, 1}, [3]float64{1, 0, 0}},
	}
	for _, c := range cases {
		pl := newPoincarePlane(c.n, 0)
		if vec3dist(pl.u, c.u) > 1e-12 || vec3dist(pl.v, c.v) > 1e-12 {
			t.Errorf("normal %v gives basis (%v, %v), want (%v, %v)", c.n, pl.u, pl.v, c.u, c.v)
		}
	}
	// And the projection then IS the pair of coordinates the labels promise.
	pl := newPoincarePlane([3]float64{0, 0, 1}, 7)
	s, u := pl.project([3]float64{3, -4, 7})
	if math.Abs(s-3) > 1e-12 || math.Abs(u+4) > 1e-12 {
		t.Errorf("z-plane projection of (3,-4,7) is (%g,%g), want (3,-4)", s, u)
	}
}

// A plane offset from the origin crosses where the offset says, not where the
// origin is. Trivial, and it is the arithmetic the offset knob rides on.
func TestTheOffsetMovesThePlane(t *testing.T) {
	pl := newPoincarePlane([3]float64{0, 0, 1}, 25)
	zero := [3]float64{}
	hit, ok := poincareCross(pl, [3]float64{0, 0, 20}, [3]float64{10, 0, 30}, zero, zero, crossRising)
	if !ok {
		t.Fatal("no crossing of the offset plane")
	}
	want := [3]float64{5, 0, 25}
	if d := vec3dist(hit, want); d > 1e-12 {
		t.Errorf("crossing at %v, want %v", hit, want)
	}
}

// ── The accumulator ──────────────────────────────────────────────────────

// The ring reads oldest-first even after it has wrapped. Slot order is the
// tempting alternative and it joins the newest hit to the oldest once per lap,
// which is one wrong pair per ring-full in the return map: rare enough to look
// like a real stray point rather than like a bug.
func TestTheLogReadsOldestFirstAcrossAWrap(t *testing.T) {
	var l poincareLog
	l.reset(4)
	for i := 0; i < 7; i++ {
		l.add(poincareHit{S: float32(i)})
	}
	if l.len() != 4 {
		t.Fatalf("ring of 4 holds %d hits", l.len())
	}
	want := []float32{3, 4, 5, 6}
	for i, w := range want {
		if got := l.at(i).S; got != w {
			t.Errorf("at(%d) = %v, want %v — the ring is being read in slot order", i, got, w)
		}
	}
}

// A reseed marks the next hit, so the return map can skip the pair that spans
// it. Those two crossings are on different trajectories; joined, they plot a
// point wherever the two happen to fall, which on an otherwise clean parabola
// is a single dot in open space that reads as structure.
func TestAReseedBreaksTheReturnMapChain(t *testing.T) {
	var l poincareLog
	l.reset(8)
	l.add(poincareHit{S: 1})
	l.add(poincareHit{S: 2})
	l.breakChain()
	l.add(poincareHit{S: 3})
	l.add(poincareHit{S: 4})

	if !l.at(0).Gap {
		t.Error("the very first hit has no predecessor and must be flagged")
	}
	if l.at(1).Gap {
		t.Error("a hit that does follow its predecessor is flagged as a break")
	}
	if !l.at(2).Gap {
		t.Error("the hit after a reseed is not flagged; the return map would join two " +
			"unrelated trajectories into one point")
	}
	if l.at(3).Gap {
		t.Error("the break leaked past the hit it belonged to")
	}
}
