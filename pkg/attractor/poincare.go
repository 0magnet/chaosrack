package attractor

import "math"

// The Poincaré section, as arithmetic. Nothing here draws, nothing here
// touches the DOM, and nothing here integrates: it is handed two consecutive
// states of a trajectory and answers where — and whether — the flow went
// through a plane between them.
//
// That separation is not tidiness. The one thing a section has to get right is
// WHERE the crossing was, and the difference between a right answer and a
// wrong one is a few lines of algebra that a browser cannot be asked about.
// It lives untagged so the host test suite can put a curve with a known
// analytic crossing through it and measure the error, which poincare_test.go
// does — see there for the numbers this file is justified by.
//
// ── Why interpolate at all ───────────────────────────────────────────────
//
// The obvious implementation takes whichever of the two straddling samples is
// nearer the plane and plots that. It is wrong by an amount that is easy to
// underestimate. The integrator lands on the plane at a uniformly-distributed
// phase within one step, so the snapped point misses the true crossing by up
// to half a step of ARC — the in-plane displacement is |v_parallel|·dt/2 on
// average, with |v_parallel| the speed of the flow along the plane. On the
// Lorenz attractor at its default dt that is a smear of the same order as the
// gap between the section's sheets, which is precisely the structure the
// section exists to show: the fractal banding turns into a fuzzy ribbon and
// the return map turns into a cloud with a parabola somewhere inside it.
//
// Linear interpolation between the two samples removes the first-order term
// and leaves the curvature: error O(dt²)·|acceleration|. Cubic Hermite, which
// costs two extra vector-field evaluations PER CROSSING (not per step — a
// crossing happens about once per orbit, so this is nothing), removes two more
// orders. Measured on a circle of radius 10 sampled at 64 steps per turn, the
// three land 2.9e-1 / 1.0e-2 / 1.7e-6 from the analytic crossing: snapping is
// 29× worse than the straight line, and the straight line is 5900× worse than
// the cubic. Halving the step divides those last two by 4 and by 16, which is
// the second and fourth order they are claimed to be. The section is
// drawn from the Hermite root; the linear one is kept because it is the
// fallback when the cubic misbehaves, and because it is what the test measures
// the cubic against.
//
// ── Why one direction ────────────────────────────────────────────────────
//
// A bounded flow that pierces a plane going one way has to come back the other
// way somewhere, so keeping both directions superimposes TWO sections: the
// up-going sheet and the down-going sheet, which are different sets. The union
// is not a Poincaré map. Its consecutive points no longer come from a single
// first-return rule — they alternate between two rules — so the return map
// stops being a function, period-2 reads as period-4, and the period-doubling
// cascade that is the whole reason to look at a return map is buried in a
// two-branch tangle. One-way is therefore the DEFAULT and not merely an
// option; "both" is offered because seeing the two sheets interleave is worth
// one glance, and because a flow that is genuinely transverse in only one
// place is easier to find by looking at both and then choosing.

// Crossing direction, in terms of the sign of the plane's signed distance
// along the trajectory. Rising is − → + and is the default; see above.
const (
	crossRising  = 0
	crossFalling = 1
	crossEither  = 2
)

// poincareDirNames are the panel's names for those values, in index order
// because paramLabels reads a setting's position as its value. They live here
// rather than on the tagged side so a direction cannot be added without a name
// or renamed in only one of the two places; the host build has no consumer for
// them, which is what the suppression below is about.
//
//nolint:unused // read from paramdefs_js.go, and the panel is js-only
var poincareDirNames = []string{"up", "down", "both"}

// poincarePlane is an oriented plane in the SYSTEM'S OWN state space: the set
// of points p with n·p == d, with n a unit normal.
//
// State space, not view space, and that is the load-bearing decision in this
// feature. The app already has a plane — the depth partition in split_js.go,
// which the shader reads as uSplitZ/uSplitSide — and it was the obvious thing
// to reuse. It is the wrong plane, for a reason that is not a matter of taste:
// that one is defined in VIEW space, parallel to the screen, deliberately so,
// because its job is to decide which half of the model is drawn in front of
// the rack panel and which behind, and it must stay parallel to the screen
// however the model is turned. A section plane that followed the camera would
// make the section depend on where the viewer happens to be standing, which is
// not a property of the dynamical system at all: turn the model and the
// scatter would reorganize itself, and every conclusion drawn from it would be
// about the mouse. So the geometry is not shared.
//
// What IS shared, and deliberately, is the PARAMETERIZATION. splitFrac says
// where its plane sits as a FRACTION of the model's reach either side of the
// model's center, so that the knob means the same thing on a Lorenz attractor
// and on a dodecahedron; the section's pos knob says the same thing about d,
// for the same reason. Two planes, one way of saying where a plane is. What it
// does not share is splitFrac's yardstick — see sectPosF in poincare_js.go for
// why modelFitExtent is the wrong number to measure this one against.
//
// u and v are an orthonormal basis OF the plane, so a crossing has 2-D
// coordinates in it. They are derived from n once, at construction, because
// they must be the same basis for every crossing — a basis recomputed
// per-point from anything that moves would rotate the section under itself.
type poincarePlane struct {
	n    [3]float64
	d    float64
	u, v [3]float64
}

// newPoincarePlane normalizes the normal and builds the in-plane basis. A zero
// or degenerate normal falls back to +z, which is the plane the section had
// before it was given an orientation at all: the honest failure here is the
// old behavior, not a plane with no direction.
func newPoincarePlane(n [3]float64, d float64) poincarePlane {
	l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
	if l < 1e-12 || math.IsNaN(l) || math.IsInf(l, 0) {
		n, l = [3]float64{0, 0, 1}, 1
	}
	n = [3]float64{n[0] / l, n[1] / l, n[2] / l}
	u, v := poincareBasis(n)
	return poincarePlane{n: n, d: d, u: u, v: v}
}

// poincareBasis builds an orthonormal (u, v) spanning the plane with normal n,
// with u × v pointing along n so the 2-D picture is right-handed when read
// from the side the trajectory rises through.
//
// The seed axis is the one n is LEAST aligned with. Picking a fixed seed
// (always +x, say) is the standard trap: when n happens to be near ±x the
// cross product is near zero and the basis is numerically garbage — the
// section comes out as a line, or flips, at exactly the axis-aligned
// orientations the panel offers as its three presets, which is to say all of
// the time.
//
// For the three axis-aligned normals this yields the bases a hand would
// choose: n=+z gives (x, y), n=+x gives (y, z), n=+y gives (z, x). That is why
// the seed order below is z, x, y rather than x, y, z.
func poincareBasis(n [3]float64) (u, v [3]float64) {
	seed := [3]float64{0, 0, 1}
	if math.Abs(n[2]) >= math.Abs(n[0]) && math.Abs(n[2]) >= math.Abs(n[1]) {
		seed = [3]float64{1, 0, 0}
	} else if math.Abs(n[0]) >= math.Abs(n[1]) {
		seed = [3]float64{0, 1, 0}
	}
	// u = normalize(seed − (seed·n)n): the seed with its normal component
	// taken out, which is in the plane by construction.
	dot := seed[0]*n[0] + seed[1]*n[1] + seed[2]*n[2]
	u = [3]float64{seed[0] - dot*n[0], seed[1] - dot*n[1], seed[2] - dot*n[2]}
	l := math.Sqrt(u[0]*u[0] + u[1]*u[1] + u[2]*u[2])
	if l < 1e-12 {
		// Unreachable given the seed choice above, and cheap insurance if that
		// choice is ever changed: any unit vector in the plane will do.
		u, l = [3]float64{n[1], -n[0], 0}, math.Hypot(n[1], n[0])
		if l < 1e-12 {
			return [3]float64{1, 0, 0}, [3]float64{0, 1, 0}
		}
	}
	u = [3]float64{u[0] / l, u[1] / l, u[2] / l}
	v = [3]float64{
		n[1]*u[2] - n[2]*u[1],
		n[2]*u[0] - n[0]*u[2],
		n[0]*u[1] - n[1]*u[0],
	}
	return u, v
}

// signed is the signed distance from p to the plane, positive on the side the
// normal points to. This is the scalar the whole file is about: a crossing is
// a sign change in it, and the crossing point is its root.
func (pl poincarePlane) signed(p [3]float64) float64 {
	return pl.n[0]*p[0] + pl.n[1]*p[1] + pl.n[2]*p[2] - pl.d
}

// project gives a point's coordinates in the plane's own 2-D basis. Applied to
// a crossing this is the section itself; applied to anything off the plane it
// is that point's shadow on it, which is not something this feature wants and
// is why only crossings are ever passed in.
func (pl poincarePlane) project(p [3]float64) (s, t float64) {
	return p[0]*pl.u[0] + p[1]*pl.u[1] + p[2]*pl.u[2],
		p[0]*pl.v[0] + p[1]*pl.v[1] + p[2]*pl.v[2]
}

// poincareAccepts reports whether the signed distances at the two ends of a
// step are a crossing in the wanted direction.
//
// The comparison is HALF-OPEN — g0 < 0 <= g1 for a rising crossing — and that
// is deliberate. A sample landing exactly on the plane (g == 0) is common with
// a plane placed at a coordinate the system visits exactly, and with two
// closed comparisons it would be counted twice: once as the end of the step
// that arrived and once as the start of the step that left. A doubled point in
// the section is a doubled point in the return map, i.e. a spurious fixed
// point sitting on the diagonal, which is exactly the feature someone reading
// a return map is looking for.
func poincareAccepts(g0, g1 float64, dir int) bool {
	rising := g0 < 0 && g1 >= 0
	falling := g0 >= 0 && g1 < 0
	switch dir {
	case crossFalling:
		return falling
	case crossEither:
		return rising || falling
	default:
		return rising
	}
}

// poincareFracLinear is the crossing's position within the step, as a fraction
// of it, by straight-line interpolation of the signed distance. It assumes
// poincareAccepts has already said there is a crossing, so g0 and g1 have
// opposite signs and the denominator cannot vanish.
func poincareFracLinear(g0, g1 float64) float64 {
	den := g0 - g1
	if den == 0 {
		return 0
	}
	return poincareClamp01(g0 / den)
}

// poincareFracHermite refines that root using the flow's own slope at each end
// of the step: m0 and m1 are dg/ds, the rate of change of the signed distance
// PER STEP (n·f(p)·dt, not n·f(p) — the parameter is the fraction of the step,
// not time).
//
// g is then a cubic in s and its root is found by Newton from the linear
// guess. Three iterations, because a cubic that already brackets a root
// converges to double precision in two from a starting point this good, and
// the third is free insurance. Anything that leaves the step — a cubic that
// wiggles, which happens when the step is far too long for the curvature —
// falls back to the linear answer rather than reporting a crossing outside the
// interval it was found in.
func poincareFracHermite(g0, g1, m0, m1 float64) float64 {
	s := poincareFracLinear(g0, g1)
	for i := 0; i < 3; i++ {
		g := hermite1(g0, m0, g1, m1, s)
		d := hermite1d(g0, m0, g1, m1, s)
		if d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return poincareFracLinear(g0, g1)
		}
		s -= g / d
		if math.IsNaN(s) || s < 0 || s > 1 {
			return poincareFracLinear(g0, g1)
		}
	}
	return s
}

// poincarePoint places the crossing in space at fraction s of the step, using
// the same cubic the root was found with so the point and the root agree. va
// and vb are the velocities at the two ends scaled BY dt, for the same reason
// the slopes are: the curve is parameterized by the step, not by time.
//
// It does NOT degrade to the straight line when the velocities are zero, which
// is the trap this comment exists for. Cubic Hermite with both slopes set to
// zero is 3s²−2s³ — smoothstep, the ease-in-out curve — so a caller with no
// velocities to give would get a crossing dragged toward whichever endpoint
// was nearer in parameter, which is snapping with extra steps. It was wrong by
// 0.92 world units on a segment whose exact answer is four lines of algebra,
// and the test that says so is the first one in poincare_test.go. Callers
// without velocities take poincareLerp instead; poincareCross picks.
func poincarePoint(a, b, va, vb [3]float64, s float64) [3]float64 {
	var out [3]float64
	for i := 0; i < 3; i++ {
		out[i] = hermite1(a[i], va[i], b[i], vb[i], s)
	}
	return out
}

// poincareLerp is the straight-line point at fraction s of the step.
func poincareLerp(a, b [3]float64, s float64) [3]float64 {
	return [3]float64{
		a[0] + s*(b[0]-a[0]),
		a[1] + s*(b[1]-a[1]),
		a[2] + s*(b[2]-a[2]),
	}
}

// hermite1 is the scalar cubic Hermite basis: value p0 with slope m0 at s=0,
// value p1 with slope m1 at s=1.
func hermite1(p0, m0, p1, m1, s float64) float64 {
	s2 := s * s
	s3 := s2 * s
	return (2*s3-3*s2+1)*p0 + (s3-2*s2+s)*m0 + (-2*s3+3*s2)*p1 + (s3-s2)*m1
}

// hermite1d is its derivative with respect to s — the Newton step's
// denominator, and the reason the root solve costs no extra evaluations of the
// vector field.
func hermite1d(p0, m0, p1, m1, s float64) float64 {
	s2 := s * s
	return (6*s2-6*s)*p0 + (3*s2-4*s+1)*m0 + (-6*s2+6*s)*p1 + (3*s2-2*s)*m1
}

// poincareClamp01 is the float64 clamp. Prefixed because audiofeatures_js.go
// already owns the name clamp01 for the float32 one, and the two cannot be the
// same function: this file works in double precision deliberately, and letting
// a root fraction round-trip through float32 would give away the accuracy the
// root solve exists for.
func poincareClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// poincareCross is the whole thing in one call: given the two ends of an
// integration step and (optionally) the velocity at each, say whether the flow
// crossed the plane in the wanted direction and where.
//
// va/vb are the vector field at a and b MULTIPLIED BY dt. Pass the zero vector
// for both to get the linear crossing — callers that cannot cheaply evaluate
// the field (the equation engine, where a field evaluation is an AST walk) do
// exactly that, and the linear answer is still two orders better than snapping.
func poincareCross(pl poincarePlane, a, b, va, vb [3]float64, dir int) (hit [3]float64, ok bool) {
	g0 := pl.signed(a)
	g1 := pl.signed(b)
	if !poincareAccepts(g0, g1, dir) {
		return hit, false
	}
	m0 := pl.n[0]*va[0] + pl.n[1]*va[1] + pl.n[2]*va[2]
	m1 := pl.n[0]*vb[0] + pl.n[1]*vb[1] + pl.n[2]*vb[2]
	if m0 == 0 && m1 == 0 {
		return poincareLerp(a, b, poincareFracLinear(g0, g1)), true
	}
	return poincarePoint(a, b, va, vb, poincareFracHermite(g0, g1, m0, m1)), true
}

// poincareSnap is the implementation this file exists to be better than:
// whichever endpoint is nearer the plane. It is not called by the app. It is
// here so the test can measure the thing that was rejected instead of
// asserting in a comment that it would have been worse — the claim at the top
// of this file about a 40× difference is checked, not remembered.
func poincareSnap(pl poincarePlane, a, b [3]float64) [3]float64 {
	if math.Abs(pl.signed(a)) <= math.Abs(pl.signed(b)) {
		return a
	}
	return b
}

// ── The accumulated section ──────────────────────────────────────────────

// poincareHit is one crossing: where it was in the system's coordinates, and
// the same point in the plane's 2-D basis. Both are kept because the two views
// need different ones — the in-place overlay draws P where the crossing
// physically is, and the flat section and the return map read S and T.
//
// float32 because these go straight into a vertex buffer. The ARITHMETIC above
// is float64: a section is the difference between nearby trajectory sheets,
// and doing the root solve in the precision the buffer happens to use would
// throw away the accuracy the root solve is for.
type poincareHit struct {
	P    [3]float32
	S, T float32
	// Gap marks a hit whose predecessor in the log is NOT its predecessor in
	// time — the integrator was reseeded between them (a divergence guard
	// fired, or the plane moved). The return map plots consecutive pairs, and a
	// pair spanning a reseed is two unrelated points joined by nothing: it
	// lands wherever the two happen to be, which on a clean parabola is a
	// single stray dot in the middle of the plot that reads as structure.
	Gap bool
}

// poincareLog is the ring the crossings accumulate in — newest replace oldest,
// so the section keeps filling in forever without growing.
//
// Ordered oldest-first through at(), because the return map needs consecutive
// pairs and "consecutive" is a statement about time. Reading the ring in slot
// order instead would join the newest point to the oldest once per wrap, which
// is one wrong dot per 8192 and therefore invisible until someone believes it.
type poincareLog struct {
	hits []poincareHit
	head int // next slot to write
	n    int // how many slots hold a hit
	gap  bool
}

func (l *poincareLog) reset(capacity int) {
	if cap(l.hits) < capacity {
		l.hits = make([]poincareHit, capacity)
	}
	l.hits = l.hits[:capacity]
	l.head, l.n = 0, 0
	// The first hit after a reset has no predecessor at all, which is the same
	// situation as a hit after a reseed and takes the same flag.
	l.gap = true
}

// breakChain says the next hit does not follow the previous one in time.
func (l *poincareLog) breakChain() { l.gap = true }

func (l *poincareLog) add(h poincareHit) {
	if len(l.hits) == 0 {
		return
	}
	h.Gap = l.gap
	l.gap = false
	l.hits[l.head] = h
	l.head = (l.head + 1) % len(l.hits)
	if l.n < len(l.hits) {
		l.n++
	}
}

func (l *poincareLog) len() int { return l.n }

// at returns the i-th hit counting from the OLDEST still in the ring.
func (l *poincareLog) at(i int) poincareHit {
	if i < 0 || i >= l.n {
		return poincareHit{}
	}
	start := l.head - l.n
	if start < 0 {
		start += len(l.hits)
	}
	return l.hits[(start+i)%len(l.hits)]
}
