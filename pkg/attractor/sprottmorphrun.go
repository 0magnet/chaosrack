package attractor

import (
	"math"
	"strconv"

	"github.com/0magnet/chaosrack/pkg/dynamics"
)

// The Sprott Morph's run, shared by the page (sprottmorph_js.go) and a native
// rack: one continuous trajectory integrating while the 30 quadratic
// coefficients slide between catalog systems — the self-programming analog
// computer re-patching itself mid-run. The sys knob parks the machine
// anywhere in the A…S cycle (fractional = between two systems); the rate
// knob makes it step itself, in systems per minute. The coefficient-space
// machinery is in pkg/dynamics (sprottmorph.go).

// SprottMorph is the morph's path through coefficient space, its trail and
// its knobs.
type SprottMorph struct {
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
	label   string
	rand    func() float64 // 0..1, for the reseed's jitter
}

// NewSprottMorph is the morph at the page's defaults, jittering its reseeds
// with rand.
func NewSprottMorph(rand func() float64) *SprottMorph {
	return &SprottMorph{sysKnob: 3, rate: 3, m: 3, knobPrv: 3, rand: rand}
}

// SprottMorphKnobs are the morph's knobs, by the page's ids.
var SprottMorphKnobs = []string{"smorph-sys", "smorph-rate"}

// Set turns knob id to v, reporting whether the morph has it.
func (sp *SprottMorph) Set(id string, v float32) bool {
	switch id {
	case "smorph-sys":
		sp.sysKnob = v
	case "smorph-rate":
		sp.rate = v
	default:
		return false
	}
	return true
}

// step advances the trajectory one RK4 tick under the blended flow,
// with a guard that reseeds onto the blend's home IC when the trajectory has
// stopped being one — because it ran away, or because it is standing still.
func (sp *SprottMorph) step(c *[30]float64, dt float64) {
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
	// RK4, as the catalog's own Sprott models step: forward Euler at their
	// timesteps is a different system, and three of them (L, M, Q) are not
	// chaotic under it — the machine showed a periodic L that the Sprott L
	// model beside it does not.
	f := func(x, y, z float64) (float64, float64, float64) { return dynamics.EvalQuad(c, x, y, z) }
	sp.sx, sp.sy, sp.sz = dynamics.RK4(f, dt, sp.sx, sp.sy, sp.sz)
	bad := sp.sx != sp.sx || sp.sy != sp.sy || sp.sz != sp.sz ||
		sp.sx < -60 || sp.sx > 60 || sp.sy < -60 || sp.sy > 60 || sp.sz < -60 || sp.sz > 60
	if bad {
		sp.reseed()
	}
}

// reseed puts the state back on the blend's home initial condition, with
// a little jitter so a reseed onto a fixed point does not land exactly on it
// again.
func (sp *SprottMorph) reseed() {
	if len(sp.systems) == 0 {
		return
	}
	i := max(int(sp.m)%len(sp.systems), 0)
	ic := sp.systems[i].IC
	sp.sx = float64(ic[0]) + 0.01*sp.rand()
	sp.sy = float64(ic[1]) + 0.01*sp.rand()
	sp.sz = float64(ic[2]) + 0.01*sp.rand()
}

// Active reports whether the morph has been entered since it last left.
func (sp *SprottMorph) Active() bool { return sp.active }

// Leave marks the morph left, so coming back enters it afresh.
func (sp *SprottMorph) Leave() { sp.active = false }

// Enter starts the morph on the system its sys knob names, with a fresh
// trail of steps points, and reports the trail's reach from the origin, which
// frames it.
func (sp *SprottMorph) Enter(steps int) float64 {
	sp.active = true
	if sp.systems == nil {
		sp.systems = dynamics.SprottMorphSystems()
	}
	sp.m = float64(sp.sysKnob)
	sp.knobPrv = sp.sysKnob
	c, dt, _, _, _ := dynamics.SprottMorphBlend(sp.systems, sp.patch())
	i := int(sp.m) % len(sp.systems)
	ic := sp.systems[i].IC
	sp.sx, sp.sy, sp.sz = float64(ic[0]), float64(ic[1]), float64(ic[2])
	sp.ring = make([]float64, steps*3)
	ext := 0.0
	for k := range steps {
		sp.step(&c, dt)
		sp.ring[k*3] = sp.sx
		sp.ring[k*3+1] = sp.sy
		sp.ring[k*3+2] = sp.sz
		for _, v := range []float64{sp.sx, sp.sy, sp.sz} {
			ext = max(ext, v, -v)
		}
	}
	sp.head, sp.fill = 0, steps
	return max(ext, 0.5)
}

// Advance integrates the blended flow n steps of its dt scaled by scale into
// a trail of steps points, the catalog position moving on by the rate, and
// reports whether the trail has enough to draw.
func (sp *SprottMorph) Advance(n int, scale float32, steps int) bool {
	if sp.systems == nil {
		sp.systems = dynamics.SprottMorphSystems()
	}
	// The sys knob seizes the position when the user moves it; otherwise the
	// machine advances itself at the rate knob's systems-per-minute.
	if sp.knobPrv != sp.sysKnob {
		sp.knobPrv = sp.sysKnob
		sp.m = float64(sp.sysKnob)
	}
	n = max(n, 1)
	sp.m += float64(sp.rate) / 60 / 60 * float64(n) * float64(scale)
	for sp.m >= float64(len(sp.systems)) {
		sp.m -= float64(len(sp.systems))
	}
	// Stepping itself, the machine RE-PATCHES: it plays each system whole and
	// moves to the next, as the glensstuff.com computer did, rather than
	// sliding through the blends between them. Measured, almost every blend
	// is no attractor at all — most fly off, the rest settle onto a cycle or
	// a point — so a machine that slid spent most of its time reseeding. A
	// blend is still there to be dialed, by hand, with the rate at zero.
	c, dt, i, j, frac := dynamics.SprottMorphBlend(sp.systems, sp.patch())
	if len(sp.ring) != steps*3 {
		sp.ring = make([]float64, steps*3)
		sp.head, sp.fill = 0, 0
	}
	for range n {
		sp.step(&c, dt*float64(scale))
		sp.ring[sp.head*3] = sp.sx
		sp.ring[sp.head*3+1] = sp.sy
		sp.ring[sp.head*3+2] = sp.sz
		sp.head = (sp.head + 1) % steps
		if sp.fill < steps {
			sp.fill++
		}
	}
	// "D-E 42%": a dash, not an arrow, as the character display has no →
	// glyph.
	sp.label = sp.systems[i].Letter
	if frac >= 0.005 {
		sp.label += "-" + sp.systems[j].Letter + " " + strconv.Itoa(int(frac*100+0.5)) + "%"
	}
	return sp.fill >= 2
}

// Label is the PATCH readout: the system the machine is wired as, or the two
// it is between and how far.
func (sp *SprottMorph) Label() string { return sp.label }

// Trail hands put the trail's steps points, oldest first; a trail not yet
// full repeats its oldest point at the start.
func (sp *SprottMorph) Trail(steps int, put func(k int, x, y, z float64)) {
	for k := range steps {
		age := steps - 1 - k
		idx := 0
		if age < sp.fill {
			idx = (sp.head - 1 - age + steps + steps) % steps
		} else {
			idx = (sp.head - sp.fill + steps + steps) % steps
		}
		put(k, sp.ring[idx*3], sp.ring[idx*3+1], sp.ring[idx*3+2])
	}
}

// patch is the catalog position the machine is wired as: the whole system it
// has reached while it steps itself, or exactly where the sys knob puts it,
// blend and all, while the rate is zero.
func (sp *SprottMorph) patch() float64 {
	if sp.rate > 0 {
		return math.Floor(sp.m)
	}
	return sp.m
}
