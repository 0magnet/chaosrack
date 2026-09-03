//go:build js && wasm

package attractor

// TURTLE — the Pisano turtle path, in two dimensions or three.
//
// Reduce an integer sequence modulo m and the remainders repeat; the length of
// the repeat is the Pisano period. Read each term as an instruction — odd turns
// left and steps forward, even turns right and steps forward, zero does neither
// — and the walk draws a figure.
//
// In three dimensions the parity of the term still picks left or right, and the
// parity of the NEXT term picks whether the turn is a yaw or a pitch. Reading
// the pair is the point: (F_n, F_{n+1}) mod m is the state of the recurrence,
// and the Pisano period is the period of that pair, which is why the period is
// the length it is.
//
// Both the arithmetic and the walk come from github.com/0magnet/pisano, which
// also decides — from a single pass, without walking further — whether the
// figure closes, drifts in a straight line, or screws away along an axis.
//
// THE WALK NEVER FINISHES, and it is never restarted. The turtle is held
// mid-stride and extended a few steps every frame, forever, the way the
// attractors integrate; the trail is the last so many points of it and the
// oldest fall off the far end as new ones arrive. Nothing is ever redrawn from
// the beginning, so there is no lap and no seam — a closed path simply arrives
// back where it started and keeps going, which is what makes it closed.
//
// That is also what makes the tints work. Two of pisano's seven color a step by
// what has happened to it before — how many times it has been walked, and in
// which direction — so a closed figure walked round again comes back in the
// next color while a path that doubles back shows where. Feeding one continuous
// walk through one tinter is the only way that means anything.
//
// There is no vector field here, so it is not a flow: nothing is being
// integrated, and the FLOW sonification (which integrates a field at audio
// rate) has nothing to integrate. It is a curve traced in time, which is what
// ClassParametric is, so Model Out SCAN traces the drawn trail as a wavetable —
// you hear the figure's shape, and the modulus changes the timbre.

import (
	"fmt"
	"syscall/js"

	"github.com/0magnet/pisano/pkg/pisano"
)

// The knobs. These are pisano's command-line flags, as far as they mean
// anything here: --mod, --seq, --mul, --cap, --tint, --trail, --cam, --cycle.
// The ones left behind write files (--out, --split, --cell, --cols, --grid,
// --labels), dress a terminal (--theme, --plain, --mono), or are a different
// design altogether (--circle). --speed is the panel's own Speed knob and
// --paused its Pause switch; --max-points is the Trail slider, which is what
// bounds the walk here. DIM is the one knob with no flag behind it: pisano
// draws in a terminal, which has no third dimension to offer.
var (
	turtleModF   float32 = 25 // 0 draws the sequence unreduced
	turtleSeqF   float32      // index into turtleSequences
	turtleMulF   float32 = 1  // multiply the Fibonacci sequence by this
	turtleCapF   float32      // term limit; 0 lets the modulus choose its own
	turtleDimF   float32 = 3
	turtleTintF  float32 // index into pisano.TintModes()
	turtleTrailF float32 // index into turtleTrails
	turtleCamF   float32 // index into turtleCams
	turtleViewF  float32 // index into turtleViews: which way to face the axis
	turtleCycleF float32 // seconds between moduli; 0 stays on one

	// Weight. PHYS is the switch — gravity cannot be it now that pulling upward
	// is a thing you can ask for, and zero gravity inside a box is a perfectly
	// good place to be: it floats, and you can still throw it at a wall.
	turtleGravF   float32 = 3 // pull, world units per second squared; below zero lifts
	turtleFricF   float32 = 0.6
	turtleBounceF float32 = 0.2
	turtleSpinF   float32 = 1 // how hard the figure is to turn
)

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

func turtleCamNames() []string {
	return turtleNames(len(turtleCams), func(i int) string { return turtleCams[i].name })
}
func turtleTrailNames() []string {
	return turtleNames(len(turtleTrails), func(i int) string { return turtleTrails[i].name })
}
func turtleSeqNames() []string {
	return turtleNames(len(turtleSequences), func(i int) string { return turtleSequences[i].name })
}
func turtleTintNames() []string {
	m := pisano.TintModes()
	return turtleNames(len(m), func(i int) string { return m[i].String() })
}

func turtleNames(n int, at func(int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = at(i)
	}
	return out
}

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

	body turtleBody // where it has got to, when it has weight

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

var turtleState *turtleWalk

// turtleCycleAt is when the CYCLE knob next steps the modulus (frameNowMs).
var turtleCycleAt float64

// turtleInfoTick paces the Info overlay's refresh. What it says about the walk
// — how much of it is on screen, how far it has got — is true only for the
// frame it was written on, and rewriting it sixty times a second to update a
// number nobody can read that fast is waste.
var turtleInfoTick int

func generateTurtle() {
	key := turtleKeyNow()
	if turtleState == nil || turtleState.key != key {
		turtleState = newTurtleWalk(key)
		// The knobs moved, so the sentence under Info is about a different
		// figure now. Doing it here rather than every frame keeps it to the one
		// moment it can change.
		updateInfoOverlay()
	}
	t := turtleState
	turtleCycle()

	// The upload path centers a trail on the running mean of what it is handed,
	// which for an attractor is the whole orbit but here would fight the camera
	// below for control of where the figure sits. The walk is placed exactly, so
	// hold that and offer no offset of our own.
	centerOffset = [3]float32{}
	centerReady = true

	// A held figure stops walking. The march is the walk extruding new points
	// at the head, so a figure that keeps being made while you are holding it
	// keeps trying to march out of your hand — you are pulling one way and the
	// floor is driving it the other, and neither of you is winning. Picking a
	// thing up should stop it going anywhere.
	if !paused && !turtleGrabState.held && !turtleSpinDrag && !turtleTiltDrag {
		// Below one step a frame the rate has to be carried between frames, or
		// truncation would round the whole Speed knob's lower half down to zero
		// and the walk would stand still.
		t.pending += turtleStepsPerFrame * float64(speedSteps) * float64(speedScale)
		whole := int(t.pending)
		t.pending -= float64(whole)
		t.advance(whole)

		turtleInfoTick++
		if turtleInfoTick >= 30 {
			turtleInfoTick = 0
			updateInfoOverlay()
		}
	}
	if len(t.pts) < 2 {
		uploadVerticesOnly(vertBuf[:0], attractorDrawMode, 0)
		return
	}

	// Exactly a trail's worth, never the slack as well. Trimming runs in
	// batches — one at a time would copy the whole slice for the sake of one
	// element — so what is HELD sawtooths between the trail length and an eighth
	// above it. Drawing all of it made the figure grow by that eighth and snap
	// back a couple of times a second, which is a wobble with nothing in the
	// walk behind it.
	n := min(len(t.pts), min(steps, t.trailLen()))
	base := len(t.pts) - n // the newest n points: the head is always on screen

	// Worked out before aiming, because where to look depends on how much is
	// being looked at: the camera belongs at the middle of the drawn window.
	t.aim(n)
	applyTurtleView(t)

	if physOn() {
		// Weight takes over the placing. The camera decides where to look from
		// when nothing is pushing the figure around; once something is, where it
		// has got to is the answer, and a camera holding it in the middle would
		// be hiding exactly what there is to watch.
		if !paused {
			t.body.step(t, t.pts[base:])
		}
		for i := 0; i < n; i++ {
			x, y, z := t.body.place(t, t.pts[base+i])
			d := i * 4
			vertBuf[d], vertBuf[d+1], vertBuf[d+2], vertBuf[d+3] = x, y, z, t.tint[base+i]
		}
		uploadVerticesOnly(vertBuf[:n*4], attractorDrawMode, n)
		return
	}
	t.body.placed = false // dropped again next time, from wherever it is standing

	for i := 0; i < n; i++ {
		p := t.pts[base+i]
		d := i * 4
		// pisano's Y grows downward, matching a terminal; here up is up.
		vertBuf[d] = (float32(p.X) - t.cx) * t.scale
		vertBuf[d+1] = -(float32(p.Y) - t.cy) * t.scale
		vertBuf[d+2] = (float32(p.Z) - t.cz) * t.scale
		vertBuf[d+3] = t.tint[base+i]
	}
	uploadVerticesOnly(vertBuf[:n*4], attractorDrawMode, n)
}

// newTurtleWalk works out the arithmetic and starts the turtle walking. It
// walks a trail's worth immediately, so the figure is there to look at rather
// than arriving as a dot that grows for the first few seconds.
func newTurtleWalk(k turtleKey) *turtleWalk {
	t := &turtleWalk{key: k}
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
	n := turtleTrails[clampIndex(int(turtleTrailF), len(turtleTrails))].n
	if n == 0 {
		// "Whole" is one circuit for a closed figure — not as much of it as
		// there is room for. Keeping more would leave lap upon lap of the same
		// shape on screen in different colors, and the figure would read as a
		// fixed patchwork rather than as something being drawn again. An open
		// path has no circuit, so it keeps what there is room for.
		n = steps
		if t.closed {
			n = t.span + 1
		}
	}
	return max(2, min(n, steps))
}

// advance walks n terms and drops whatever has aged out of the trail.
func (t *turtleWalk) advance(n int) {
	const inv = 1.0 / float32(turtlePalette)
	for i := 0; i < n; i++ {
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
			refreshGradient()
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
	c := clampIndex(int(turtleCamF), len(turtleCams))
	if c != camAuto {
		return c
	}
	if t.closed {
		return camFit
	}
	return camLock
}

// turtleCycle is pisano's --cycle: step to the next modulus every so often. It
// drives the knob rather than the variable, so the panel readout, the permalink
// and the restart all follow from the one place they normally would.
func turtleCycle() {
	secs := float64(turtleCycleF)
	if secs <= 0 || paused {
		turtleCycleAt = 0
		return
	}
	if turtleCycleAt == 0 {
		turtleCycleAt = frameNowMs + secs*1000
		return
	}
	if frameNowMs < turtleCycleAt {
		return
	}
	turtleCycleAt = frameNowMs + secs*1000
	knob := doc.Call("getElementById", "turtle-mod")
	if !knob.Truthy() {
		return
	}
	next := int(turtleModF) + 1
	if next > int(turtleModMax) {
		next = 1
	}
	knob.Set("value", next)
	knob.Call("dispatchEvent", js.Global().Get("Event").New("input", map[string]interface{}{"bubbles": true}))
}

// turtleModMax is the MOD knob's top, kept beside the knob definition it has to
// agree with so cycling wraps where the dial does.
const turtleModMax float32 = 300

func turtleKeyNow() turtleKey {
	return turtleKey{
		mod:  int(turtleModF),
		seq:  clampIndex(int(turtleSeqF), len(turtleSequences)),
		mul:  max(1, int(turtleMulF)),
		cap:  max(0, int(turtleCapF)),
		dim:  int(turtleDimF),
		tint: clampIndex(int(turtleTintF), len(pisano.TintModes())),
	}
}

func clampIndex(i, n int) int {
	if i < 0 || i >= n {
		return 0
	}
	return i
}

// turtleShapeLabel is what the Info overlay adds for this mode: which figure is
// being walked and what it does when the walk is repeated forever — decided
// from one pass, without walking it — then how far the walk has actually got.
func turtleShapeLabel() string {
	t := turtleState
	if t == nil {
		return ""
	}
	tint := pisano.TintModes()[clampIndex(t.key.tint, len(pisano.TintModes()))]
	trail := turtleTrails[clampIndex(int(turtleTrailF), len(turtleTrails))].name
	cam := turtleCams[clampIndex(int(turtleCamF), len(turtleCams))].name
	if cam == "auto" {
		cam += "/" + turtleCams[t.camera()].name
	}
	walked := t.dropped + len(t.pts)
	return fmt.Sprintf("%s\ntint %v, trail %s (%d of %d points walked), cam %s",
		t.label, tint, trail, len(t.pts), walked, cam)
}
