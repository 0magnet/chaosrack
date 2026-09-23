//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"strconv"
	"syscall/js"
)

// Mode wiring for Sprott Morph (the coefficient-space machinery lives in
// sprottmorph.go). One continuous trajectory integrates while the 30
// quadratic coefficients slide between catalog systems — the self-
// programming analog computer re-patching itself mid-run. The sys knob
// parks the machine anywhere in the A…S cycle (fractional = between two
// systems); the rate knob makes it step itself, in systems per minute.

var (
	morphSysKnob float32 = 3 // catalog position the sys knob requests (3 = D)
	morphRate    float32 = 3 // self-programming speed, systems/minute

	morphSystems []sprottMorphSys
	morphM       float64 = 3 // live catalog position (knob + auto-advance)
	morphKnobPrv float32 = 3
	morphSX      float64
	morphSY      float64
	morphSZ      float64
	morphRing    []float64
	morphHead    int
	morphFill    int
	morphActive  bool
	morphLED     js.Value
	morphTick    int
)

// morphStep advances the trajectory one Euler tick under the blended flow,
// with a guard that reseeds onto the blend's home IC when the trajectory has
// stopped being one — because it ran away, or because it is standing still.
func morphStep(c *[30]float64, dt float64) {
	dx, dy, dz := evalQuad(c, morphSX, morphSY, morphSZ)
	// STANDING STILL IS AS DEAD AS DIVERGING, and only the second was caught.
	//
	// The state starts at the origin, which most of these systems have as an
	// exact fixed point: system D is (-y, x+z, xz+3y²), so at (0,0,0) all
	// three derivatives are exactly zero. The trajectory never moved, never
	// left the bounds, never went NaN, so the divergence guard below never
	// fired and the model sat there — a perfectly static "attractor" at the
	// default sys=3. Sprott A escapes only because its ż is 1−y² = 1 at the
	// origin, which is why this looked like it worked on some settings.
	//
	// Exact zeroes, not a threshold: a real trajectory slowing to a crawl near
	// a fixed point is the interesting part of these systems and must not be
	// jammed. Only a derivative that is identically zero means the integrator
	// can never move again whatever the step size.
	if dx == 0 && dy == 0 && dz == 0 {
		morphReseed()
		return
	}
	morphSX += dx * dt
	morphSY += dy * dt
	morphSZ += dz * dt
	bad := morphSX != morphSX || morphSY != morphSY || morphSZ != morphSZ ||
		morphSX < -60 || morphSX > 60 || morphSY < -60 || morphSY > 60 || morphSZ < -60 || morphSZ > 60
	if bad {
		morphReseed()
	}
}

// morphReseed puts the state back on the blend's home initial condition, with
// a little jitter so a reseed onto a fixed point does not land exactly on it
// again.
func morphReseed() {
	if len(morphSystems) == 0 {
		return
	}
	i := int(morphM) % len(morphSystems)
	if i < 0 {
		i = 0
	}
	ic := morphSystems[i].ic
	morphSX = float64(ic[0]) + 0.01*jamRand()
	morphSY = float64(ic[1]) + 0.01*jamRand()
	morphSZ = float64(ic[2]) + 0.01*jamRand()
}

// generateSprottMorph integrates the blended flow into the trail ring and
// keeps the PATCH readout current.
func generateSprottMorph() {
	if morphSystems == nil {
		morphSystems = sprottMorphSystems()
	}
	// The sys knob seizes the position when the user moves it; otherwise the
	// machine advances itself at the rate knob's systems-per-minute.
	if morphKnobPrv != morphSysKnob {
		morphKnobPrv = morphSysKnob
		morphM = float64(morphSysKnob)
	}
	n := speedSteps
	if n < 1 {
		n = 1
	}
	morphM += float64(morphRate) / 60 / 60 * float64(n) * float64(speedScale)
	for morphM >= float64(len(morphSystems)) {
		morphM -= float64(len(morphSystems))
	}
	c, dt, i, j, frac := morphBlend(morphSystems, morphM)
	if len(morphRing) != steps*3 {
		morphRing = make([]float64, steps*3)
		morphHead, morphFill = 0, 0
	}
	for s := 0; s < n; s++ {
		morphStep(&c, dt*float64(speedScale))
		morphRing[morphHead*3] = morphSX
		morphRing[morphHead*3+1] = morphSY
		morphRing[morphHead*3+2] = morphSZ
		morphHead = (morphHead + 1) % steps
		if morphFill < steps {
			morphFill++
		}
	}
	if morphFill < 2 {
		return
	}
	vertices := vertBuf[:steps*4]
	invN := float32(1) / float32(steps-1)
	for k := 0; k < steps; k++ {
		age := steps - 1 - k
		idx := 0
		if age < morphFill {
			idx = (morphHead - 1 - age + steps + steps) % steps
		} else {
			idx = (morphHead - morphFill + steps + steps) % steps
		}
		v := k * 4
		vertices[v] = float32(morphRing[idx*3])
		vertices[v+1] = float32(morphRing[idx*3+1])
		vertices[v+2] = float32(morphRing[idx*3+2])
		vertices[v+3] = float32(k) * invN
	}
	uploadVerticesOnly(vertices, attractorDrawMode, steps)
	// PATCH readout: "D→E 42%" (throttled — DOM writes are not free).
	morphTick++
	if morphLED.Truthy() && morphTick%10 == 0 {
		// A dash, not an arrow — the DSEG LED font has no → glyph.
		txt := morphSystems[i].letter
		if frac >= 0.005 {
			txt += "-" + morphSystems[j].letter + " " + strconv.Itoa(int(frac*100+0.5)) + "%"
		}
		morphLED.Set("textContent", txt)
	}
}

// syncSprottMorphExtras: entry warms the ring on the blend at the knob and
// frames the camera from the warmed extent; while active, the Patch module
// (static markup, big wired readout) is shown.
func syncSprottMorphExtras(mode string) {
	if sect := dom.Doc.Call("getElementById", "smorph-module"); sect.Truthy() {
		if mode == "sprottmorph" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	morphLED = dom.Doc.Call("getElementById", "smorph-led")
	if mode != "sprottmorph" {
		morphActive = false
		return
	}
	if !morphActive {
		morphActive = true
		if morphSystems == nil {
			morphSystems = sprottMorphSystems()
		}
		morphM = float64(morphSysKnob)
		c, dt, _, _, _ := morphBlend(morphSystems, morphM)
		i := int(morphM) % len(morphSystems)
		ic := morphSystems[i].ic
		morphSX, morphSY, morphSZ = float64(ic[0]), float64(ic[1]), float64(ic[2])
		morphRing = make([]float64, steps*3)
		ext := 0.0
		for k := 0; k < steps; k++ {
			morphStep(&c, dt)
			morphRing[k*3] = morphSX
			morphRing[k*3+1] = morphSY
			morphRing[k*3+2] = morphSZ
			for _, v := range []float64{morphSX, morphSY, morphSZ} {
				if v > ext {
					ext = v
				}
				if -v > ext {
					ext = -v
				}
			}
		}
		morphHead, morphFill = 0, steps
		// Frame the warmed structure directly (no fit-ordering dependence).
		if ext < 0.5 {
			ext = 0.5
		}
		view.fitOverride = float32(ext)
		dist := fitDistFor(view.fitOverride)
		view.initDist = dist
		view.defaultDist = dist
		updateViewMatrix()
	}
}
