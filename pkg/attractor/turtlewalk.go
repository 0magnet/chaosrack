package attractor

import (
	"fmt"
	"math"

	"github.com/0magnet/pisano/pkg/pisano"
)

// The Turtle's walk, shared by the page (turtle.go, whose file comment says
// what the figure is and why the walk never finishes) and a native rack: the
// sequences and the knobs' dials, the walk held mid-stride with its trail and
// tints, and the camera that holds it in frame. Where the points are drawn,
// and the weight a figure can be given, are the page's.

// turtleSequences is the SEQ knob's dial, in the order pisano lists them. Only
// the Fibonacci sequence takes a multiplier — "fib×k" is a sequence, where
// "lucas×k" is not one pisano offers — so the others ignore it, exactly as the
// --mul flag does.
var turtleSequences = []struct {
	name  string
	build func(mul int) pisano.Sequence
}{
	{"fib", func(mul int) pisano.Sequence {
		if mul > 1 {
			return pisano.Scaled(mul)
		}
		return pisano.Fibonacci()
	}},
	{"lucas", func(int) pisano.Sequence { return pisano.Lucas() }},
	{"tri", func(int) pisano.Sequence { return pisano.Triangular() }},
	{"nat", func(int) pisano.Sequence { return pisano.Naturals() }},
	{"prime", func(int) pisano.Sequence { return pisano.Primes() }},
}

// turtleTrails is how much of the path stays on screen — pisano's --trail, with
// its point counts. Zero means keep everything the buffer will hold: for a
// closed path that is the whole figure many times over, so it stands complete
// and still while the color moves through it; for an open one it is as much of
// the drift as there is room for, and the rest has scrolled away.
var turtleTrails = []struct {
	name string
	n    int
}{
	{"whole", 0},
	{"long", 1500},
	{"short", 400},
	{"comet", 90},
}

// turtleCams is pisano's --cam. A terminal's viewport and the world box here
// are the same problem wearing different clothes: the drawing is bigger than
// what can be shown, so something has to decide what is on screen.
//
// Follow is exact and unwatchable — pinning the head to the middle slides the
// whole of the rest of the drawing on every single frame, so the figure you are
// trying to look at never holds still. Fitting is no better on a path that
// drifts: its bounding box steps every time a batch of old points is trimmed,
// and the whole picture jumps with it.
//
// A scope does not have this problem, and not because it chases anything. The
// timebase cancels the signal's own progression, so the trace stands still
// while it runs through. That is what lock does, and why auto picks it: the
// drift is known exactly — it is what one pass of the period displaces the
// turtle by, which the classification reads off a single pass — so the camera
// moves at exactly that velocity and nothing has to be tracked at all.
var turtleCams = []struct {
	name string
}{
	{"auto"},   // fit if the figure closes, lock if it drifts
	{"fit"},    // scale the trail into the box, every frame
	{"lock"},   // cancel the drift: the figure stands still and the walk runs through it
	{"follow"}, // pin the head to the middle, and let everything else slide
}

const (
	camAuto = iota
	camFit
	camLock
	camFollow
)

// turtlePalette is how many colors a tint has to play with. Six, because that
// is what pisano tints with, and because the modes are designed around it —
// TintStep steps through the whole palette once per circuit.
const turtlePalette = 6

// turtleStepsPerFrame is what Speed 1 means here: pisano's --speed default, in
// its units — path steps drawn per frame.
const turtleStepsPerFrame = 4

// turtleMaxSeen bounds the tinter's memory. Two of the modes remember every
// distinct piece of path ever walked, which for a closed figure is the figure
// however long it is watched — but an open one walked for an hour is a million
// segments, and none of the ones that scrolled off will ever be walked again.
// Starting the tinter over is visible as a change of color, which is honest:
// it no longer knows.
const turtleMaxSeen = 300000

// turtleKey is everything the walk is built from — and ONLY that. Changing one
// of these is asking for a different figure, so the walk starts over; changing
// anything else is asking for the same walk to be shown differently, so it
// carries on. That is why the trail length, the camera, the weight and the
// cycle are not in here: shortening a trail should crop what is on screen, not
// throw the walk away and start it again from the beginning.
type turtleKey struct {
	mod, seq, mul, cap, dim, tint int
}

// turtleWalk is a turtle held mid-stride, and the trail behind it.
type turtleWalk struct {
	// The knobs the walk reads as it goes, set by whoever drives it each
	// frame: the TRAIL and CAM dials' positions, and the trail budget, the
	// panel's Trail slider.
	trailF, camF float32
	budget       int
	// rescaled says the scale has just settled, so the gradient's range is
	// to be read again off the figure at its size.
	rescaled bool

	key    turtleKey
	period pisano.Period
	closed bool
	label  string

	w2     *pisano.Walker // one of these is nil; DIM says which
	w3     *pisano.Walker3
	tinter *pisano.Tinter
	span   int // steps in one circuit, for the age tint's sweep

	pts     []pisano.Pt3 // the trail in lattice coordinates, oldest first
	tint    []float32    // what the tinter said about the step that arrived at each
	dropped int          // points that have scrolled off the far end
	pending float64      // steps owed at speeds below one a frame

	// Which way the figure propagates, for the view modes that aim down it.
	// axisDir is the direction; the lock point below is where that line sits.
	// Looking down the barrel of one of these needs both.
	axisDir [3]float32
	axisSet bool

	scale      float32 // lattice units to world units, held once chosen
	scaleSet   bool
	cx, cy, cz float32 // where the camera is centered, in lattice units
	aimed      bool    // the camera has somewhere to ease from

	vel     [3]float32 // lattice units the walk drifts per step, exactly
	lock    [3]float32 // where that drift started from
	lockSet bool
}

// newTurtleWalk works out the arithmetic and starts the turtle walking, its
// trail kept to the TRAIL dial and the budget. It walks a trail's worth
// immediately, so the figure is there to look at rather than arriving as a
// dot that grows for the first few seconds.
func newTurtleWalk(k turtleKey, trailF, camF float32, budget int) *turtleWalk {
	t := &turtleWalk{key: k, trailF: trailF, camF: camF, budget: budget}
	build := turtleSequences[clampIndex(k.seq, len(turtleSequences))].build

	if k.mod <= 0 {
		// Unreduced: the sequence itself, saturating rather than wrapping. The
		// figure is the plus sign the video ends on.
		u, err := pisano.UnreducedPeriod(build(k.mul))
		if err != nil {
			t.label = "no unreduced period: " + err.Error()
			return t
		}
		t.period = u
	} else {
		t.period = pisano.Compute(build(k.mul), k.mod, k.cap)
	}
	if len(t.period.Terms) == 0 {
		t.label = "no period"
		return t
	}

	// What the walk does forever, decided from one pass without walking it.
	var periods int
	var drift pisano.Pt3 // what the walk displaces by over `over` steps
	over := 1
	moves := t.movesPerPass()
	if k.dim >= 3 {
		s := pisano.Classify3(t.period.Terms)
		t.closed, periods, t.label = s.Closed, s.Periods, s.String()
		// Axial is what the drift accumulates to over a whole cycle of the
		// rotation, which is the only interval a screw translates uniformly
		// over: one pass of a helix also turns, so its Drift alone is not a
		// velocity.
		drift, over = s.Axial, max(1, s.Periods)*moves
		// And where that advance happens. A screw turns about a line, and
		// centering on anything off that line leaves the figure swinging around
		// it — which is what a shifting center looks like when you view it down
		// its own axis. The classification knows the line exactly, so there is
		// nothing to measure and nothing to settle.
		if d := s.AxisDir(); d != (pisano.Pt3{}) {
			t.axisDir = [3]float32{float32(d.X), float32(d.Y), float32(d.Z)}
			t.axisSet = true
		}
		if s.Turn != pisano.IdentityFrame3 {
			num, den := s.Axis()
			d := float32(den)
			t.lock = [3]float32{float32(num.X) / d, float32(num.Y) / d, float32(num.Z) / d}
			t.lockSet = true
		}
		t.w3 = pisano.NewWalker3(t.period)
	} else {
		s := pisano.Classify(t.period.Terms)
		t.closed, periods, t.label = s.Closed, s.Periods, s.String()
		// In the plane a path that turns at all closes, so a path that drifts
		// is a pure translation and one pass of it is a velocity. A closed one
		// has no drift to cancel however its passes are displaced.
		if !s.Closed {
			drift, over = pisano.Pt3{X: s.Drift.X, Y: s.Drift.Y}, moves
		}
		t.w2 = pisano.NewWalker(t.period)
	}
	if over > 0 && !t.closed {
		t.vel = [3]float32{
			float32(drift.X) / float32(over),
			float32(drift.Y) / float32(over),
			float32(drift.Z) / float32(over),
		}
	}
	head := fmt.Sprintf("%s mod %d, period %d", t.period.Seq, k.mod, len(t.period.Terms))
	if k.mod <= 0 {
		head = fmt.Sprintf("%s unreduced, %d terms", t.period.Seq, len(t.period.Terms))
	}
	t.label = head + " — " + t.label

	// A sweep of the palette spans one circuit, so the age tint travels round a
	// closed figure exactly once per lap. Zero terms move nothing, so a circuit
	// is fewer steps than it has terms.
	t.span = max(1, t.movesPerPass()*max(periods, 1))
	t.newTinter()
	// A closed figure is walked once round straight away, so it is there to look
	// at rather than arriving as a dot; an open one has no circuit to prime with
	// and grows from its start, which is the walk beginning where it begins.
	if t.closed {
		t.advance(len(t.period.Terms) * max(periods, 1))
	}
	return t
}

// movesPerPass is how many of a pass's terms actually move the turtle.
func (t *turtleWalk) movesPerPass() int {
	n := 0
	for _, term := range t.period.Terms {
		if term != 0 {
			n++
		}
	}
	return n
}

func (t *turtleWalk) newTinter() {
	mode := pisano.TintModes()[clampIndex(t.key.tint, len(pisano.TintModes()))]
	t.tinter = pisano.NewTinter(mode, turtlePalette, t.span)
}

// trailLen is how many points to keep, which is also what bounds the walk: past
// this, the oldest are gone and the memory does not grow. The panel's own Trail
// slider is the ceiling, since that is the size of the buffer they are drawn
// from — pisano's --max-points, wearing the panel's clothes.
func (t *turtleWalk) trailLen() int {
	n := turtleTrails[clampIndex(int(t.trailF), len(turtleTrails))].n
	if n == 0 {
		// "Whole" is one circuit for a closed figure — not as much of it as
		// there is room for. Keeping more would leave lap upon lap of the same
		// shape on screen in different colors, and the figure would read as a
		// fixed patchwork rather than as something being drawn again. An open
		// path has no circuit, so it keeps what there is room for.
		n = t.budget
		if t.closed {
			n = t.span + 1
		}
	}
	return max(2, min(n, t.budget))
}

// advance walks n terms and drops whatever has aged out of the trail.
func (t *turtleWalk) advance(n int) {
	const inv = 1.0 / float32(turtlePalette)
	for range n {
		var to pisano.Pt3
		var idx int
		switch {
		case t.w3 != nil:
			s, moved := t.w3.Next()
			if !moved {
				continue
			}
			to, idx = s.To, t.tinter.Tint3(s)
		case t.w2 != nil:
			s, moved := t.w2.Next()
			if !moved {
				continue
			}
			to, idx = pisano.Pt3{X: s.To.X, Y: s.To.Y}, t.tinter.Tint(s)
		default:
			return
		}
		if idx < 0 {
			idx = 0 // pisano leaves the run-in uncolored; here it takes the first color
		}
		t.pts = append(t.pts, to)
		t.tint = append(t.tint, (float32(idx)+0.5)*inv)
	}
	t.trim()
	if t.tinter.Len() > turtleMaxSeen {
		// It has remembered more path than can ever be walked again.
		t.newTinter()
	}
}

// trim discards the oldest of the path. It runs in batches rather than every
// frame: each trim shifts the whole slice down, so trimming one point at a time
// would copy the entire trail on every tick for the sake of one element.
func (t *turtleWalk) trim() {
	limit := t.trailLen()
	slack := max(4, limit/8)
	if len(t.pts) <= limit+slack {
		return
	}
	drop := len(t.pts) - limit
	t.pts = append(t.pts[:0], t.pts[drop:]...)
	t.tint = append(t.tint[:0], t.tint[drop:]...)
	t.dropped += drop
}

// aim points the camera, which is the only thing that can move: the figure is
// drawn at a fixed number of world units per lattice step, so it neither shrinks
// as it drifts nor breathes as the trail moves over it.
//
// Fitting is right for a figure that closes — the trail is the same shape every
// time round, so fitting it once is fitting it forever and the figure stands
// still. Following is right for one that drifts, which has no size to fit: the
// scale is taken from the first trail's worth and held, and the camera chases
// the head, which is what makes an endless path watchable.
func (t *turtleWalk) aim(drawn int) {
	cam := t.camera()

	// The held scale has to come from a representative amount of path. Taking it
	// from the first two points a walk produced sets it off the length of one
	// lattice step, which is a scale on which the figure is astronomical — so it
	// keeps being taken until the trail has filled, and only then is held.
	scale := t.scale
	if cam == camFit || !t.scaleSet {
		// Measuring the figure is the only thing the bounding box is still for,
		// and walking every point of a long trail to get it is the most
		// expensive thing this mode does per frame — so it is only walked when
		// the answer is going to be used.
		lo, hi := pisano.Bounds3(t.pts)
		span := float32(max(hi.X-lo.X, hi.Y-lo.Y, hi.Z-lo.Z))
		if span <= 0 {
			span = 1
		}
		scale = box / span
		if cam != camFit && len(t.pts) >= t.scaleSample() {
			t.scaleSet = true
			// The gradient's range was read off whatever was on screen before
			// the figure reached its size; now that it has one, read it again.
			t.rescaled = true
		}
	}

	// Where the camera would like to be, which is not where it goes this frame.
	//
	// Only FOLLOW is allowed to care where the head is. Everything else places
	// the camera by the drift — the one direction the figure is going overall,
	// known exactly from the classification rather than guessed at from the
	// last few frames of a head that is swinging round a helix. Fitting used to
	// center on the trail's bounding box, and that box is defined by exactly
	// the head and tail motion you do not want followed: it steps sideways every
	// time the head turns a corner and every time a batch of tail is trimmed, so
	// the whole picture went with it. Fitting now only decides how big the
	// figure is drawn; where it sits is the drift, the same as lock.
	cxT, cyT, czT := t.drifted(drawn)
	if cam == camFollow {
		head := t.pts[len(t.pts)-1]
		cxT, cyT, czT = float32(head.X), float32(head.Y), float32(head.Z)
	}

	// Ease into it. Every one of these targets moves in jumps — the bounding box
	// steps whenever a batch of old points is trimmed, and the head steps a
	// whole lattice unit — so a camera that went straight there would jerk on
	// every one of them. Easing turns each jump into a glide, and costs nothing
	// when the target is not moving, which for a closed figure being fitted is
	// always. Lock's target moves smoothly by construction and the easing only
	// gives it a constant lag, which is a constant offset in the framing.
	if !t.aimed {
		t.cx, t.cy, t.cz, t.scale, t.aimed = cxT, cyT, czT, scale, true
		return
	}
	const ease = 0.12
	t.cx += (cxT - t.cx) * ease
	t.cy += (cyT - t.cy) * ease
	t.cz += (czT - t.cz) * ease
	t.scale += (scale - t.scale) * ease
}

// drifted is where the figure is now, if you only account for the direction it
// is going overall: the point the walk turns about, carried along the axis at
// exactly the rate one pass of the period displaces it by.
//
// A sine wave on a scope does not need chasing — the timebase cancels the
// signal's own progression and the trace stands still while it runs through.
// Both halves of that are exact here and neither is measured: the drift comes
// from one pass of the period, and the axis a screw turns about comes with it.
func (t *turtleWalk) drifted(drawn int) (x, y, z float32) {
	// The MIDDLE of what is on screen, which is not the end of it. The walk
	// advances by the same amount every step, so a window of `drawn` points
	// ending at the head has its middle half a window back along the drift —
	// and a camera placed at the head is looking at the head, with the figure
	// trailing off behind it and turning about its tip.
	mid := float32(t.dropped+len(t.pts)) - float32(drawn)/2
	if !t.lockSet {
		// A figure with no net turn has no axis to be found, so where it sits is
		// worked back from where the trail actually is — once, when the trail
		// has filled, so the picture does not shift again afterwards.
		lo, hi := pisano.Bounds3(t.pts)
		t.lock = [3]float32{
			float32(lo.X+hi.X)/2 - t.vel[0]*mid,
			float32(lo.Y+hi.Y)/2 - t.vel[1]*mid,
			float32(lo.Z+hi.Z)/2 - t.vel[2]*mid,
		}
		t.lockSet = len(t.pts) >= t.trailLen()
	}
	return t.lock[0] + t.vel[0]*mid,
		t.lock[1] + t.vel[1]*mid,
		t.lock[2] + t.vel[2]*mid
}

// scaleSample is how much of the walk the size is decided from.
//
// It used to be the whole trail, and that was wrong in a way that took a while
// to name: the trail spends its first minute or so filling up, the figure was
// re-fitted into the frame on every frame of it, and the result is a camera
// that backs away continuously for as long as the thing keeps growing. You
// cannot get close to something that is receding at exactly the rate you
// approach it.
//
// A circuit is the size of a closed figure, and a few hundred points is a fair
// look at one that drifts. Either is reached in a couple of seconds, and after
// that the size is settled and the ZOOM knob means something. Fitting stays
// available for when what you want is the whole of it in frame, and it goes on
// re-fitting, because that is what it is for.
func (t *turtleWalk) scaleSample() int {
	n := 600
	if t.closed {
		n = t.span + 1
	}
	return max(2, min(n, t.trailLen()))
}

// box is how much of the world a figure is drawn into, as the globe is.
const box float32 = 2.4

// camera resolves the auto setting the way pisano's does: a figure that closes
// is fitted, and the drift of one that does not is canceled.
func (t *turtleWalk) camera() int {
	c := clampIndex(int(t.camF), len(turtleCams))
	if c != camAuto {
		return c
	}
	if t.closed {
		return camFit
	}
	return camLock
}

func clampIndex(i, n int) int {
	if i < 0 || i >= n {
		return 0
	}
	return i
}

// turtleKeyOf is the walk the knobs ask for.
func turtleKeyOf(mod, seq, mul, capF, dim, tint float32) turtleKey {
	return turtleKey{
		mod:  int(mod),
		seq:  clampIndex(int(seq), len(turtleSequences)),
		mul:  max(1, int(mul)),
		cap:  max(0, int(capF)),
		dim:  int(dim),
		tint: clampIndex(int(tint), len(pisano.TintModes())),
	}
}

// TurtleKnobs are the Turtle's knobs that make and show its walk, by the
// page's ids, in the order TurtleRun.Step takes them.
var TurtleKnobs = []string{"turtle-mod", "turtle-seq", "turtle-mul", "turtle-cap", "turtle-dim", "turtle-tint", "turtle-trail", "turtle-cam", "turtle-turn"}

// TurtleRun is a Turtle walked for a rack without the page: the walk, made
// again when the knobs ask for a different one.
type TurtleRun struct {
	walk *turtleWalk
	body turtleBody
	buf  [][4]float32 // the trail placed, before TURN draws it
}

// TurtlePhysics is the Turtle's weight for a rack without the page: the room
// it lives in, ±HalfW by ±HalfH about the middle; the Physics knobs; and the
// step of one of the page's frames, Dt. The camera is far, as a native view
// is flat.
type TurtlePhysics struct {
	HalfW, HalfH             float32
	Grav, Fric, Bounce, Spin float32
	Dt                       float32
}

// Step walks on by frames of the page's at speed (steps a frame, each
// scale), while running, with the knobs at k (TurtleKnobs' order) and a
// trail budget of budget, and hands put the trail on screen, oldest first,
// with each point's tint: placed as the walk's camera holds it, or with
// phys, where its weight has carried it.
func (r *TurtleRun) Step(k [9]float32, budget int, frames float64, steps int, scale float32, running bool, phys *TurtlePhysics, put func(x, y, z, tint float32)) int {
	key := turtleKeyOf(k[0], k[1], k[2], k[3], k[4], k[5])
	if r.walk == nil || r.walk.key != key {
		r.walk = newTurtleWalk(key, k[6], k[7], budget)
		r.body = turtleBody{} // a new figure is dropped afresh, as the page drops it
	}
	t := r.walk
	t.trailF, t.camF, t.budget = k[6], k[7], budget
	if running {
		// Below one step a frame the rate is carried between frames, or the
		// whole lower half of the Speed knob would round down to zero.
		t.pending += turtleStepsPerFrame * float64(steps) * float64(scale) * frames
		whole := int(t.pending)
		t.pending -= float64(whole)
		t.advance(whole)
	}
	if len(t.pts) < 2 {
		return 0
	}
	n := min(len(t.pts), min(budget, t.trailLen()))
	base := len(t.pts) - n
	t.aim(n)
	t.rescaled = false
	if phys != nil {
		w := turtleWorld{halfW: phys.HalfW, halfH: phys.HalfH, camDist: 1e4,
			grav: phys.Grav, fric: phys.Fric, bounce: phys.Bounce, spin: phys.Spin, dt: phys.Dt}
		if running {
			// A step each of the page's frames this one stands for, each the
			// page's own: one long step would be held to the step limit and
			// slow the figure down.
			for range max(1, int(frames+0.5)) {
				r.body.step(t, t.pts[base:], w)
			}
		}
	} else {
		r.body.placed = false // dropped again next time, from wherever it is standing
	}
	r.buf = t.placed(&r.body, base, n, phys != nil, r.buf)
	return drawTurtle(r.buf, k[8], budget, func(v [4]float32) { put(v[0], v[1], v[2], v[3]) })
}

// placed is the trail's n points from base, x, y, z and tint each, in buf's
// room: where the body has carried them when it has weight, else as the
// walk's camera holds them.
func (t *turtleWalk) placed(body *turtleBody, base, n int, weighed bool, buf [][4]float32) [][4]float32 {
	buf = buf[:0]
	for i := range n {
		p := t.pts[base+i]
		if weighed {
			x, y, z := body.place(t, p)
			buf = append(buf, [4]float32{x, y, z, t.tint[base+i]})
			continue
		}
		// pisano's Y grows downward, matching a terminal; here up is up.
		buf = append(buf, [4]float32{(float32(p.X) - t.cx) * t.scale, -(float32(p.Y) - t.cy) * t.scale, (float32(p.Z) - t.cz) * t.scale, t.tint[base+i]})
	}
	return buf
}

// drawTurtle hands put the placed trail pts as TURN at turn draws it, in at
// most room points, and says how many.
func drawTurtle(pts [][4]float32, turn float32, room int, put func(v [4]float32)) int {
	if turn >= 0.5 {
		return roundCorners(pts, room, put)
	}
	for _, p := range pts {
		put(p)
	}
	return len(pts)
}

// Label is what the walk is: its sequence, modulus and period, and what it
// does when repeated forever.
func (r *TurtleRun) Label() string {
	if r.walk == nil {
		return ""
	}
	return r.walk.label
}

// TurtlePhysicsKnob is one of the Turtle's Physics knobs: its id, its label,
// and where it starts and runs.
type TurtlePhysicsKnob struct {
	ID, Label           string
	Def, Min, Max, Step float32
}

// TurtlePhysicsKnobs are the Physics module's knobs, in its order: what the
// page builds them from, and where a rack without the page starts them —
// the page builds them only with the switch up, so its catalog has no values
// for them.
var TurtlePhysicsKnobs = []TurtlePhysicsKnob{
	{"turtle-grav", "grav", 3, -8, 8, 0.1},       // pull, world units per second squared; below zero lifts
	{"turtle-fric", "fric", 0.6, 0, 2, 0.05},     // how hard the floor and walls grip
	{"turtle-bounce", "bounce", 0.2, 0, 1, 0.05}, // how much of a hit comes back
	{"turtle-spin", "spin", 1, 0.1, 8, 0.1},      // how readily it turns
}

// Rounded turns (TURN on arc): every corner of the walk is drawn as an arc
// from the middle of the step into it to the middle of the step out of it.
// The turtle's steps are all one lattice unit and all its turns square, so
// each arc is a quarter circle of half a step's radius and the path is a chain
// of them, with no straight left in it. It is a way of drawing the walk, not a
// different walk: the turtle still stands on the lattice, and the tints run
// round the arcs as they ran along the steps.

// TurtleTurns are TURN's positions.
var TurtleTurns = []string{"sqr", "arc"}

// turtleArcMost and turtleArcLeast are how many points an arc is drawn with:
// the most when there is room, and the fewest it is still a curve at. Below
// that the oldest of the trail is left off to make room, since the room is the
// vertex buffer's and a turn drawn square would not be the turn asked for.
const (
	turtleArcMost  = 8
	turtleArcLeast = 3
)

// roundCorners hands put the trail pts (x, y, z, tint each, oldest first) with
// its corners turned on arcs, in at most room points, and says how many it
// handed.
func roundCorners(pts [][4]float32, room int, put func(v [4]float32)) int {
	if len(pts) < 3 || room < 2+turtleArcLeast {
		for _, p := range pts[:min(len(pts), room)] {
			put(p)
		}
		return min(len(pts), room)
	}
	k := min(turtleArcMost, (room-2)/(len(pts)-2))
	if k < turtleArcLeast {
		k = turtleArcLeast
		pts = pts[len(pts)-(2+(room-2)/k):]
	}
	put(pts[0])
	n := 1
	for i := 1; i+1 < len(pts); i++ {
		p0, p1, p2 := pts[i-1], pts[i], pts[i+1]
		var a, b [4]float32
		for j := range 4 {
			a[j], b[j] = (p0[j]+p1[j])/2, (p1[j]+p2[j])/2
		}
		for s := range k {
			put(arcPoint(a, p1, b, float32(s)/float32(k)))
			n++
		}
	}
	put(pts[len(pts)-1])
	return n + 1
}

// arcPoint is the point t of the way round the turn at corner c from a to b:
// on the circle that touches both legs where they are when the legs are square
// and alike, as the turtle's are, and on the parabola between them when
// something has bent them out of true. The tint goes the parabola's way.
func arcPoint(a, c, b [4]float32, t float32) [4]float32 {
	var u, w [3]float32
	var uu, ww, uw float32
	for j := range 3 {
		u[j], w[j] = a[j]-c[j], b[j]-c[j]
		uu, ww, uw = uu+u[j]*u[j], ww+w[j]*w[j], uw+u[j]*w[j]
	}
	var out [4]float32
	q0, q1, q2 := (1-t)*(1-t), 2*t*(1-t), t*t
	out[3] = q0*a[3] + q1*c[3] + q2*b[3]
	square := uu > 0 && float32(math.Abs(float64(uu-ww))) < 1e-3*uu && float32(math.Abs(float64(uw))) < 1e-3*uu
	if !square {
		for j := range 3 {
			out[j] = q0*a[j] + q1*c[j] + q2*b[j]
		}
		return out
	}
	// The circle's middle is across the corner from it; from there a is
	// back along w and b back along u.
	th := float64(t) * math.Pi / 2
	co, si := float32(math.Cos(th)), float32(math.Sin(th))
	for j := range 3 {
		mid := c[j] + u[j] + w[j]
		out[j] = mid - w[j]*co - u[j]*si
	}
	return out
}
