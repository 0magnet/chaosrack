//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dom"
)

// Bouncing Ball — the classic analog-computer demo (Telefunken shipped it
// with their RA-series machines to sell integrators): two integrator
// chains (velocity → position) under constant gravity, a comparator that
// flips the vertical velocity at the floor with a restitution loss, and
// wall reflections for the horizontal drift. The scope shows the familiar
// train of shrinking parabolic arcs; when the bounces decay away, the
// machine re-kicks the ball — the loop that ran unattended in trade-show
// windows. The trail IS the trajectory (scan semantics like the flows),
// so persist paints the full arc family, and each floor hit blips the
// shared audio context at a pitch set by impact speed.

// bouncingBall is the Bouncing Ball demo: the ball, its trail, and the audio
// context it holds.
type bouncingBall struct {
	grav       float32 // gravity
	rest       float32 // restitution (bounce energy keep)
	drift      float32 // horizontal speed
	x, y       float64
	vx, vy     float64
	ring       []float64
	head       int
	fill       int
	ctxHeld    bool
	active     bool
	warm       bool // entry warmup in progress (mute the blips)
	kicks      int  // machine re-kicks since mode entry
	shownKicks int  // last count latched onto the Launcher LED
}

var ball = bouncingBall{
	grav:       12,
	rest:       0.88,
	drift:      0.7,
	x:          -1.2,
	y:          0.9,
	vx:         0.7,
	shownKicks: -1,
}

const (
	bounceFloor = -1.0
	bounceCeil  = 1.0
	bounceWall  = 1.4
)

// bounceStep advances the ball by one integrator tick.
func (b *bouncingBall) step(dt float64) {
	b.vy -= float64(b.grav) * dt
	b.x += b.vx * dt
	b.y += b.vy * dt
	// Comparator: floor bounce with restitution, plus a blip whose pitch
	// follows the impact speed (harder hit = higher blip).
	if b.y < bounceFloor && b.vy < 0 {
		b.y = bounceFloor
		imp := -b.vy
		b.vy = imp * float64(b.rest)
		b.beep(180+90*imp, 45)
		// Re-kick when the bounces have decayed away, like the unattended
		// trade-show loop: fresh upward velocity, fresh drift direction.
		if b.vy < 0.25 {
			b.vy = 3.6 + 1.4*jamRand()
			b.vx = float64(b.drift) * (0.4 + 0.6*jamRand())
			if jamRand() < 0.5 {
				b.vx = -b.vx
			}
			if !b.warm {
				b.kicks++
			}
			b.beep(490, 90)
		}
	}
	if b.y > bounceCeil && b.vy > 0 {
		b.y = bounceCeil
		b.vy = -b.vy * float64(b.rest)
		b.beep(320, 40)
	}
	if b.x > bounceWall && b.vx > 0 || b.x < -bounceWall && b.vx < 0 {
		b.vx = -b.vx
		b.beep(226, 40)
	}
	// The drift knob retunes the horizontal speed live (sign preserved).
	if b.vx > 0 {
		b.vx = float64(b.drift)
	} else if b.vx < 0 {
		b.vx = -float64(b.drift)
	}
}

// generateBounceBall advances the integrators (sub-stepped by the Speed
// control like the flows) into a private position ring, then streams the
// ring oldest→newest into the trail buffer.
func (b *bouncingBall) generateBounceBall() {
	if len(b.ring) != sim.steps*2 {
		b.ring = make([]float64, sim.steps*2)
		b.head, b.fill = 0, 0
	}
	n := sim.speedSteps
	if n < 1 {
		n = 1
	}
	dt := 0.016 * float64(sim.speedScale)
	for s := 0; s < n; s++ {
		b.step(dt)
		b.ring[b.head*2] = b.x
		b.ring[b.head*2+1] = b.y
		b.head = (b.head + 1) % sim.steps
		if b.fill < sim.steps {
			b.fill++
		}
	}
	if b.kicks != b.shownKicks {
		b.shownKicks = b.kicks
		if led := dom.Doc.Call("getElementById", "bounce-kicks"); led.Truthy() {
			led.Set("textContent", strconv.Itoa(b.kicks))
		}
	}
	if b.fill < 2 {
		return
	}
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	for i := 0; i < sim.steps; i++ {
		// Oldest sample first; before the ring fills, backfill with the oldest
		// we have so the strip stays degenerate rather than garbage.
		age := sim.steps - 1 - i
		idx := 0
		if age < b.fill {
			idx = (b.head - 1 - age + sim.steps + sim.steps) % sim.steps
		} else {
			idx = (b.head - b.fill + sim.steps + sim.steps) % sim.steps
		}
		j := i * 4
		vertices[j] = float32(b.ring[idx*2])
		vertices[j+1] = float32(b.ring[idx*2+1])
		vertices[j+2] = 0
		vertices[j+3] = float32(i) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
}

// bounceBeep: one short sine blip on the shared context. The acquire only
// audibly resumes once some real user gesture has unlocked audio; until
// then the demo just runs silent.
func (b *bouncingBall) beep(freq float64, ms int) {
	if b.warm || run.selectedMode != "bounceball" {
		return
	}
	ctx := acquireAudioCtx("bounce")
	if !ctx.Truthy() {
		return
	}
	b.ctxHeld = true
	osc := ctx.Call("createOscillator")
	g := ctx.Call("createGain")
	osc.Set("type", "sine")
	osc.Get("frequency").Set("value", freq)
	now := ctx.Get("currentTime").Float()
	dur := float64(ms) / 1000
	g.Get("gain").Call("setValueAtTime", 0.1, now)
	g.Get("gain").Call("linearRampToValueAtTime", 0, now+dur)
	osc.Call("connect", g)
	g.Call("connect", ctx.Get("destination"))
	osc.Call("start")
	osc.Call("stop", now+dur+0.01)
}

// syncBounceExtras runs on every panel rebuild: entering re-drops the ball
// face-on; leaving releases the blip lease.
func (b *bouncingBall) syncBounceExtras(mode string) {
	if sect := dom.Doc.Call("getElementById", "bounce-module"); sect.Truthy() {
		if mode == "bounceball" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	if mode == "bounceball" {
		if b.active {
			return
		}
		b.active = true
		b.kicks = 0
		b.x, b.y = -1.2, bounceDropHeight()
		b.vx, b.vy = float64(b.drift), 0
		// Warm the ring with a real trajectory (blips muted) so every camera
		// fit measures true arcs, never a near-empty ring.
		b.warm = true
		b.ring = make([]float64, sim.steps*2)
		for i := 0; i < sim.steps; i++ {
			b.step(0.016)
			b.ring[i*2] = b.x
			b.ring[i*2+1] = b.y
		}
		b.warm = false
		b.head, b.fill = 0, sim.steps
		normalizeOrientation()
		// The trail ring is nearly empty at entry, so any auto-fit would frame
		// a speck. Hand the next fit the demo box's real extent AND set the
		// camera directly (the hyper-Rössler pattern — no ordering dependence
		// on which autoFitCamera call consumes the one-shot override).
		view.fitOverride = bounceWall + 0.1
		dist := fitDistFor(view.fitOverride)
		view.initDist = dist
		view.defaultDist = dist
		view.updateViewMatrix()
		return
	}
	b.active = false
	if b.ctxHeld {
		releaseAudioCtx("bounce")
		b.ctxHeld = false
	}
}
