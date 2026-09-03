package pisano

import "fmt"

// The turtle in three dimensions.
//
// In the plane the heading lives in Z/4 and one pass of the period is a rigid
// motion: turn through some whole number of right angles, then translate. That
// is what lets Classify decide closure without walking. In space the heading
// lives in the rotation group of the cube — order 24 — and one pass is still a
// rigid motion, so the same argument works and says more.
//
// Iterating a rigid motion (R, t) k times displaces by
//
//	S = (I + R + … + R^{k-1}) t
//
// and taking k to be the order of R makes that sum k times the projection of t
// onto R's fixed space. So the path closes exactly when the drift has no
// component along the axis of rotation. That reduces correctly: in the plane a
// non-trivial rotation fixes only the origin, the sum always vanishes, and
// every turning path closes — which is the two-dimensional theorem.
//
// Space adds an outcome the plane cannot have. When there is a net turn AND an
// axial drift, iterating is a screw motion: the figure winds along its own axis
// forever. It also adds a period: element orders in Z/4 are 1, 2 and 4, but in
// the rotation group of the cube they are 1, 2, 3 and 4, so a path can close
// after exactly three passes — from a rotation about a body diagonal.
//
// The rule
//
// A term still turns the turtle and steps it forward one unit, and a zero term
// still does neither. What is new is which way the turn goes: the parity of the
// term picks left or right, as in the plane, and the parity of the NEXT term
// picks whether the turn is a yaw or a pitch.
//
// Reading the pair rather than a second bit of one term is not arbitrary. The
// pair (F_n, F_{n+1}) mod m is the state of the recurrence, and the Pisano
// period is precisely the period of that pair — it is why the period is the
// length it is. Driving the third dimension from the pair keeps the figure a
// picture of the thing the arithmetic is about.
//
// A variant taking both bits from the one term was measured against this and
// lost on every count: it cannot use the third dimension at all when the
// modulus is 2 (a residue has no second bit there) and under-uses it for small
// moduli generally; its helices wind about the initial "up" axis far more than
// anything else, which is the starting frame leaking into the figure; and its
// figures come out flatter. Over the Fibonacci moduli to 3000 this rule turns
// out of plane on 51–58% of moves whatever the modulus, against 38–45% and
// falling for the other.

// Pt3 is a lattice point in space. Y grows downward, matching Pt.
type Pt3 struct{ X, Y, Z int }

// Add is the vector sum.
func (a Pt3) Add(b Pt3) Pt3 { return Pt3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

func (a Pt3) neg() Pt3     { return Pt3{-a.X, -a.Y, -a.Z} }
func (a Pt3) isZero() bool { return a == Pt3{} }

// Frame3 is the turtle's orientation: where it faces, which way is up, and
// which way is right.
//
// Right is heading × up throughout, so the frame stays right-handed and every
// turn below is a rotation rather than a reflection — which is what keeps one
// pass a rigid motion and the argument above valid.
type Frame3 struct{ H, U, R Pt3 }

// IdentityFrame3 faces +x with +y overhead, so right is +z.
var IdentityFrame3 = Frame3{H: Pt3{1, 0, 0}, U: Pt3{0, 1, 0}, R: Pt3{0, 0, 1}}

// The four quarter turns. Yaw is about up and pitch is about right; together
// they generate the whole rotation group of the cube, so the turtle can reach
// any of its 24 orientations without ever needing to roll.
func (f Frame3) yawLeft() Frame3   { return Frame3{H: f.R.neg(), U: f.U, R: f.H} }
func (f Frame3) yawRight() Frame3  { return Frame3{H: f.R, U: f.U, R: f.H.neg()} }
func (f Frame3) pitchUp() Frame3   { return Frame3{H: f.U, U: f.H.neg(), R: f.R} }
func (f Frame3) pitchDown() Frame3 { return Frame3{H: f.U.neg(), U: f.H, R: f.R} }

// Apply rotates a vector by the rotation carrying IdentityFrame3 to f. The
// frame's axes are that rotation's columns, in the order (H, U, R).
func (f Frame3) Apply(v Pt3) Pt3 {
	return Pt3{
		X: f.H.X*v.X + f.U.X*v.Y + f.R.X*v.Z,
		Y: f.H.Y*v.X + f.U.Y*v.Y + f.R.Y*v.Z,
		Z: f.H.Z*v.X + f.U.Z*v.Y + f.R.Z*v.Z,
	}
}

// Compose is f applied after g.
func (f Frame3) Compose(g Frame3) Frame3 {
	return Frame3{H: f.Apply(g.H), U: f.Apply(g.U), R: f.Apply(g.R)}
}

// Order is how many times the rotation must be applied to return to where it
// started: 1, 2, 3 or 4 for a rotation of the cube, three being the one with no
// counterpart in the plane.
func (f Frame3) Order() int {
	g := f
	for k := 1; k <= 24; k++ {
		if g == IdentityFrame3 {
			return k
		}
		g = f.Compose(g)
	}
	return 0 // unreachable for a rotation of the cube
}

// Turtle3 is a position and an orientation. It starts at the origin facing +x.
type Turtle3 struct {
	Pos   Pt3
	Frame Frame3
}

// NewTurtle3 is a turtle at the origin in the identity frame. The zero value
// would have a degenerate frame, so this is not optional.
func NewTurtle3() Turtle3 { return Turtle3{Frame: IdentityFrame3} }

// Step applies one term, given the term that follows it. It reports whether the
// turtle moved: a zero term turns nothing and goes nowhere.
func (t *Turtle3) Step(term, next int) bool {
	if term == 0 {
		return false
	}
	left := term&1 != 0
	pitch := next&1 != 0
	switch {
	case pitch && left:
		t.Frame = t.Frame.pitchUp()
	case pitch:
		t.Frame = t.Frame.pitchDown()
	case left:
		t.Frame = t.Frame.yawLeft()
	default:
		t.Frame = t.Frame.yawRight()
	}
	t.Pos = t.Pos.Add(t.Frame.H)
	return true
}

// Shape3 classifies a path in space, exactly, from one pass.
type Shape3 struct {
	Closed  bool
	Helical bool // a net turn and an axial drift: it screws away forever
	Periods int  // passes to close, or the order of the rotation if it never does
	Turn    Frame3
	Drift   Pt3
	Axial   Pt3 // what the drift accumulates to over Periods passes
}

func (s Shape3) String() string {
	switch {
	case s.Closed:
		return fmt.Sprintf("closed after %d pass(es)", s.Periods)
	case s.Helical:
		return fmt.Sprintf("helical, order %d, advances (%d,%d,%d) per %d passes",
			s.Periods, s.Axial.X, s.Axial.Y, s.Axial.Z, s.Periods)
	default:
		return fmt.Sprintf("open, drifts (%d,%d,%d) per pass", s.Drift.X, s.Drift.Y, s.Drift.Z)
	}
}

// Axis is where the motion happens, as against what it is.
//
// A screw is a rotation about a LINE together with a translation along it, and
// Axial says only how far it advances — not where the line is. That is the
// difference between a figure that turns on the spot and one that swings around
// something off to the side, and without it a viewer that cancels the advance
// still has the figure orbiting the axis it failed to find.
//
// The point returned is on the line, as a fraction: the axis passes through
// (num.X/den, num.Y/den, num.Z/den). It is exact, and it is a fraction because
// the axis of a lattice motion generally misses the lattice — a rotation by a
// quarter turn about a line halfway between two rows of points is not a rotation
// about any point.
//
// Finding it needs no matrix inverted. Take the motion apart into the advance
// along the axis, which is Axial/Periods per pass, and what is left, which is a
// pure rotation of finite order. The orbit of any point under a pure rotation is
// the vertices of a regular polygon centered on the axis — so the average of one
// orbit is a point on it, and averaging is arithmetic.
func (s Shape3) Axis() (num Pt3, den int) {
	k := s.Periods
	if k < 1 {
		return Pt3{}, 1
	}
	// Scaled by k so nothing has to be a fraction yet: k·(what is left of the
	// translation once the advance along the axis is taken out).
	rest := Pt3{
		X: k*s.Drift.X - s.Axial.X,
		Y: k*s.Drift.Y - s.Axial.Y,
		Z: k*s.Drift.Z - s.Axial.Z,
	}
	var sum, p Pt3
	for i := 0; i < k; i++ {
		sum = sum.Add(p)
		p = s.Turn.Apply(p).Add(rest)
	}
	return sum, k * k
}

// Passes is how many times to walk the period for a given render: whatever it
// takes for a closed path, or the caller's repeat count for one that never
// closes. It mirrors Shape.Passes.
func (s Shape3) Passes(reps int) int {
	if s.Closed {
		return s.Periods
	}
	if reps < 1 {
		reps = 1
	}
	return reps * s.Periods
}

// Classify3 reads the shape of a path off a single pass of the repeating block.
//
// Only Terms is consulted, as in Classify: a run-in is a prefix, and a prefix
// cannot change what iterating the block does forever.
func Classify3(terms []int) Shape3 {
	r, t := pass3(terms)
	k := r.Order()

	var s Pt3
	acc := t
	for i := 0; i < k; i++ {
		if i > 0 {
			acc = r.Apply(acc)
		}
		s = s.Add(acc)
	}

	sh := Shape3{Periods: k, Turn: r, Drift: t, Axial: s}
	switch {
	case s.isZero():
		sh.Closed = true
	case r != IdentityFrame3:
		sh.Helical = true
	}
	return sh
}

// pass3 walks one period and reports the rigid motion it performed.
//
// The term after the last is the first, which is not a fudge: the block really
// is periodic there, so that is genuinely the next term. It also has to be, or
// one pass would not be the same motion every time and nothing above would
// hold.
func pass3(terms []int) (Frame3, Pt3) {
	t := NewTurtle3()
	for i := range terms {
		t.Step(terms[i], terms[(i+1)%len(terms)])
	}
	return t.Frame, t.Pos
}

// Step3 is one move of a walk in space, carrying everything a renderer might
// key a color on. It mirrors Step.
type Step3 struct {
	From, To Pt3
	Term     int    // the term that produced it
	Pass     int    // which walk of the repeating block, or -1 for the run-in
	Dir      Frame3 // the orientation it moved in
	Index    int    // how many steps came before it
}

// Walk3 walks a whole period in space: the run-in once, then the repeating
// block as many times as asked, reporting every move. Terms that do not move
// the turtle produce no step, so a pass contributes fewer steps than it has
// terms. It mirrors Walk.
func Walk3(p Period, passes int) []Step3 {
	if passes < 1 {
		passes = 1
	}
	if !p.Bounded {
		// Terms is a truncated prefix, not a cycle. Repeating it would draw a
		// loop the sequence does not have.
		passes = 1
	}
	if len(p.Terms) == 0 {
		return nil
	}

	t := NewTurtle3()
	steps := make([]Step3, 0, len(p.Head)+len(p.Terms)*passes)
	move := func(term, next, pass int) {
		from := t.Pos
		if !t.Step(term, next) {
			return
		}
		steps = append(steps, Step3{
			From: from, To: t.Pos, Term: term,
			Pass: pass, Dir: t.Frame, Index: len(steps),
		})
	}

	// The run-in reads its successor from the stream, which at its end is the
	// first term of the block.
	for i, term := range p.Head {
		next := p.Terms[0]
		if i+1 < len(p.Head) {
			next = p.Head[i+1]
		}
		move(term, next, -1)
	}
	for pass := 0; pass < passes; pass++ {
		for i := range p.Terms {
			move(p.Terms[i], p.Terms[(i+1)%len(p.Terms)], pass)
		}
	}
	return steps
}

// Path3 is every lattice point a walk visits, starting at the origin. It is
// what a renderer wants: a polyline.
func Path3(p Period, passes int) []Pt3 {
	steps := Walk3(p, passes)
	if len(steps) == 0 {
		return []Pt3{{}}
	}
	pts := make([]Pt3, 0, len(steps)+1)
	pts = append(pts, steps[0].From)
	for _, s := range steps {
		pts = append(pts, s.To)
	}
	return pts
}

// Bounds3 is the smallest box containing a path.
func Bounds3(pts []Pt3) (lo, hi Pt3) {
	if len(pts) == 0 {
		return
	}
	lo, hi = pts[0], pts[0]
	for _, p := range pts {
		lo = Pt3{min(lo.X, p.X), min(lo.Y, p.Y), min(lo.Z, p.Z)}
		hi = Pt3{max(hi.X, p.X), max(hi.Y, p.Y), max(hi.Z, p.Z)}
	}
	return lo, hi
}

// Span3 is the dimension of the space a path actually occupies: 3 for a figure
// that uses the room it was given, 2 for one that stayed flat.
func Span3(pts []Pt3) int {
	if len(pts) < 2 {
		return 0
	}
	var basis []Pt3
	for _, p := range pts[1:] {
		v := Pt3{p.X - pts[0].X, p.Y - pts[0].Y, p.Z - pts[0].Z}
		if v.isZero() {
			continue
		}
		switch len(basis) {
		case 0:
			basis = append(basis, v)
		case 1:
			if !parallel3(basis[0], v) {
				basis = append(basis, v)
			}
		default:
			if det3(basis[0], basis[1], v) != 0 {
				return 3
			}
		}
	}
	return len(basis)
}

func parallel3(a, b Pt3) bool {
	return a.Y*b.Z-a.Z*b.Y == 0 && a.Z*b.X-a.X*b.Z == 0 && a.X*b.Y-a.Y*b.X == 0
}

func det3(a, b, c Pt3) int {
	return a.X*(b.Y*c.Z-b.Z*c.Y) - a.Y*(b.X*c.Z-b.Z*c.X) + a.Z*(b.X*c.Y-b.Y*c.X)
}

// AxisDir is the direction the path propagates: the screw axis as a primitive
// integer vector, or the zero vector when there is no single direction.
//
// This is the companion to Axis, which says where the axis line IS and not
// which way it points. Both are needed to look down the barrel of one of these
// figures: the point says where to aim, this says which way to face.
//
// Two cases, and the difference matters. When the motion has a turn, the axis
// is the direction that turn leaves alone — the eigenvector of the rotation.
// Summing the rotation over one full turn projects any vector onto that
// direction (the components across the axis cancel, being the vertices of a
// regular polygon about it), so a basis vector that does not cancel to nothing
// gives the axis in integers, with no eigen-solving and no floating point.
//
// When there is no turn the rotation leaves everything alone and has no axis at
// all; the figure is a pure translation, and the direction it propagates is its
// drift. That is the honest answer to "which way is this going" for a path that
// simply marches.
//
// The result is reduced to its primitive form, so parallel axes compare equal.
func (s Shape3) AxisDir() Pt3 {
	if s.Turn == IdentityFrame3 {
		return primitive3(s.Drift)
	}
	k := s.Turn.Order()
	for _, basis := range []Pt3{{X: 1}, {Y: 1}, {Z: 1}} {
		var sum, p Pt3
		p = basis
		for i := 0; i < k; i++ {
			sum = sum.Add(p)
			p = s.Turn.Apply(p)
		}
		if !sum.isZero() {
			return primitive3(sum)
		}
	}
	return Pt3{}
}

// primitive3 divides a vector by the greatest common divisor of its parts, so
// that two parallel directions are the same vector rather than two multiples of
// one. The sign is left alone: a screw has a handedness and reversing the axis
// would discard it.
func primitive3(v Pt3) Pt3 {
	g := gcd3(gcd3(abs3(v.X), abs3(v.Y)), abs3(v.Z))
	if g <= 1 {
		return v
	}
	return Pt3{X: v.X / g, Y: v.Y / g, Z: v.Z / g}
}

func gcd3(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func abs3(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
