//go:build js && wasm

package attractor

import (
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/scopegames"
)

// Bouncing Ball — the classic analog-computer demo, its rules in
// scopegames.Ball. The scope shows the familiar train of shrinking parabolic
// arcs. The trail IS the trajectory (scan semantics like the flows), so
// persist paints the full arc family, and each floor hit blips the shared
// audio context at a pitch set by impact speed.

// bouncingBall is the Bouncing Ball demo: the ball, its trail, and the audio
// context it holds.
type bouncingBall struct {
	scopegames.Ball
	ring       []float64
	head       int
	fill       int
	ctxHeld    bool
	active     bool
	shownKicks int // last count latched onto the Launcher LED
}

var ball = bouncingBall{Ball: scopegames.NewBall(jamRand), shownKicks: -1}

func init() { ball.Beep = ball.beep }

// generateBounceBall advances the integrators (sub-stepped by the Speed
// control like the flows) into a private position ring, then streams the
// ring oldest→newest into the trail buffer.
func (b *bouncingBall) generateBounceBall() {
	if len(b.ring) != sim.steps*2 {
		b.ring = make([]float64, sim.steps*2)
		b.head, b.fill = 0, 0
	}
	n := max(sim.speedSteps, 1)
	dt := scopegames.BallFrameDT * float64(sim.speedScale)
	for range n {
		b.Step(dt)
		b.ring[b.head*2] = b.X
		b.ring[b.head*2+1] = b.Y
		b.head = (b.head + 1) % sim.steps
		if b.fill < sim.steps {
			b.fill++
		}
	}
	if b.Kicks != b.shownKicks {
		b.shownKicks = b.Kicks
		if led := dom.Doc.Call("getElementById", "bounce-kicks"); led.Truthy() {
			setDotText(led, strconv.Itoa(b.Kicks))
		}
	}
	if b.fill < 2 {
		return
	}
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	for i := range sim.steps {
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

// beep: one short sine blip on the shared context. The acquire only
// audibly resumes once some real user gesture has unlocked audio; until
// then the demo just runs silent.
func (b *bouncingBall) beep(freq float64, ms int) {
	if run.selectedMode != "bounceball" {
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
	g.Get("gain").Call("setValueAtTime", scopegames.BallBlipGain, now)
	g.Get("gain").Call("linearRampToValueAtTime", 0, now+dur)
	osc.Call("connect", g)
	mixEnsure(ctx)
	g.Call("connect", mixIn("fx")) // the Mixer's SOUNDS column
	osc.Call("start")
	osc.Call("stop", now+dur+0.01)
}

// syncBounceExtras runs on every panel rebuild: entering re-drops the ball
// face-on; leaving releases the blip lease.
func (b *bouncingBall) syncBounceExtras(mode string) {
	// The module is in the rack whatever the model (panelhtml_js.go): the rack
	// is the same instrument under every model, so its panels do not come and go.
	if mode == "bounceball" {
		if b.active {
			return
		}
		b.active = true
		b.Enter()
		// Warm the ring with a real trajectory (blips muted) so every camera
		// fit measures true arcs, never a near-empty ring.
		b.Warm = true
		b.ring = make([]float64, sim.steps*2)
		for i := range sim.steps {
			b.Step(scopegames.BallFrameDT)
			b.ring[i*2] = b.X
			b.ring[i*2+1] = b.Y
		}
		b.Warm = false
		b.head, b.fill = 0, sim.steps
		normalizeOrientation()
		// The trail ring is nearly empty at entry, so any auto-fit would frame
		// a speck. Hand the next fit the demo box's real extent AND set the
		// camera directly (the hyper-Rössler pattern — no ordering dependence
		// on which autoFitCamera call consumes the one-shot override).
		view.fitOverride = scopegames.BallWall + 0.1
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
