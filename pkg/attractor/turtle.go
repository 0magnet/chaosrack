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

	"github.com/0magnet/chaosrack/pkg/dom"

	"github.com/0magnet/pisano/pkg/pisano"
)

// turtleMode is the Turtle mode: the walk and its knobs.
type turtleMode struct {
	// The knobs. These are pisano's command-line flags, as far as they mean
	// anything here: --mod, --seq, --mul, --cap, --tint, --trail, --cam, --cycle.
	// The ones left behind write files (--out, --split, --cell, --cols, --grid,
	// --labels), dress a terminal (--theme, --plain, --mono), or are a different
	// design altogether (--circle). --speed is the panel's own Speed knob and
	// --paused its Pause switch; --max-points is the Trail slider, which is what
	// bounds the walk here. DIM is the one knob with no flag behind it: pisano
	// draws in a terminal, which has no third dimension to offer.
	modF   float32 // 0 draws the sequence unreduced
	seqF   float32 // index into turtleSequences
	mulF   float32 // multiply the Fibonacci sequence by this
	capF   float32 // term limit; 0 lets the modulus choose its own
	dimF   float32
	tintF  float32 // index into pisano.TintModes()
	trailF float32 // index into turtleTrails
	camF   float32 // index into turtleCams
	viewF  float32 // index into turtleViews: which way to face the axis
	cycleF float32 // seconds between moduli; 0 stays on one
	turnF  float32 // index into TurtleTurns: square corners or arcs

	// Weight. PHYS is the switch — gravity cannot be it now that pulling upward
	// is a thing you can ask for, and zero gravity inside a box is a perfectly
	// good place to be: it floats, and you can still throw it at a wall.
	gravF   float32 // pull, world units per second squared; below zero lifts
	fricF   float32
	bounceF float32
	spinF   float32 // how hard the figure is to turn
	state   *turtleWalk
	placed  [][4]float32 // the trail placed, before TURN draws it

	// body is where the figure has got to when it has weight (turtlebody.go),
	// a fresh one with each new walk.
	body turtleBody

	// cycleAt is when the CYCLE knob next steps the modulus (frameNowMs).
	cycleAt float64

	// infoTick paces the Info overlay's refresh. What it says about the walk
	// — how much of it is on screen, how far it has got — is true only for the
	// frame it was written on, and rewriting it sixty times a second to update a
	// number nobody can read that fast is waste.
	infoTick int
}

var turtle = turtleMode{
	modF:    25,
	mulF:    1,
	dimF:    3,
	gravF:   3,
	fricF:   0.6,
	bounceF: 0.2,
	spinF:   1,
}

func (tu *turtleMode) generateTurtle() {
	key := tu.keyNow()
	if tu.state == nil || tu.state.key != key {
		tu.state = newTurtleWalk(key, tu.trailF, tu.camF, sim.steps)
		tu.body = turtleBody{}
		// The knobs moved, so the sentence under Info is about a different
		// figure now. Doing it here rather than every frame keeps it to the one
		// moment it can change.
		updateInfoOverlay()
	}
	t := tu.state
	t.trailF, t.camF, t.budget = tu.trailF, tu.camF, sim.steps
	tu.cycle()

	// The upload path centers a trail on the running mean of what it is handed,
	// which for an attractor is the whole orbit but here would fight the camera
	// below for control of where the figure sits. The walk is placed exactly, so
	// hold that and offer no offset of our own.
	sim.centerOffset = [3]float32{}
	sim.centerReady = true

	// A held figure stops walking. The march is the walk extruding new points
	// at the head, so a figure that keeps being made while you are holding it
	// keeps trying to march out of your hand — you are pulling one way and the
	// floor is driving it the other, and neither of you is winning. Picking a
	// thing up should stop it going anywhere.
	if !run.paused && !grab.grabState.held && !grab.spinDrag && !grab.tiltDrag {
		// Below one step a frame the rate has to be carried between frames, or
		// truncation would round the whole Speed knob's lower half down to zero
		// and the walk would stand still.
		t.pending += turtleStepsPerFrame * float64(sim.speedSteps) * float64(sim.speedScale)
		whole := int(t.pending)
		t.pending -= float64(whole)
		t.advance(whole)

		tu.infoTick++
		if tu.infoTick >= 30 {
			tu.infoTick = 0
			updateInfoOverlay()
		}
	}
	if len(t.pts) < 2 {
		gpu.uploadVerticesOnly(sim.vertBuf[:0], gpu.drawMode, 0)
		return
	}

	// Exactly a trail's worth, never the slack as well. Trimming runs in
	// batches — one at a time would copy the whole slice for the sake of one
	// element — so what is HELD sawtooths between the trail length and an eighth
	// above it. Drawing all of it made the figure grow by that eighth and snap
	// back a couple of times a second, which is a wobble with nothing in the
	// walk behind it.
	n := min(len(t.pts), min(sim.steps, t.trailLen()))
	base := len(t.pts) - n // the newest n points: the head is always on screen

	// Worked out before aiming, because where to look depends on how much is
	// being looked at: the camera belongs at the middle of the drawn window.
	t.aim(n)
	if t.rescaled {
		// The gradient's range was read off whatever was on screen before
		// the figure reached its size; now that it has one, read it again.
		t.rescaled = false
		refreshGradient()
	}
	applyTurtleView(t)

	weighed := physOn()
	if weighed {
		// Square on, every frame and not only when the switch is thrown: a link's
		// pose is applied after its switches, a load with none is given a random
		// one, and the View's knobs still turn the camera. Turned, the floor was
		// not the bottom of the picture, and the figure fell, and was pulled by
		// the hand, at the angle the view was turned by.
		if !viewSquare() {
			normalizeOrientation()
		}
		// Weight takes over the placing. The camera decides where to look from
		// when nothing is pushing the figure around; once something is, where it
		// has got to is the answer, and a camera holding it in the middle would
		// be hiding exactly what there is to watch.
		if !run.paused {
			tu.body.step(t, t.pts[base:], pageTurtleWorld())
		}
	} else {
		tu.body.placed = false // dropped again next time, from wherever it is standing
	}
	tu.placed = t.placed(&tu.body, base, n, weighed, tu.placed)
	// Drawn as TURN says, into the vertex buffer, which is the room an arc's
	// points have.
	d := 0
	drawn := drawTurtle(tu.placed, tu.turnF, len(sim.vertBuf)/4, func(v [4]float32) {
		copy(sim.vertBuf[d:d+4], v[:])
		d += 4
	})
	gpu.uploadVerticesOnly(sim.vertBuf[:drawn*4], gpu.drawMode, drawn)
}

// cycle is pisano's --cycle: step to the next modulus every so often. It
// drives the knob rather than the variable, so the panel readout, the permalink
// and the restart all follow from the one place they normally would.
func (tu *turtleMode) cycle() {
	secs := float64(tu.cycleF)
	if secs <= 0 || run.paused {
		tu.cycleAt = 0
		return
	}
	if tu.cycleAt == 0 {
		tu.cycleAt = frameNowMs + secs*1000
		return
	}
	if frameNowMs < tu.cycleAt {
		return
	}
	tu.cycleAt = frameNowMs + secs*1000
	knob := dom.Doc.Call("getElementById", "turtle-mod")
	if !knob.Truthy() {
		return
	}
	next := int(tu.modF) + 1
	if next > int(turtleModMax) {
		next = 1
	}
	knob.Set("value", next)
	dom.Fire(knob, "input")
}

// turtleModMax is the MOD knob's top, kept beside the knob definition it has to
// agree with so cycling wraps where the dial does.
const turtleModMax float32 = 300

func (tu *turtleMode) keyNow() turtleKey {
	return turtleKeyOf(tu.modF, tu.seqF, tu.mulF, tu.capF, tu.dimF, tu.tintF)
}

// shapeLabel is what the Info overlay adds for this mode: which figure is
// being walked and what it does when the walk is repeated forever — decided
// from one pass, without walking it — then how far the walk has actually got.
func (tu *turtleMode) shapeLabel() string {
	t := tu.state
	if t == nil {
		return ""
	}
	tint := pisano.TintModes()[clampIndex(t.key.tint, len(pisano.TintModes()))]
	trail := turtleTrails[clampIndex(int(tu.trailF), len(turtleTrails))].name
	cam := turtleCams[clampIndex(int(tu.camF), len(turtleCams))].name
	if cam == "auto" {
		cam += "/" + turtleCams[t.camera()].name
	}
	walked := t.dropped + len(t.pts)
	return fmt.Sprintf("%s\ntint %v, trail %s (%d of %d points walked), cam %s",
		t.label, tint, trail, len(t.pts), walked, cam)
}

// The dials' names, for the page's panel.

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
