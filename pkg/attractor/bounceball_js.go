//go:build js && wasm

package attractor

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

var (
	bounceGrav  float32 = 12   // gravity
	bounceRest  float32 = 0.88 // restitution (bounce energy keep)
	bounceDrift float32 = 0.7  // horizontal speed

	bounceX, bounceY   float64 = -1.2, 0.9
	bounceVX, bounceVY float64 = 0.7, 0
	bounceRing         []float64
	bounceHead         int
	bounceFill         int
	bounceCtxHeld      bool
	bounceActive       bool
	bounceWarm         bool // entry warmup in progress (mute the blips)
)

const (
	bounceFloor = -1.0
	bounceCeil  = 1.0
	bounceWall  = 1.4
)

// bounceStep advances the ball by one integrator tick.
func bounceStep(dt float64) {
	bounceVY -= float64(bounceGrav) * dt
	bounceX += bounceVX * dt
	bounceY += bounceVY * dt
	// Comparator: floor bounce with restitution, plus a blip whose pitch
	// follows the impact speed (harder hit = higher blip).
	if bounceY < bounceFloor && bounceVY < 0 {
		bounceY = bounceFloor
		imp := -bounceVY
		bounceVY = imp * float64(bounceRest)
		bounceBeep(180+90*imp, 45)
		// Re-kick when the bounces have decayed away, like the unattended
		// trade-show loop: fresh upward velocity, fresh drift direction.
		if bounceVY < 0.25 {
			bounceVY = 3.6 + 1.4*jamRand()
			bounceVX = float64(bounceDrift) * (0.4 + 0.6*jamRand())
			if jamRand() < 0.5 {
				bounceVX = -bounceVX
			}
			bounceBeep(490, 90)
		}
	}
	if bounceY > bounceCeil && bounceVY > 0 {
		bounceY = bounceCeil
		bounceVY = -bounceVY * float64(bounceRest)
		bounceBeep(320, 40)
	}
	if bounceX > bounceWall && bounceVX > 0 || bounceX < -bounceWall && bounceVX < 0 {
		bounceVX = -bounceVX
		bounceBeep(226, 40)
	}
	// The drift knob retunes the horizontal speed live (sign preserved).
	if bounceVX > 0 {
		bounceVX = float64(bounceDrift)
	} else if bounceVX < 0 {
		bounceVX = -float64(bounceDrift)
	}
}

// generateBounceBall advances the integrators (sub-stepped by the Speed
// control like the flows) into a private position ring, then streams the
// ring oldest→newest into the trail buffer.
func generateBounceBall() {
	if len(bounceRing) != steps*2 {
		bounceRing = make([]float64, steps*2)
		bounceHead, bounceFill = 0, 0
	}
	n := speedSteps
	if n < 1 {
		n = 1
	}
	dt := 0.016 * float64(speedScale)
	for s := 0; s < n; s++ {
		bounceStep(dt)
		bounceRing[bounceHead*2] = bounceX
		bounceRing[bounceHead*2+1] = bounceY
		bounceHead = (bounceHead + 1) % steps
		if bounceFill < steps {
			bounceFill++
		}
	}
	if bounceFill < 2 {
		return
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	for i := 0; i < steps; i++ {
		// Oldest sample first; before the ring fills, backfill with the oldest
		// we have so the strip stays degenerate rather than garbage.
		age := steps - 1 - i
		idx := 0
		if age < bounceFill {
			idx = (bounceHead - 1 - age + steps + steps) % steps
		} else {
			idx = (bounceHead - bounceFill + steps + steps) % steps
		}
		j := i * 4
		vertices[j] = float32(bounceRing[idx*2])
		vertices[j+1] = float32(bounceRing[idx*2+1])
		vertices[j+2] = 0
		vertices[j+3] = float32(i) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
}

// bounceBeep: one short sine blip on the shared context. The acquire only
// audibly resumes once some real user gesture has unlocked audio; until
// then the demo just runs silent.
func bounceBeep(freq float64, ms int) {
	if bounceWarm || selectedMode != "bounceball" {
		return
	}
	ctx := acquireAudioCtx("bounce")
	if !ctx.Truthy() {
		return
	}
	bounceCtxHeld = true
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
func syncBounceExtras(mode string) {
	if mode == "bounceball" {
		if bounceActive {
			return
		}
		bounceActive = true
		bounceX, bounceY = -1.2, 0.9
		bounceVX, bounceVY = float64(bounceDrift), 0
		// Warm the ring with a real trajectory (blips muted) so every camera
		// fit measures true arcs, never a near-empty ring.
		bounceWarm = true
		bounceRing = make([]float64, steps*2)
		for i := 0; i < steps; i++ {
			bounceStep(0.016)
			bounceRing[i*2] = bounceX
			bounceRing[i*2+1] = bounceY
		}
		bounceWarm = false
		bounceHead, bounceFill = 0, steps
		normalizeOrientation()
		// The trail ring is nearly empty at entry, so any auto-fit would frame
		// a speck. Hand the next fit the demo box's real extent AND set the
		// camera directly (the hyper-Rössler pattern — no ordering dependence
		// on which autoFitCamera call consumes the one-shot override).
		fitExtentOverride = bounceWall + 0.1
		dist := fitExtentOverride * 3
		initCameraDist = dist
		defaultCameraDist = dist
		updateViewMatrix()
		return
	}
	bounceActive = false
	if bounceCtxHeld {
		releaseAudioCtx("bounce")
		bounceCtxHeld = false
	}
}
