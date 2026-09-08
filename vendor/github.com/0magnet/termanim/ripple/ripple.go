// Package ripple is a ripple tank: drops fall on still water and the rings
// they leave run into one another.
//
// The interference is the whole point. One drop on its own is a target
// pattern, which is pretty for a second and then boring; several drops at
// once produce the lattice of crests and dead spots that a real tank shows,
// and — with the walls turned up — the tank keeps ringing with its own
// reflections long after the drop that started it has died. What the eye
// reads is not the rings but the moving pattern where they cross.
//
// # WHERE THE PHYSICS CAME FROM
//
// The height field, the damped wave equation, the CFL speed clamp, the
// viscous Spread term and the Reflect knob that blends hard walls against Mur
// absorbing boundaries are ported from pkg/ripple in
// github.com/0magnet/chaosrack, by the same author, where they drive the water
// lens a browser looks through. Adapted rather than imported: that package
// exists to be sampled through its gradient by a shader, this one exists to be
// colored by its height in a terminal, and an animation in this repository
// should not drag in a module for three hundred lines of arithmetic.
//
// # WHY A HEIGHT FIELD AND NOT A FLUID
//
// A real fluid solver carries velocity, pressure and advection, and would
// produce a better splash. It would not produce better rings: what makes
// water read as water here is that a disturbance travels outward at a fixed
// speed, decays, and adds to whatever it meets, and that is exactly the wave
// equation — two multiply-adds per cell. The eye cannot tell, and the cost
// difference is what lets this run full-screen.
package ripple

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Water runs from the dark of a trough through the flat of still water to a
// lit crest.
//
// It is deliberately not symmetric about the middle: a real surface is lit
// from above, so a crest catches the light and a trough only shades. Half the
// ramp climbing to white and the other half falling to near-black would make
// the field read as a plasma rather than as water.
var Water = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 0, G: 6, B: 32},
	canvas.Stop{At: 0.40, R: 0, G: 48, B: 104},
	canvas.Stop{At: 0.50, R: 0, G: 78, B: 132},
	canvas.Stop{At: 0.62, R: 32, G: 150, B: 196},
	canvas.Stop{At: 0.82, R: 150, G: 224, B: 250},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

// maxSpeed is the CFL limit for the five-point stencil below. Past it the
// integration is not merely inaccurate, it is unconditionally unstable: the
// field does not look wrong, it goes to NaN within a few hundred steps and the
// screen goes black. step clamps to it, so no setting of Speed can crash the
// animation.
const maxSpeed = 0.7071 // sqrt(0.5)

// defaultStepRate is simulation steps per second. The medium's constants are
// per step, not per second, so this is what fixes how fast a ring actually
// crosses the tank; sixty is a wave front moving about a pixel every two
// steps, which is a ring that takes a couple of seconds to reach the wall.
const defaultStepRate = 60

// maxCatchUp bounds how many steps one frame may run. canvas already clamps
// the elapsed time it hands over, but a rule that costs a full sweep of the
// grid should never run an unbounded number of times because the process was
// descheduled.
const maxCatchUp = 6

// refArea is the surface the drop rate is quoted against — a middling
// terminal. A fixed number of drops per second is a downpour in a small
// window and a lonely plink in a large one, so the rate follows the area.
const refArea = 160 * 90

// Ripple is the animation. The zero value is not usable; call New.
type Ripple struct {
	w, h int

	// Three buffers, because the wave equation is second order in time: the
	// next height at a cell needs both the current height and the one before
	// it. float32 rather than float64 because a step walks all three of these
	// end to end and the cost at full screen is memory traffic, not
	// arithmetic; the extra digits would never reach a color index.
	cur, prev, next []float32

	rng *rand.Rand

	// acc carries the fraction of a simulation step left over from the last
	// frame, and drops the fraction of a drop. Both are discrete events that
	// cannot be run by a fractional amount, so elapsed time accumulates and
	// whole ones are taken out of it.
	acc, drops float64

	// Speed is how far a wave travels per step, as a fraction of a cell.
	// Faster rings cross the tank sooner and interfere more often. Clamped to
	// the CFL limit, so this knob cannot be turned into a crash.
	Speed float64

	// Damping is the fraction of amplitude kept per step. 1 is a medium that
	// rings forever and silts up with standing waves until no individual drop
	// can be seen; below about 0.9 a drop dies before its ring crosses the
	// tank. The useful range is narrow and near the top.
	Damping float64

	// Reflect is what the walls do, 0..1. At 1 the boundary is a wall and the
	// tank rings with its own echoes, which is what makes a small surface
	// interesting; at 0 it is open water and a wave leaves for good, so the
	// screen shows only the drops that fell recently. In between, some of
	// each ring comes back. See applyEdges: the two ends are different media,
	// not two settings of one.
	Reflect float64

	// Spread is a viscous smoothing applied after each step, 0..1. It is not
	// part of the wave equation: it is what separates water from a drum head.
	// A drum head keeps every ripple it is given however fine, while a liquid
	// loses the shortest wavelengths within a wavelength or two, which is why
	// real water looks glassy between the waves you meant to make.
	Spread float64

	// StepRate is simulation steps per second. Raising it speeds every wave
	// up together, which is a different thing from raising Speed: Speed also
	// changes the wavelength a drop leaves behind.
	StepRate float64

	// DropRate is drops per second on a middling terminal; the actual rate
	// scales with the area so a big window is not left nearly empty. Turn it
	// down for a few clean expanding rings, up for chop.
	DropRate float64

	// DropRadius is how wide a drop lands, in pixels. A single-cell impulse
	// contains every wavelength the grid can represent including the ones at
	// its resolution limit, and those do not travel — they sit and shimmer. A
	// few cells across is what radiates a ring.
	DropRadius float64

	// DropDepth is how hard a drop hits. It dents the surface rather than
	// raising it, which is what a falling drop does; the crest comes back up
	// on its own a few steps later.
	DropDepth float64

	// Gain scales height to color. The rings a drop leaves are a fraction of
	// the depth of the dent that made them, so drawing the raw height would
	// show the splash and nothing else.
	Gain float64

	// Palette colors the surface by height, trough to crest.
	Palette canvas.Palette
}

// New returns a ripple tank. seed of 0 gives a fixed sequence of drops, which
// makes tests repeatable; anything else varies where the rain falls.
func New(seed int64) *Ripple {
	return &Ripple{
		rng:        rand.New(rand.NewSource(seed)), //nolint:gosec
		Speed:      0.5,
		Damping:    0.995,
		Reflect:    1,
		Spread:     0.06,
		StepRate:   defaultStepRate,
		DropRate:   2.5,
		DropRadius: 3,
		DropDepth:  1,
		Gain:       3.5,
		Palette:    Water,
	}
}

// Resize allocates the field. Called before the first frame and on every
// resize; the water starts still, so a resize is a fresh tank rather than the
// old one stretched.
func (r *Ripple) Resize(w, h int) {
	r.w, r.h = w, h
	n := w * h
	r.cur = make([]float32, n)
	r.prev = make([]float32, n)
	r.next = make([]float32, n)
	r.acc, r.drops = 0, 0
}

// Frame rains on the tank, steps the water and draws it.
func (r *Ripple) Frame(s *canvas.Surface, dt float64) {
	if r.w < 4 || r.h < 4 {
		return
	}
	rate := r.StepRate
	if rate <= 0 {
		rate = defaultStepRate
	}
	r.acc += dt * rate
	if r.acc > maxCatchUp {
		r.acc = maxCatchUp
	}
	// Rain is dropped inside the step loop rather than once per frame, so
	// that everything the animation does is driven by whole simulation steps
	// and two runs covering the same wall-clock time land in exactly the same
	// state whatever frame rate divided it up.
	for r.acc >= 1 {
		r.rain(1 / rate)
		r.step()
		r.acc--
	}
	r.draw(s)
}

// rain adds however many drops fell in a step's worth of time.
func (r *Ripple) rain(dt float64) {
	area := float64(r.w * r.h)
	r.drops += dt * r.DropRate * area / refArea
	// A tank that was paused should not be hit by every drop it missed at
	// once; one waiting drop is enough of a backlog.
	if r.drops > 1 {
		r.drops = 1
	}
	for r.drops >= 1 {
		r.drops--
		// Radius varies a little, so the rings are not all the same
		// wavelength and the interference has something to work with.
		rad := r.DropRadius * (0.7 + r.rng.Float64()*0.6)
		r.drop(
			r.rng.Float64()*float64(r.w),
			r.rng.Float64()*float64(r.h),
			rad, -r.DropDepth)
	}
}

// drop dents (or raises) a region centered on x,y.
//
// The profile is a raised cosine rather than a spike, for the reason given on
// DropRadius: a spike is mostly energy at wavelengths the grid cannot carry.
func (r *Ripple) drop(x, y, radius, amp float64) {
	if radius < 1 {
		radius = 1
	}
	n := int(radius) + 1
	cx, cy := int(x), int(y)
	for dy := -n; dy <= n; dy++ {
		py := cy + dy
		if py < 0 || py >= r.h {
			continue
		}
		for dx := -n; dx <= n; dx++ {
			px := cx + dx
			if px < 0 || px >= r.w {
				continue
			}
			d := math.Hypot(float64(dx), float64(dy))
			if d > radius {
				continue
			}
			// Full amplitude at the center, zero and flat at the rim.
			w := 0.5 * (1 + math.Cos(d/radius*math.Pi))
			r.cur[py*r.w+px] += float32(amp * w)
		}
	}
}

// step advances the field by one tick of the wave equation.
func (r *Ripple) step() {
	c := r.Speed
	if c > maxSpeed {
		c = maxSpeed // see maxSpeed: past this it is not ugly, it is NaN
	}
	if c < 0 {
		c = 0
	}
	c2 := float32(c * c)
	damp := r.Damping
	if damp > 1 {
		damp = 1
	} else if damp < 0 {
		damp = 0
	}
	d32 := float32(damp)

	w, h := r.w, r.h
	for y := 1; y < h-1; y++ {
		row := y * w
		for x := 1; x < w-1; x++ {
			i := row + x
			// Five-point Laplacian, and the second-order time step that makes
			// it a wave rather than a diffusion. Drop the prev term and the
			// same stencil spreads a drop into a smear that never travels.
			lap := r.cur[i-1] + r.cur[i+1] + r.cur[i-w] + r.cur[i+w] - 4*r.cur[i]
			r.next[i] = (2*r.cur[i] - r.prev[i] + c2*lap) * d32
		}
	}
	r.applyEdges(float32(c))
	r.prev, r.cur, r.next = r.cur, r.next, r.prev

	if r.Spread > 0 {
		r.smooth()
	}
}

// applyEdges sets the boundary cells according to Reflect.
//
// The two ends are different media, not two settings of one:
//
//   - A WALL (Reflect 1) copies the inward neighbor outward. A ring arrives,
//     turns around and comes back, and the tank fills with the interference
//     between what was sent and what returned.
//
//   - OPEN WATER (Reflect 0) lets the ring leave and never come back, using
//     the first-order Mur condition below. The surface then shows only what
//     fell recently, which is the quieter and more legible of the two.
//
// Neither is more correct than the other, which is why it is a knob. The Mur
// condition is the standard one for this stencil: at the boundary,
//
//	u[edge]^{n+1} = u[in]^n + (c-1)/(c+1) · (u[in]^{n+1} − u[edge]^n)
//
// It is exact for a wave arriving square-on and leaks a little for one
// arriving at an angle, which is the usual trade and invisible here.
func (r *Ripple) applyEdges(c float32) {
	ref := float32(r.Reflect)
	if ref > 1 {
		ref = 1
	} else if ref < 0 {
		ref = 0
	}
	k := (c - 1) / (c + 1)

	// edge blends wall against open water for one boundary cell, given its
	// inward neighbor.
	edge := func(edgeIdx, inIdx int) float32 {
		wall := r.next[inIdx]
		open := r.cur[inIdx] + k*(r.next[inIdx]-r.cur[edgeIdx])
		return open + (wall-open)*ref
	}

	w, h := r.w, r.h
	for x := 0; x < w; x++ {
		top, topIn := x, w+x
		bot, botIn := (h-1)*w+x, (h-2)*w+x
		r.next[top] = edge(top, topIn)
		r.next[bot] = edge(bot, botIn)
	}
	for y := 0; y < h; y++ {
		left, leftIn := y*w, y*w+1
		right, rightIn := y*w+w-1, y*w+w-2
		r.next[left] = edge(left, leftIn)
		r.next[right] = edge(right, rightIn)
	}
}

// smooth is the viscous term: a small blend toward the neighborhood mean,
// applied after the wave step so it damps the shortest wavelengths hardest.
func (r *Ripple) smooth() {
	s := float32(r.Spread)
	if s > 1 {
		s = 1
	}
	w, h := r.w, r.h
	copy(r.next, r.cur)
	for y := 1; y < h-1; y++ {
		row := y * w
		for x := 1; x < w-1; x++ {
			i := row + x
			mean := (r.cur[i-1] + r.cur[i+1] + r.cur[i-w] + r.cur[i+w]) * 0.25
			r.next[i] = r.cur[i] + (mean-r.cur[i])*s
		}
	}
	r.cur, r.next = r.next, r.cur
}

// draw colors the surface by height. Still water is the middle of the ramp,
// so a tank at rest is a flat expanse rather than black.
func (r *Ripple) draw(s *canvas.Surface) {
	gain := r.Gain * 127
	for y := 0; y < r.h; y++ {
		row := y * r.w
		for x := 0; x < r.w; x++ {
			i := int(128 + float64(r.cur[row+x])*gain)
			if i < 0 {
				i = 0
			} else if i > 255 {
				i = 255
			}
			s.Set(x, y, r.Palette[i])
		}
	}
}

// height reads one cell, clamped to the field, so a caller sampling a
// neighborhood at the edge does not have to special-case it.
func (r *Ripple) height(x, y int) float32 {
	if x < 0 {
		x = 0
	} else if x >= r.w {
		x = r.w - 1
	}
	if y < 0 {
		y = 0
	} else if y >= r.h {
		y = r.h - 1
	}
	return r.cur[y*r.w+x]
}

// energy is the summed square of the field. A field about to go unstable does
// so here first, long before anything shows on screen.
func (r *Ripple) energy() float64 {
	var e float64
	for _, v := range r.cur {
		e += float64(v) * float64(v)
	}
	return e
}

// Run draws a ripple tank on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
