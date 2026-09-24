//go:build js && wasm

package attractor

import (
	"github.com/0magnet/chaosrack/pkg/dom"
	"github.com/0magnet/chaosrack/pkg/dynamics"
	"strconv"
	"syscall/js"
)

// Mode wiring for Sprott Morph (the coefficient-space machinery lives in
// sprottmorph.go). One continuous trajectory integrates while the 30
// quadratic coefficients slide between catalog systems — the self-
// programming analog computer re-patching itself mid-run. The sys knob
// parks the machine anywhere in the A…S cycle (fractional = between two
// systems); the rate knob makes it step itself, in systems per minute.

// sprottMorph is the Sprott Morph mode: the path through coefficient space,
// its trail, and its knobs.
type sprottMorph struct {
	sysKnob float32 // catalog position the sys knob requests (3 = D)
	rate    float32 // self-programming speed, systems/minute
	systems []dynamics.SprottMorphSys
	m       float64 // live catalog position (knob + auto-advance)
	knobPrv float32
	sx      float64
	sy      float64
	sz      float64
	ring    []float64
	head    int
	fill    int
	active  bool
	led     js.Value
	tick    int
}

var morph = sprottMorph{
	sysKnob: 3,
	rate:    3,
	m:       3,
	knobPrv: 3,
}

// step advances the trajectory one Euler tick under the blended flow,
// with a guard that reseeds onto the blend's home IC when the trajectory has
// stopped being one — because it ran away, or because it is standing still.
func (sp *sprottMorph) step(c *[30]float64, dt float64) {
	dx, dy, dz := dynamics.EvalQuad(c, sp.sx, sp.sy, sp.sz)
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
		sp.reseed()
		return
	}
	sp.sx += dx * dt
	sp.sy += dy * dt
	sp.sz += dz * dt
	bad := sp.sx != sp.sx || sp.sy != sp.sy || sp.sz != sp.sz ||
		sp.sx < -60 || sp.sx > 60 || sp.sy < -60 || sp.sy > 60 || sp.sz < -60 || sp.sz > 60
	if bad {
		sp.reseed()
	}
}

// reseed puts the state back on the blend's home initial condition, with
// a little jitter so a reseed onto a fixed point does not land exactly on it
// again.
func (sp *sprottMorph) reseed() {
	if len(sp.systems) == 0 {
		return
	}
	i := max(int(sp.m)%len(sp.systems), 0)
	ic := sp.systems[i].IC
	sp.sx = float64(ic[0]) + 0.01*jamRand()
	sp.sy = float64(ic[1]) + 0.01*jamRand()
	sp.sz = float64(ic[2]) + 0.01*jamRand()
}

// generateSprottMorph integrates the blended flow into the trail ring and
// keeps the PATCH readout current.
func (sp *sprottMorph) generateSprottMorph() {
	if sp.systems == nil {
		sp.systems = dynamics.SprottMorphSystems()
	}
	// The sys knob seizes the position when the user moves it; otherwise the
	// machine advances itself at the rate knob's systems-per-minute.
	if sp.knobPrv != sp.sysKnob {
		sp.knobPrv = sp.sysKnob
		sp.m = float64(sp.sysKnob)
	}
	n := max(sim.speedSteps, 1)
	sp.m += float64(sp.rate) / 60 / 60 * float64(n) * float64(sim.speedScale)
	for sp.m >= float64(len(sp.systems)) {
		sp.m -= float64(len(sp.systems))
	}
	c, dt, i, j, frac := dynamics.SprottMorphBlend(sp.systems, sp.m)
	if len(sp.ring) != sim.steps*3 {
		sp.ring = make([]float64, sim.steps*3)
		sp.head, sp.fill = 0, 0
	}
	for range n {
		sp.step(&c, dt*float64(sim.speedScale))
		sp.ring[sp.head*3] = sp.sx
		sp.ring[sp.head*3+1] = sp.sy
		sp.ring[sp.head*3+2] = sp.sz
		sp.head = (sp.head + 1) % sim.steps
		if sp.fill < sim.steps {
			sp.fill++
		}
	}
	if sp.fill < 2 {
		return
	}
	vertices := sim.vertBuf[:sim.steps*4]
	invN := float32(1) / float32(sim.steps-1)
	for k := range sim.steps {
		age := sim.steps - 1 - k
		idx := 0
		if age < sp.fill {
			idx = (sp.head - 1 - age + sim.steps + sim.steps) % sim.steps
		} else {
			idx = (sp.head - sp.fill + sim.steps + sim.steps) % sim.steps
		}
		v := k * 4
		vertices[v] = float32(sp.ring[idx*3])
		vertices[v+1] = float32(sp.ring[idx*3+1])
		vertices[v+2] = float32(sp.ring[idx*3+2])
		vertices[v+3] = float32(k) * invN
	}
	gpu.uploadVerticesOnly(vertices, gpu.drawMode, sim.steps)
	// PATCH readout: "D→E 42%" (throttled — DOM writes are not free).
	sp.tick++
	if sp.led.Truthy() && sp.tick%10 == 0 {
		// A dash, not an arrow — the DSEG LED font has no → glyph.
		txt := sp.systems[i].Letter
		if frac >= 0.005 {
			txt += "-" + sp.systems[j].Letter + " " + strconv.Itoa(int(frac*100+0.5)) + "%"
		}
		sp.led.Set("textContent", txt)
	}
}

// syncSprottMorphExtras: entry warms the ring on the blend at the knob and
// frames the camera from the warmed extent; while active, the Patch module
// (static markup, big wired readout) is shown.
func (sp *sprottMorph) syncSprottMorphExtras(mode string) {
	if sect := dom.Doc.Call("getElementById", "smorph-module"); sect.Truthy() {
		if mode == "sprottmorph" {
			sect.Get("style").Set("display", "")
		} else {
			sect.Get("style").Set("display", "none")
		}
	}
	sp.led = dom.Doc.Call("getElementById", "smorph-led")
	if mode != "sprottmorph" {
		sp.active = false
		return
	}
	if !sp.active {
		sp.active = true
		if sp.systems == nil {
			sp.systems = dynamics.SprottMorphSystems()
		}
		sp.m = float64(sp.sysKnob)
		c, dt, _, _, _ := dynamics.SprottMorphBlend(sp.systems, sp.m)
		i := int(sp.m) % len(sp.systems)
		ic := sp.systems[i].IC
		sp.sx, sp.sy, sp.sz = float64(ic[0]), float64(ic[1]), float64(ic[2])
		sp.ring = make([]float64, sim.steps*3)
		ext := 0.0
		for k := range sim.steps {
			sp.step(&c, dt)
			sp.ring[k*3] = sp.sx
			sp.ring[k*3+1] = sp.sy
			sp.ring[k*3+2] = sp.sz
			for _, v := range []float64{sp.sx, sp.sy, sp.sz} {
				if v > ext {
					ext = v
				}
				if -v > ext {
					ext = -v
				}
			}
		}
		sp.head, sp.fill = 0, sim.steps
		// Frame the warmed structure directly (no fit-ordering dependence).
		if ext < 0.5 {
			ext = 0.5
		}
		view.fitOverride = float32(ext)
		dist := fitDistFor(view.fitOverride)
		view.initDist = dist
		view.defaultDist = dist
		view.updateViewMatrix()
	}
}
