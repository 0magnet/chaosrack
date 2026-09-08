// Package ripple is the fluid surface the water lens looks through.
//
// It is a height field on a grid, stepped by the damped wave equation. No
// GL, no syscall/js, no build tag: the physics is testable on a machine, and
// the browser layer above it only uploads the field as a texture and samples
// through its gradient.
//
// # WHY A HEIGHT FIELD AND NOT A FLUID
//
// What a water surface does to an image is refract it, and refraction depends
// on the surface NORMAL — which on a height field is the gradient. Simulating
// actual fluid (velocity, pressure, advection) would produce a better splash
// and exactly the same refraction, because the image never sees the velocity.
// The wave equation is two multiply-adds per cell and the eye cannot tell.
//
// What the eye CAN tell is the four things below, which is why they are
// controls rather than constants: how fast waves travel, how long they last,
// how sharp they stay, and what the edges do to them. Those are what make one
// medium read as water, another as oil, another as a drum head — and what
// separates a tank with walls from open water.
package ripple

import "math"

// Field is a square height field and its previous state. Two buffers, because
// the wave equation is second order in time: the next height at a cell needs
// both the current one and the one before it.
type Field struct {
	W, H int
	cur  []float32
	prev []float32
	next []float32

	// Speed is how far a wave travels per step, as a fraction of a cell. The
	// wave equation is only stable while c*c <= 0.5 on this stencil — above
	// that the field does not "look wrong", it explodes to NaN in a few
	// hundred steps and the screen goes black. Step clamps it, so the knob
	// cannot be turned into a crash.
	Speed float32

	// Damping is the fraction of amplitude kept per step. 1 is a medium that
	// rings forever and fills with standing waves until nothing is legible;
	// below about 0.9 a drop dies before it crosses the tank. The useful
	// range is narrow and near the top, which is why the knob is exponential.
	Damping float32

	// Reflect is what the walls do, 0..1. At 1 the boundary is a wall and the
	// tank rings with its own echoes; at 0 it is open water and a wave leaves
	// for good. In between, some of each wave comes back. See applyEdges — the
	// two ends are different media, not two settings of one, which is why this
	// is a knob.
	Reflect float32

	// Spread is a viscous smoothing applied after each step, 0..1. It is not
	// in the wave equation: it is what separates water from a drum head. A
	// drum head keeps every ripple it is given, however fine; a liquid loses
	// the finest ones to viscosity within a wavelength or two, which is why
	// real water looks smooth between the waves you meant to make.
	Spread float32
}

// maxSpeed is the CFL limit for this five-point stencil. Past it the
// integration is unconditionally unstable.
const maxSpeed = 0.7071 // sqrt(0.5)

// New returns a still field of w by h cells.
func New(w, h int) *Field {
	if w < 4 {
		w = 4
	}
	if h < 4 {
		h = 4
	}
	n := w * h
	return &Field{
		W: w, H: h,
		cur:     make([]float32, n),
		prev:    make([]float32, n),
		next:    make([]float32, n),
		Speed:   0.5,
		Reflect: 1,
		Damping: 0.985,
		Spread:  0.12,
	}
}

// Height reads one cell, clamped to the field so a caller sampling a
// neighborhood at the edge does not have to special-case it.
func (f *Field) Height(x, y int) float32 {
	if x < 0 {
		x = 0
	} else if x >= f.W {
		x = f.W - 1
	}
	if y < 0 {
		y = 0
	} else if y >= f.H {
		y = f.H - 1
	}
	return f.cur[y*f.W+x]
}

// Heights is the current field, row-major, for uploading as a texture.
func (f *Field) Heights() []float32 { return f.cur }

// Drop adds a raised (or lowered) region centered on x,y with the given radius
// — a finger in the water, a drip, one cycle of a speaker cone.
//
// The profile is a raised cosine rather than a spike. A single-cell impulse
// contains every wavelength the grid can represent, including the ones at the
// stencil's resolution limit, and those do not propagate — they sit and
// shimmer. A smooth bump of a few cells radiates a ring, which is what a
// disturbance in water actually does.
func (f *Field) Drop(x, y, radius, amp float32) {
	if radius < 1 {
		radius = 1
	}
	r := int(radius) + 1
	cx, cy := int(x), int(y)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			px, py := cx+dx, cy+dy
			if px < 0 || py < 0 || px >= f.W || py >= f.H {
				continue
			}
			d := float32(math.Hypot(float64(dx), float64(dy)))
			if d > radius {
				continue
			}
			// cos ramp: full amplitude at the center, zero and flat at the rim.
			w := 0.5 * (1 + float32(math.Cos(float64(d/radius)*math.Pi)))
			f.cur[py*f.W+px] += amp * w
		}
	}
}

// Line adds a disturbance along the segment from one point to another, which
// is what a finger DRAGGED through water leaves — a wake, not a series of
// dots. Called with the pointer's previous and current position, it stays
// continuous however fast the pointer moves; dropping at the current position
// alone leaves gaps at speed, and the gaps read as a dotted line.
func (f *Field) Line(x0, y0, x1, y1, radius, amp float32) {
	dx, dy := float64(x1-x0), float64(y1-y0)
	steps := int(math.Hypot(dx, dy)/float64(radius)*2) + 1
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		f.Drop(x0+(x1-x0)*t, y0+(y1-y0)*t, radius, amp/float32(steps+1)*2)
	}
}

// Step advances the field by one tick.
func (f *Field) Step() {
	c := f.Speed
	if c > maxSpeed {
		c = maxSpeed // see maxSpeed: past this it is not ugly, it is NaN
	}
	if c < 0 {
		c = 0
	}
	c2 := c * c
	damp := f.Damping
	if damp > 1 {
		damp = 1
	}
	if damp < 0 {
		damp = 0
	}

	w, h := f.W, f.H
	for y := 1; y < h-1; y++ {
		row := y * w
		for x := 1; x < w-1; x++ {
			i := row + x
			// Five-point Laplacian, and the second-order time step that makes
			// it a wave rather than a diffusion.
			lap := f.cur[i-1] + f.cur[i+1] + f.cur[i-w] + f.cur[i+w] - 4*f.cur[i]
			f.next[i] = (2*f.cur[i] - f.prev[i] + c2*lap) * damp
		}
	}
	f.applyEdges(c)
	f.prev, f.cur, f.next = f.cur, f.next, f.prev

	if f.Spread > 0 {
		f.smooth()
	}
}

// applyEdges sets the boundary cells according to Reflect.
//
// The two ends are different media, not two settings of one:
//
//   - A WALL (Reflect 1) copies the inward neighbor outward. A wave arrives,
//     turns around and comes back, and the tank fills with the interference
//     between what was sent and what returned. This is a ripple tank, and it
//     is what makes a small surface interesting.
//
//   - OPEN WATER (Reflect 0) lets the wave leave and never come back, using
//     the first-order Mur condition below. The surface then shows only what is
//     being made right now, which is what you want when the disturbance is the
//     signal — audio driving a speaker in the tank, say, where returning echoes
//     are the previous second's music smeared over this one.
//
// Neither is "correct", which is why it is a knob and not a constant. In
// between, a fraction of each wave returns: a tank with soft edges, which is
// most real water.
//
// The Mur condition is the standard one for this stencil: at the boundary,
//
//	u[edge]^{n+1} = u[in]^n + (c-1)/(c+1) · (u[in]^{n+1} − u[edge]^n)
//
// It is exact for a wave arriving square-on and leaks a little for one
// arriving at an angle, which is the usual trade and invisible here.
func (f *Field) applyEdges(c float32) {
	r := f.Reflect
	if r > 1 {
		r = 1
	} else if r < 0 {
		r = 0
	}
	k := (c - 1) / (c + 1)

	// edge returns the blend of wall and open water for one boundary cell,
	// given its inward neighbor.
	edge := func(edgeIdx, inIdx int) float32 {
		wall := f.next[inIdx]
		open := f.cur[inIdx] + k*(f.next[inIdx]-f.cur[edgeIdx])
		return open + (wall-open)*r
	}

	w, h := f.W, f.H
	for x := 0; x < w; x++ {
		top, topIn := x, w+x
		bot, botIn := (h-1)*w+x, (h-2)*w+x
		f.next[top] = edge(top, topIn)
		f.next[bot] = edge(bot, botIn)
	}
	for y := 0; y < h; y++ {
		left, leftIn := y*w, y*w+1
		right, rightIn := y*w+w-1, y*w+w-2
		f.next[left] = edge(left, leftIn)
		f.next[right] = edge(right, rightIn)
	}
}

// smooth is the viscous term: a small blend toward the neighborhood mean,
// applied after the wave step so it damps the shortest wavelengths hardest.
func (f *Field) smooth() {
	s := f.Spread
	if s > 1 {
		s = 1
	}
	w, h := f.W, f.H
	copy(f.next, f.cur)
	for y := 1; y < h-1; y++ {
		row := y * w
		for x := 1; x < w-1; x++ {
			i := row + x
			mean := (f.cur[i-1] + f.cur[i+1] + f.cur[i-w] + f.cur[i+w]) * 0.25
			f.next[i] = f.cur[i] + (mean-f.cur[i])*s
		}
	}
	f.cur, f.next = f.next, f.cur
}

// Gradient is the surface slope at a cell — the thing refraction actually
// depends on. Central differences, so the sample is centered on the cell
// rather than half a cell off in each direction as a forward difference is.
func (f *Field) Gradient(x, y int) (gx, gy float32) {
	return (f.Height(x+1, y) - f.Height(x-1, y)) * 0.5,
		(f.Height(x, y+1) - f.Height(x, y-1)) * 0.5
}

// Energy is the summed square of the field, which is what a stability check
// wants: a field that is going to explode does so here first, and long before
// anything is visible on screen.
func (f *Field) Energy() float64 {
	var e float64
	for _, v := range f.cur {
		e += float64(v) * float64(v)
	}
	return e
}

// Still empties the field. Used when the medium's parameters change enough
// that the waves in flight were computed under different physics.
func (f *Field) Still() {
	for i := range f.cur {
		f.cur[i], f.prev[i], f.next[i] = 0, 0, 0
	}
}
