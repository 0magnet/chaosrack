// Package boids is Reynolds' flocking.
//
// Every boid steers by three local rules, evaluated over the neighbours inside
// a perception radius and nothing else. Separation turns it away from any
// neighbour that has come too close, alignment turns it toward the average
// heading of its neighbours, and cohesion turns it toward their average
// position. No boid knows about the flock; the flock is what the three rules
// add up to. Speed is clamped to a band so the flock neither freezes into a
// lattice nor accelerates off the screen, and the edges wrap, so a flock that
// leaves one side arrives at the other still in formation.
//
// Written from that description. Craig Reynolds' 1987 paper is the source of
// the technique; no implementation of it was consulted or transliterated.
package boids

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

type boid struct {
	x, y   float64 // position in pixels
	vx, vy float64 // velocity in pixels per second
}

// Boids is the animation. The zero value is not usable; call New.
type Boids struct {
	w, h  float64
	flock []boid

	// Accelerations for one frame, held here rather than declared in Frame
	// because every boid has to steer by the *current* velocities of its
	// neighbours: integrating boid 0 before boid 1 has looked at it would let
	// the update order leak into the flock's shape.
	ax, ay []float64

	// A uniform grid over the surface, one cell to a perception radius, so a
	// boid only tests the nine cells that can hold a neighbour. The naive form
	// of this is O(n²) — every boid against every other — which is fine for a
	// few dozen and quadratically less fine for a few hundred. heads holds the
	// first boid in each cell, next the rest of that cell's chain; both are
	// sized once here and refilled in place each frame.
	gw, gh int
	heads  []int
	next   []int

	// Derived from the surface in Resize. The units are worth stating, because
	// only some of these are rates and only rates get scaled by the elapsed
	// time: radius and sepDist are lengths and are the same at any frame rate,
	// as are the three rule weights below.
	radius   float64 // perception radius, in pixels
	sepDist  float64 // in pixels: below this a neighbour is crowding us
	minSpeed float64 // pixels per second
	maxSpeed float64 // pixels per second
	maxForce float64 // pixels per second per second

	rng *rand.Rand

	// Count is how many boids to fly. Zero, the default, picks a number from
	// the area of the surface so a large window gets a large flock and a small
	// one does not turn into a solid block of pixels.
	Count int
	// Separation, Alignment and Cohesion weight the three rules. They are
	// applied to unit vectors, so only their ratios matter.
	//
	// Separation is the largest because it is the only rule that acts against
	// collision, it acts over a much shorter range than the other two, and a
	// flock that cannot push itself apart collapses to a single point and stops
	// looking like anything. Cohesion is the smallest because it is attractive
	// everywhere in the neighbourhood and never satisfied: given a weight near
	// separation's it wins the argument at every distance and the flock clumps.
	// Alignment sits between them — it is what turns a crowd into a flock, but
	// on its own it only makes the boids parallel, not together.
	Separation, Alignment, Cohesion float64
	// Wander is a small random steering, as a fraction of the maximum steering
	// force. Three rules and nothing else settle into a perfectly parallel
	// flock that flies in a straight line forever; a little noise keeps it
	// turning, splitting and rejoining. It is added to the steering vector and
	// so passes through the same clamp, and is scaled by the elapsed time along
	// with the rest of the acceleration.
	Wander float64
	// Tail is how many pixels of trail to draw behind each boid. Zero picks a
	// length from the surface size. A single pixel shows where a boid is but
	// not where it is going, and heading is the whole point of the effect.
	Tail int
	// Palette colours each boid by its heading.
	Palette canvas.Palette
}

// heading is the default ramp: a hue wheel, red through yellow, green, cyan and
// violet and back to red.
//
// Two things are required of it and neither is decorative. It has to close on
// the colour it opened with, because heading is an angle, and a ramp that does
// not close puts a hard colour seam across the flock at due west where the
// angle wraps. And it has to stay bright the whole way round: the ramps in
// canvas are intensity ramps that run from black upward, and a boid flying in
// whichever direction landed on the dark end of one would simply not be drawn.
var heading = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 255, G: 72, B: 72},
	canvas.Stop{At: 0.17, R: 255, G: 208, B: 64},
	canvas.Stop{At: 0.33, R: 104, G: 255, B: 96},
	canvas.Stop{At: 0.50, R: 64, G: 240, B: 240},
	canvas.Stop{At: 0.67, R: 112, G: 144, B: 255},
	canvas.Stop{At: 0.83, R: 240, G: 104, B: 255},
	canvas.Stop{At: 1.00, R: 255, G: 72, B: 72},
)

// New returns a flocking animation. seed of 0 gives a fixed arrangement, which
// makes tests repeatable.
func New(seed int64) *Boids {
	return &Boids{
		rng:        rand.New(rand.NewSource(seed)),
		Separation: 1.6,
		Alignment:  0.7,
		Cohesion:   0.35,
		Wander:     0.12,
		Palette:    heading,
	}
}

// pixelsPerBoid sets the automatic flock size: one boid per this many pixels of
// surface. Sparser than this and the boids never find each other, because two
// that never come within a perception radius cannot flock; denser and the
// screen is a wall of tails.
const pixelsPerBoid = 120

// Resize scatters the flock. Called before the first frame and on every resize.
func (b *Boids) Resize(w, h int) {
	b.w, b.h = float64(w), float64(h)
	if w <= 0 || h <= 0 {
		b.flock = nil
		return
	}
	small := math.Min(b.w, b.h)

	// The perception radius has to be a decent fraction of the window: it is
	// what decides how many neighbours a boid has, and with the density above
	// this gives it a handful rather than one or none.
	b.radius = math.Max(4, 0.22*small)
	// Separation acts over a short range only. Beyond about a third of the
	// perception radius, keeping away from a neighbour is not avoidance any
	// more, it is just cohesion with the sign flipped.
	b.sepDist = 0.35 * b.radius
	// A boid should cross the window in a few seconds at thirty frames a
	// second, so speed scales with the window rather than being fixed.
	b.maxSpeed = math.Max(10.5, 0.42*small)
	// The floor is what keeps the flock alive: without it, three rules that all
	// point at a settled flock's centre cancel and the boids hang still.
	b.minSpeed = 0.55 * b.maxSpeed
	// Steering is an acceleration, and an acceleration divided by a speed is a
	// turning rate: three radians a second, or about a hundred and seventy
	// degrees — quick enough to dodge, slow enough that the turns read as arcs
	// rather than as a boid snapping round.
	//
	// It is worth being clear that this is a limit on the change of velocity
	// per second and not on the change per frame. The two are the same thing
	// only at one frame rate; if this were applied per frame the flock would
	// turn twice as sharply for being drawn twice as often, even though its
	// speed looked right.
	b.maxForce = 3 * b.maxSpeed

	n := b.Count
	if n <= 0 {
		n = w * h / pixelsPerBoid
	}
	if n < 12 {
		n = 12
	}
	if n > 400 {
		// The cost is one distance test per boid pair in range; a cap keeps a
		// maximised window from turning into a physics benchmark.
		n = 400
	}

	tail := b.Tail
	if tail <= 0 {
		tail = 2 + int(small/28)
		if tail > 6 {
			tail = 6
		}
	}
	b.Tail = tail

	b.flock = make([]boid, n)
	for i := range b.flock {
		// A random heading at the full speed, so the flock has to form rather
		// than starting formed.
		a := b.rng.Float64() * 2 * math.Pi
		b.flock[i] = boid{
			x:  b.rng.Float64() * b.w,
			y:  b.rng.Float64() * b.h,
			vx: math.Cos(a) * b.maxSpeed,
			vy: math.Sin(a) * b.maxSpeed,
		}
	}

	// One grid cell per perception radius: with this the nine cells around a
	// boid are guaranteed to contain every neighbour within range.
	b.gw = int(b.w / b.radius)
	b.gh = int(b.h / b.radius)
	if b.gw < 1 {
		b.gw = 1
	}
	if b.gh < 1 {
		b.gh = 1
	}
	b.heads = make([]int, b.gw*b.gh)
	b.next = make([]int, n)
	b.ax = make([]float64, n)
	b.ay = make([]float64, n)
}

// wrapDelta returns the shortest offset from one coordinate to another on a
// surface that wraps. Without it a boid on the left edge sees one on the right
// edge as most of a screen away and flies off from its own flock.
func wrapDelta(d, size float64) float64 {
	half := size / 2
	if d > half {
		return d - size
	}
	if d < -half {
		return d + size
	}
	return d
}

// cellRange fills out with the grid indices to scan around c on an axis of n
// cells, wrapping, and returns how many it wrote. An axis narrower than three
// cells is scanned whole: wrapping there would name the same cell twice and
// count its boids twice over.
func cellRange(c, n int, out *[3]int) int {
	if n < 3 {
		for i := 0; i < n; i++ {
			out[i] = i
		}
		return n
	}
	out[0] = (c - 1 + n) % n
	out[1] = c
	out[2] = (c + 1) % n
	return 3
}

// cellOf maps a position to its grid cell.
func (b *Boids) cellOf(x, y float64) (int, int) {
	gx := int(x / b.w * float64(b.gw))
	gy := int(y / b.h * float64(b.gh))
	if gx < 0 {
		gx = 0
	} else if gx >= b.gw {
		gx = b.gw - 1
	}
	if gy < 0 {
		gy = 0
	} else if gy >= b.gh {
		gy = b.gh - 1
	}
	return gx, gy
}

// Frame steers the flock, moves it and draws it. dt is the seconds elapsed
// since the previous frame.
func (b *Boids) Frame(s *canvas.Surface, dt float64) {
	if len(b.flock) == 0 || dt <= 0 {
		return
	}
	b.index()
	b.steer()
	b.move(dt)
	b.draw(s)
}

// index refills the spatial grid from the current positions.
func (b *Boids) index() {
	for i := range b.heads {
		b.heads[i] = -1
	}
	for i := range b.flock {
		gx, gy := b.cellOf(b.flock[i].x, b.flock[i].y)
		c := gy*b.gw + gx
		b.next[i] = b.heads[c]
		b.heads[c] = i
	}
}

// steer computes this frame's acceleration for every boid from the three rules.
func (b *Boids) steer() {
	r2 := b.radius * b.radius
	sep2 := b.sepDist * b.sepDist
	for i := range b.flock {
		p := &b.flock[i]
		var (
			n            int
			sumX, sumY   float64 // offsets to neighbours: cohesion
			sumVX, sumVY float64 // neighbours' velocities: alignment
			sepX, sepY   float64 // accumulated push away: separation
		)
		gx, gy := b.cellOf(p.x, p.y)
		var cxs, cys [3]int
		ncx := cellRange(gx, b.gw, &cxs)
		ncy := cellRange(gy, b.gh, &cys)
		for a := 0; a < ncx; a++ {
			for c := 0; c < ncy; c++ {
				for j := b.heads[cys[c]*b.gw+cxs[a]]; j >= 0; j = b.next[j] {
					if j == i {
						continue
					}
					q := &b.flock[j]
					dx := wrapDelta(q.x-p.x, b.w)
					dy := wrapDelta(q.y-p.y, b.h)
					d2 := dx*dx + dy*dy
					if d2 > r2 || d2 == 0 {
						continue
					}
					n++
					sumX += dx
					sumY += dy
					sumVX += q.vx
					sumVY += q.vy
					if d2 < sep2 {
						// Weighted by one over the distance, so a neighbour
						// about to be collided with counts for far more than
						// one merely close. A flat push would treat both the
						// same and the flock would grind against itself.
						sepX -= dx / d2
						sepY -= dy / d2
					}
				}
			}
		}

		var ax, ay float64
		if n > 0 {
			fn := float64(n)
			cx, cy := unit(sumX/fn, sumY/fn)
			// Alignment is the difference between the neighbours' average
			// velocity and our own: the correction, not the target. Steering
			// toward the average itself would keep pulling at a boid that
			// already matches the flock exactly.
			lx, ly := unit(sumVX/fn-p.vx, sumVY/fn-p.vy)
			sx, sy := unit(sepX, sepY)
			ax = b.Separation*sx + b.Alignment*lx + b.Cohesion*cx
			ay = b.Separation*sy + b.Alignment*ly + b.Cohesion*cy
		}
		ax += (b.rng.Float64()*2 - 1) * b.Wander
		ay += (b.rng.Float64()*2 - 1) * b.Wander

		// Clamp the combined steering to the turning rate. The weights are
		// applied to unit vectors and can add to more than one, so this is
		// what stops three rules agreeing from turning a boid faster than
		// anything with momentum can turn. The result is an acceleration, in
		// pixels per second squared; move multiplies it by the elapsed time.
		if m := math.Hypot(ax, ay); m > 1 {
			ax, ay = ax/m, ay/m
		}
		b.ax[i] = ax * b.maxForce
		b.ay[i] = ay * b.maxForce
	}
}

// move applies the accelerations over dt seconds, clamps speed and wraps at
// the edges.
//
// This is where the two kinds of quantity meet. The accelerations are per
// second squared, so a frame's worth of one is a·dt; the velocities are per
// second, so a frame's worth of one is v·dt. The speed clamp sits between the
// two and is not scaled at all — it is a bound on the velocity itself, and a
// bound does not depend on how often it is checked.
func (b *Boids) move(dt float64) {
	for i := range b.flock {
		p := &b.flock[i]
		p.vx += b.ax[i] * dt
		p.vy += b.ay[i] * dt
		sp := math.Hypot(p.vx, p.vy)
		switch {
		case sp == 0:
			// Nothing to scale a zero vector by; give it a heading back.
			a := b.rng.Float64() * 2 * math.Pi
			p.vx, p.vy = math.Cos(a)*b.minSpeed, math.Sin(a)*b.minSpeed
		case sp > b.maxSpeed:
			p.vx *= b.maxSpeed / sp
			p.vy *= b.maxSpeed / sp
		case sp < b.minSpeed:
			p.vx *= b.minSpeed / sp
			p.vy *= b.minSpeed / sp
		}
		p.x = wrapPos(p.x+p.vx*dt, b.w)
		p.y = wrapPos(p.y+p.vy*dt, b.h)
	}
}

// draw paints each boid as a head and a trail behind it.
func (b *Boids) draw(s *canvas.Surface) {
	s.Clear()
	w, h := s.Size()
	for i := range b.flock {
		p := &b.flock[i]
		sp := math.Hypot(p.vx, p.vy)
		if sp == 0 {
			continue
		}
		ux, uy := p.vx/sp, p.vy/sp

		// Heading, from -pi..pi onto the whole ramp. Colouring by direction
		// rather than by index means a flock that has aligned shares a colour,
		// so the eye reads the sub-flocks without being told about them.
		idx := int((math.Atan2(p.vy, p.vx)/(2*math.Pi) + 0.5) * 255)
		if idx < 0 {
			idx = 0
		} else if idx > 255 {
			idx = 255
		}
		cr, cg, cb := b.Palette[idx].RGB()

		for k := 0; k <= b.Tail; k++ {
			// Fade along the trail so the leading pixel is unambiguous. Both
			// ends of a uniformly bright streak look alike and the flock reads
			// as static hyphens.
			f := 1 - 0.75*float64(k)/float64(b.Tail+1)
			col := tcell.NewRGBColor(
				int32(float64(cr)*f),
				int32(float64(cg)*f),
				int32(float64(cb)*f),
			)
			x := wrapIndex(p.x-ux*float64(k), w)
			y := wrapIndex(p.y-uy*float64(k), h)
			s.Set(x, y, col)
		}
	}
}

// unit returns the vector scaled to length one, or zero for a zero vector.
func unit(x, y float64) (float64, float64) {
	m := math.Hypot(x, y)
	if m == 0 {
		return 0, 0
	}
	return x / m, y / m
}

// wrapPos brings a coordinate back into [0, size).
func wrapPos(v, size float64) float64 {
	for v < 0 {
		v += size
	}
	for v >= size {
		v -= size
	}
	return v
}

// wrapIndex rounds a coordinate down to a pixel and wraps it into [0, n).
func wrapIndex(v float64, n int) int {
	i := int(math.Floor(v)) % n
	if i < 0 {
		i += n
	}
	return i
}

// Run flocks boids across the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
