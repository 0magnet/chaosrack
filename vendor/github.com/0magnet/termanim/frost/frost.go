// Package frost is diffusion-limited aggregation: frost on a window, or coral.
//
// One frozen pixel in the middle. Particles wander in from outside on a random
// walk, and the first time one finds itself touching the frozen cluster it
// freezes where it stands and never moves again. That is the whole rule, and
// out of it comes the branched, feathery, self-similar shape that turns up
// wherever growth is limited by how fast material arrives rather than by how
// fast it can be laid down — frost ferns, mineral dendrites, copper
// electrodeposits, the edge of a lichen.
//
// The branching is not a rule, it is a consequence, and it is worth saying why
// because the shape is otherwise mysterious. A tip that sticks out into the
// open is hit by wanderers from a wide arc; a hollow between two tips is
// screened, because almost every walk that would reach it touches a tip on the
// way in. So tips grow faster than hollows, and the lead they gain makes them
// screen more, which is a feedback loop with no stable width. The cluster ends
// up mostly empty space — its dimension is about 1.71, not 2 — and it can
// never fill in behind itself.
//
// Color is the order of arrival, so the picture is also its own clock: the
// oldest work sits dim at the middle and the growing tips are bright, and the
// eye reads out the history of the growth from the gradient along a branch.
//
// # Why the spawn ring and the kill radius
//
// Written naively this is unwatchably slow. Release a walker from the edge of
// the window and it takes on the order of the square of that distance in steps
// to arrive anywhere near the cluster, nearly all of them spent far away doing
// nothing, and the cost per particle grows as the cluster does. Two standard
// tricks fix it, both due to the physics literature on the model:
//
//   - a walker is released on a circle just outside the current cluster
//     radius, rather than from the edge of the window, so it starts where the
//     interesting part of its walk begins; and
//   - a walker that wanders out past a larger kill radius is not followed. It
//     is put back on the release circle at a fresh random angle, because a
//     two-dimensional random walk is recurrent and would come back eventually
//     anyway, and following it out and back is the same picture for a hundred
//     times the work.
//
// The honest caveat: re-releasing at a uniformly random angle is not exactly
// the distribution a walk that escaped would return with. The exact fix is to
// re-inject weighted by the harmonic measure of the kill circle, and the
// approximation used here biases the growth very slightly toward radial
// symmetry. At the size of a terminal window that is invisible, and it buys
// two orders of magnitude.
//
// Written from a description of the model, not from any implementation of it.
package frost

import (
	"math"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Ice is the default ramp: the deep blue of old, buried growth through cyan to
// a white-hot growing tip.
//
// It runs cold on purpose. The same animation with a warm ramp reads as coral
// or as a mineral dendrite, which is the same object — swapping the palette is
// the whole of the difference.
var Ice = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 12, G: 24, B: 72},
	canvas.Stop{At: 0.45, R: 32, G: 120, B: 200},
	canvas.Stop{At: 0.80, R: 150, G: 230, B: 255},
	canvas.Stop{At: 1.00, R: 255, G: 255, B: 255},
)

// walker is one wandering particle. Positions are lattice cells and may be
// outside the surface, which is where the kill radius does its work.
type walker struct {
	x, y int
}

// The four lattice steps a walker may take.
var (
	stepX = [4]int{1, -1, 0, 0}
	stepY = [4]int{0, 0, 1, -1}
)

// Frost is the animation. The zero value is not usable; call New.
type Frost struct {
	w, h   int
	cx, cy int // the seed, and the center every radius is measured from

	// grid holds 0 for open space, or the arrival ordinal of the particle
	// frozen there — 1 for the seed, counting up. Storing the ordinal rather
	// than a bit is what makes the growth history visible: the value is both
	// "this is ice" and "this is when".
	grid []uint32

	walkers []walker

	stuck uint32  // particles frozen so far, which is the newest ordinal
	maxR  float64 // how far the cluster reaches from the seed, in pixels
	limit float64 // the radius at which the cluster has filled the window

	// acc is unspent elapsed time. A lattice step is discrete, so a frame runs
	// a whole number of them and carries the remainder, which keeps the growth
	// at the same speed whatever the frame rate is.
	acc   float64
	steps int

	rng *rand.Rand

	// Walkers is how many particles wander at once. They do not interact — one
	// at a time would grow exactly the same cluster — so this is only a way of
	// spending a frame's budget on several walks instead of one. More of them
	// makes the drifting motes around the cluster easier to see and costs
	// nothing else.
	Walkers int

	// StepsPerSecond is how many lattice steps each walker takes per second.
	// This is the single knob for how fast the frost grows; everything else
	// about the shape is fixed by the model.
	StepsPerSecond float64

	// SpawnMargin is how far outside the cluster radius a walker is released,
	// in pixels. Too small and a walker is released already touching a branch,
	// which grows a smooth shell instead of a dendrite; a few pixels is enough
	// for the walk to forget where it started.
	SpawnMargin float64

	// KillFactor is the kill radius as a multiple of the release radius. Below
	// about 1.5 the growth is visibly distorted, because walks that would have
	// come back around to the far side of the cluster are being recycled
	// instead. Above 3 it costs work and changes nothing.
	KillFactor float64

	// MinIntensity is how dim the oldest ice is, from 0 to 255. The newest is
	// always at full brightness. A floor rather than a fade to black, because
	// the buried structure is the record of how the thing grew and losing it
	// would leave only a bright rind.
	MinIntensity int

	// ShowWalkers draws the wandering particles as faint motes. They are not
	// part of the cluster and they will not be there next frame; what they buy
	// is that the picture is visibly doing something even early on, when the
	// cluster is a handful of pixels and growing slowly.
	ShowWalkers bool

	// Palette colors the ice by arrival order, dim for the oldest.
	Palette canvas.Palette
}

// New returns a frost animation. seed of 0 grows a fixed crystal, which makes
// tests repeatable.
func New(seed int64) *Frost {
	return &Frost{
		rng: rand.New(rand.NewSource(seed)), //nolint:gosec
		// 32 walkers at 8000 steps each is twelve times the throughput this
		// first shipped with, and the geometry below is untouched: SpawnMargin
		// and KillFactor both distort the shape, and throughput does not.
		//
		// Measured on a 200x100 surface, which is a 200x50 terminal: at the
		// original 8/2600 the crystal reached 411 pixels in a minute and had
		// still not filled the window, so what a viewer saw was a small smudge
		// not obviously doing anything. At 32/8000 it fills and starts over
		// every twenty to thirty seconds, which is a growth arc you can watch,
		// and it costs 0.26ms a frame.
		Walkers:        32,
		StepsPerSecond: 8000,
		SpawnMargin:    6,
		KillFactor:     2.5,
		MinIntensity:   60,
		ShowWalkers:    true,
		Palette:        Ice,
	}
}

// Resize plants a fresh seed and releases the walkers. Called before the first
// frame and on every resize.
func (f *Frost) Resize(w, h int) {
	f.w, f.h = w, h
	f.grid = make([]uint32, w*h)

	n := f.Walkers
	if n < 1 {
		n = 1
	}
	f.walkers = make([]walker, n)
	f.reset()
}

// reset clears the window and starts a new crystal from one frozen pixel.
func (f *Frost) reset() {
	for i := range f.grid {
		f.grid[i] = 0
	}
	f.acc, f.steps = 0, 0
	f.cx, f.cy = f.w/2, f.h/2
	f.maxR = 0
	f.stuck = 0
	// The cluster is a disc about the seed, so it runs out of room at half the
	// shorter side. Stopping a little short of that leaves the release circle
	// somewhere a walker can still get around the outside of.
	f.limit = float64(min(f.w, f.h))/2 - 2
	if f.w == 0 || f.h == 0 {
		return
	}
	f.stuck = 1
	f.grid[f.cy*f.w+f.cx] = 1
	for i := range f.walkers {
		f.release(&f.walkers[i])
	}
}

// release puts a walker on the circle just outside the cluster.
func (f *Frost) release(p *walker) {
	a := f.rng.Float64() * 2 * math.Pi
	r := f.spawnRadius()
	p.x = f.cx + int(math.Round(math.Cos(a)*r))
	p.y = f.cy + int(math.Round(math.Sin(a)*r))
}

func (f *Frost) spawnRadius() float64 { return f.maxR + f.SpawnMargin }

// maxStepsPerFrame caps how much one frame will catch up by, per walker. A
// fully clamped frame at the default rate is 260 steps, so this is slack.
const maxStepsPerFrame = 4000

// Frame walks the particles for dt seconds and draws the crystal.
func (f *Frost) Frame(s *canvas.Surface, dt float64) {
	if f.w == 0 || f.h == 0 {
		return
	}
	if f.StepsPerSecond > 0 {
		interval := 1 / f.StepsPerSecond
		f.acc += dt
		for n := 0; f.acc >= interval; n++ {
			if n >= maxStepsPerFrame {
				f.acc = 0
				break
			}
			f.step()
			f.acc -= interval
		}
	}
	f.draw(s)
}

// step moves every walker one lattice cell and freezes any that has arrived.
func (f *Frost) step() {
	f.steps++
	kill := f.spawnRadius() * f.KillFactor
	for i := range f.walkers {
		p := &f.walkers[i]
		d := f.rng.Intn(4)
		p.x += stepX[d]
		p.y += stepY[d]

		dx, dy := float64(p.x-f.cx), float64(p.y-f.cy)
		if dx*dx+dy*dy > kill*kill {
			// Gone. See the package comment for why it is not followed.
			f.release(p)
			continue
		}
		if !f.touching(p.x, p.y) {
			continue
		}
		f.freeze(p.x, p.y)
		if f.maxR >= f.limit {
			// The crystal has filled the window. Start another one rather
			// than leave a finished picture on the screen: the growth is the
			// animation, and a static frost fern is a screenshot.
			f.reset()
			return
		}
		f.release(p)
	}
}

// touching reports whether an open cell inside the window has ice orthogonally
// next to it.
//
// Orthogonal and not diagonal: sticking to a diagonal neighbor lets a branch
// advance corner to corner, which fattens the tips and washes out the very
// screening effect that makes the shape. It also means the cluster is
// four-connected, which is a property worth being able to assert.
func (f *Frost) touching(x, y int) bool {
	if x < 0 || y < 0 || x >= f.w || y >= f.h {
		return false
	}
	if f.grid[y*f.w+x] != 0 {
		// A walker never enters an occupied cell: to step into one it would
		// have had to be touching it first, and it would have frozen then.
		return false
	}
	i := y*f.w + x
	return (x > 0 && f.grid[i-1] != 0) ||
		(x+1 < f.w && f.grid[i+1] != 0) ||
		(y > 0 && f.grid[i-f.w] != 0) ||
		(y+1 < f.h && f.grid[i+f.w] != 0)
}

// freeze turns one open cell into ice and extends the cluster radius.
func (f *Frost) freeze(x, y int) {
	f.stuck++
	f.grid[y*f.w+x] = f.stuck
	dx, dy := float64(x-f.cx), float64(y-f.cy)
	if r := math.Sqrt(dx*dx + dy*dy); r > f.maxR {
		f.maxR = r
	}
}

// draw paints the crystal, oldest ice dim and the growing tips bright.
func (f *Frost) draw(s *canvas.Surface) {
	span := 255 - f.MinIntensity
	if span < 0 {
		span = 0
	}
	newest := f.stuck
	if newest == 0 {
		newest = 1
	}
	// One divide for the whole crystal rather than one per pixel of ice.
	scale := float64(span) / float64(newest)
	for y := 0; y < f.h; y++ {
		row := y * f.w
		for x := 0; x < f.w; x++ {
			g := f.grid[row+x]
			if g == 0 {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			// Arrival order scaled against however much has arrived so far,
			// so the ramp always spans the whole crystal however big it has
			// got. A fixed scale would leave a large crystal all one color.
			v := f.MinIntensity + int(float64(g)*scale)
			if v > 255 {
				v = 255
			}
			s.Set(x, y, f.Palette[v])
		}
	}
	if !f.ShowWalkers {
		return
	}
	for _, p := range f.walkers {
		if s.At(p.x, p.y) == tcell.ColorDefault {
			s.Set(p.x, p.y, f.Palette[f.MinIntensity/2])
		}
	}
}

// Run grows frost on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
