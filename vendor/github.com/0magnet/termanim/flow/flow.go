// Package flow drifts thousands of particles through a noise field and lets
// their trails draw it.
//
// # WHY CURL AND NOT THE NOISE ITSELF
//
// The obvious way to make a flow field is to read a direction out of a noise
// function directly. It looks wrong within a few seconds: a field made that
// way has sources and sinks in it, so every particle eventually falls into a
// drain and the screen ends up as a handful of bright clots with nothing
// moving between them.
//
// A fluid does not do that because it cannot: what flows in must flow out. The
// cheap way to get that property exactly rather than approximately is to take
// a scalar noise field as a stream function and use its perpendicular
// gradient — velocity = (∂ψ/∂y, −∂ψ/∂x). The divergence of that is
// ∂²ψ/∂x∂y − ∂²ψ/∂y∂x, which is zero identically — and differenced on the same
// stencil it is built from, the cancellation is term by term, so the field is
// divergence free to the last bit rather than to a tolerance. Particles follow
// the contours of ψ forever, and because ψ itself drifts slowly in time they
// are carried across it instead of running the same loop.
//
// # WHAT THE EYE SEES
//
// One particle is a moving dot and says nothing. What makes the field visible
// is the fading trail each one leaves: where many particles have crossed
// recently the deposits pile up and the streamline stands out bright, and
// where none have been the trail decays back to the background. The picture is
// therefore a time exposure of the field rather than a snapshot of it, which
// is the only way a structure this smooth becomes legible at a terminal's
// resolution.
//
// The noise is a small value-noise function written here rather than pulled
// in: two octaves on an integer lattice with a quintic fade, seeded from an
// integer. Gradient noise would have fewer grid-aligned artifacts, but nothing
// on screen shows the lattice — what is drawn is where particles went, not the
// noise — and value noise is half the arithmetic.
//
// Written from the description of the technique. No code is taken from any
// existing implementation.
package flow

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Ink runs from the faintest trail worth drawing to the bright core of a
// streamline many particles are using at once.
//
// It starts dark rather than at black because the first visible step of the
// ramp is a trail that has nearly faded, and a ramp that begins at the
// background color wastes its low end on trails nobody can see anyway.
var Ink = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 12, G: 16, B: 48},
	canvas.Stop{At: 0.30, R: 32, G: 72, B: 168},
	canvas.Stop{At: 0.62, R: 64, G: 186, B: 224},
	canvas.Stop{At: 0.85, R: 176, G: 236, B: 250},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

// refArea is the surface the particle density is quoted against.
const refArea = 1000

// eps is the step the stream function is differenced over, in noise units.
//
// It has to be small enough that the difference is a derivative and large
// enough that it is not floating-point noise. A hundredth of a lattice cell
// is four orders of magnitude above the rounding error of a float64 and four
// orders below the scale the field varies on.
const eps = 0.01

// Flow is the animation. The zero value is not usable; call New.
type Flow struct {
	w, h int
	t    float64

	parts []particle

	// trail is the deposit at each pixel, decayed every frame. Kept as its own
	// buffer rather than read back off the surface because the surface stores
	// colors, and recovering an intensity from a palette entry is not possible
	// once two ramp positions round to the same color.
	trail []float32

	rng  *rand.Rand
	seed uint32

	// Density is particles per thousand pixels. The count has to follow the
	// area: a number that fills a small window is a scattering of dots in a
	// large one, and the trails only add up into streamlines when enough
	// particles share a path.
	Density float64

	// Speed is how fast a particle is carried, in pixels per second at full
	// field strength. Fast enough and the trails smear into a wash; slow
	// enough and the field drifts out from under them before they draw it.
	Speed float64

	// Scale is roughly how many noise cells span the shorter side of the
	// surface. Larger is a finer, more turbulent field with smaller eddies.
	Scale float64

	// Drift is how fast the field itself changes, in noise units per second.
	// At zero the streamlines are fixed and every particle ends up on the one
	// closed contour it started nearest to.
	Drift float64

	// Deposit is how much a particle adds to the pixel it is on, per second.
	// Per second rather than per frame: a per-frame deposit would draw twice as
	// dark on a machine running twice as fast, which is the same bug as motion
	// that follows the frame count.
	Deposit float64

	// TrailLife is the time constant of the fade, in seconds: how long a
	// deposit takes to fall to about a third of its strength. This is the
	// exposure time of the picture, and it is the knob that matters most —
	// short is a swarm of comets, long is a smoke drawing that keeps every
	// stroke.
	TrailLife float64

	// MaxAge is how long a particle lives before it is moved somewhere new, in
	// seconds. Even a divergence-free field has regions nothing enters, and
	// without recycling those stay bare forever while the particles that
	// started in a fast eddy retrace it all afternoon.
	MaxAge float64

	// Floor is the faintest trail that is drawn at all, from 0 to 1. Below it
	// a pixel is left as the terminal's own background, which is what keeps
	// the field looking like strokes on a dark ground rather than a dim wash
	// over the whole window.
	Floor float64

	// Palette colors a pixel by how much has been deposited on it.
	Palette canvas.Palette
}

type particle struct {
	x, y float64
	age  float64
}

// New returns a flow field. seed of 0 gives a fixed field and a fixed set of
// starting positions, which makes tests repeatable; anything else is a
// different field entirely, not merely a different arrangement in the same one.
func New(seed int64) *Flow {
	return &Flow{
		rng: rand.New(rand.NewSource(seed)), //nolint:gosec
		// Folded to 32 bits because the noise hash is integer arithmetic. The
		// multiply is there so that seeds 0 and 1 — the ones tests and demos
		// actually use — do not differ only in their lowest bit, which a hash
		// this small would carry through to visibly similar fields.
		seed:      uint32(uint64(seed) * 2654435761), //nolint:gosec
		Density:   150,
		Speed:     34,
		Scale:     2.4,
		Drift:     0.08,
		Deposit:   2.2,
		TrailLife: 2.2,
		MaxAge:    12,
		Floor:     0.02,
		Palette:   Ink,
	}
}

// Resize allocates the particles and the trail buffer, and scatters the
// particles over the new surface.
func (f *Flow) Resize(w, h int) {
	f.w, f.h = w, h
	f.trail = make([]float32, w*h)

	n := int(f.Density * float64(w*h) / refArea)
	if n < 1 {
		n = 1
	}
	f.parts = make([]particle, n)
	for i := range f.parts {
		f.parts[i] = f.spawn()
	}
}

// spawn puts a particle somewhere random with a random age.
//
// The age is random rather than zero so that the particles do not all reach
// MaxAge together: recycling the entire population in one frame is visible as
// a pulse, where a steady trickle is not.
func (f *Flow) spawn() particle {
	return particle{
		x:   f.rng.Float64() * float64(f.w),
		y:   f.rng.Float64() * float64(f.h),
		age: f.rng.Float64() * f.MaxAge,
	}
}

// Frame advances the field, moves every particle, fades the trails and draws
// what is left.
func (f *Flow) Frame(s *canvas.Surface, dt float64) {
	if f.w == 0 || f.h == 0 {
		return
	}
	f.t += dt * f.Drift

	// Exponential decay, so the fade is the same per second whatever the
	// frame rate: a per-frame multiplier would make trails linger twice as
	// long on a machine drawing twice as often.
	life := f.TrailLife
	if life <= 0 {
		life = 1e-6
	}
	keep := float32(math.Exp(-dt / life))
	for i := range f.trail {
		f.trail[i] *= keep
	}

	fw, fh := float64(f.w), float64(f.h)
	speed := f.Speed * dt
	dep := float32(f.Deposit * dt)
	for i := range f.parts {
		p := &f.parts[i]
		vx, vy := f.velocity(p.x, p.y)
		p.x += vx * speed
		p.y += vy * speed

		// Wrapped rather than reflected. A particle that bounced would leave a
		// trail doubling back on itself, and the field it is following does
		// not turn around at the edge — the wrap is a lie about the noise, but
		// it is a lie about a region the eye cannot compare against anything.
		p.x = wrap(p.x, fw)
		p.y = wrap(p.y, fh)

		p.age += dt
		if p.age > f.MaxAge {
			*p = f.spawn()
			p.age = 0
		}

		// Deposit. Clamped at one so a streamline everything is using does not
		// keep climbing past the top of the ramp and take a full TrailLife to
		// come back down once the flow moves off it.
		j := int(p.y)*f.w + int(p.x)
		if j >= 0 && j < len(f.trail) {
			if f.trail[j] += dep; f.trail[j] > 1 {
				f.trail[j] = 1
			}
		}
	}

	floor := float32(f.Floor)
	for y := 0; y < f.h; y++ {
		row := y * f.w
		for x := 0; x < f.w; x++ {
			v := f.trail[row+x]
			if v <= floor {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			i := int(v * 255)
			if i > 255 {
				i = 255
			}
			s.Set(x, y, f.Palette[i])
		}
	}
}

// wrap folds a coordinate back onto the surface.
func wrap(v, span float64) float64 {
	if span <= 0 {
		return 0
	}
	for v < 0 {
		v += span
	}
	for v >= span {
		v -= span
	}
	return v
}

// velocity is the flow at a point in pixel coordinates, as a unit-ish vector.
//
// It is the perpendicular gradient of the stream function, by central
// differences. Taking BOTH derivatives this way is what makes the discrete
// divergence identically zero and not merely small: the four cross terms that
// a numerical divergence of this vector field expands into cancel exactly,
// whatever the noise underneath is.
func (f *Flow) velocity(x, y float64) (vx, vy float64) {
	s := f.Scale / float64(min(f.w, f.h))
	nx, ny := x*s, y*s
	dpdx := (f.potential(nx+eps, ny) - f.potential(nx-eps, ny)) / (2 * eps)
	dpdy := (f.potential(nx, ny+eps) - f.potential(nx, ny-eps)) / (2 * eps)
	return dpdy, -dpdx
}

// potential is the stream function: two octaves of value noise, drifting.
//
// Two rather than one because a single octave is all eddies of the same size,
// which reads as a repeating pattern however unrepeating it actually is. The
// second is at an irrational-ish multiple of the first so their lattices never
// line up, and offset so the two do not share their zero crossings.
func (f *Flow) potential(x, y float64) float64 {
	return (f.noise(x, y, f.t) + 0.5*f.noise(x*2.17+11.3, y*2.17-7.1, f.t*1.9)) / 1.5
}

// noise is value noise on the integer lattice: a hashed value at each corner
// of the cell a point falls in, blended with a quintic fade.
//
// The fade is quintic rather than the cheaper cubic smoothstep because this
// field is differentiated. Cubic smoothstep is continuous in its first
// derivative but not its second, so a velocity built from it has a visible
// crease at every lattice line; the quintic's second derivative vanishes at
// both ends and the creases go away.
func (f *Flow) noise(x, y, z float64) float64 {
	xi, yi, zi := math.Floor(x), math.Floor(y), math.Floor(z)
	xf, yf, zf := x-xi, y-yi, z-zi
	u, v, w := fade(xf), fade(yf), fade(zf)

	ix, iy, iz := int32(xi), int32(yi), int32(zi)
	c000 := f.lattice(ix, iy, iz)
	c100 := f.lattice(ix+1, iy, iz)
	c010 := f.lattice(ix, iy+1, iz)
	c110 := f.lattice(ix+1, iy+1, iz)
	c001 := f.lattice(ix, iy, iz+1)
	c101 := f.lattice(ix+1, iy, iz+1)
	c011 := f.lattice(ix, iy+1, iz+1)
	c111 := f.lattice(ix+1, iy+1, iz+1)

	x00 := c000 + (c100-c000)*u
	x10 := c010 + (c110-c010)*u
	x01 := c001 + (c101-c001)*u
	x11 := c011 + (c111-c011)*u
	y0 := x00 + (x10-x00)*v
	y1 := x01 + (x11-x01)*v
	return y0 + (y1-y0)*w
}

// fade is the quintic 6t⁵-15t⁴+10t³, flat in both its first and second
// derivatives at 0 and 1.
func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

// lattice is the value at one integer lattice point, in -1..1.
//
// An integer hash rather than a table of permutations: a table would be state
// to build and carry, and this is three multiplies and three xor-shifts. The
// constants are the usual odd multipliers chosen so that every input bit
// reaches the top of the word.
func (f *Flow) lattice(x, y, z int32) float64 {
	h := uint32(x)*0x9E3779B1 ^ uint32(y)*0x85EBCA77 ^ uint32(z)*0xC2B2AE3D ^ f.seed //nolint:gosec // a hash: the wraparound is the mixing, not an overflow
	h ^= h >> 15
	h *= 0x2C1B3C6D
	h ^= h >> 13
	h *= 0x297A2D39
	h ^= h >> 16
	return float64(h)/float64(1<<31) - 1
}

// Run draws a flow field on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
