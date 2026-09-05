// Package sand is a falling-sand cellular automaton.
//
// Every pixel is either empty or a grain. Once a frame each grain tries to
// move down; if the cell below is taken it tries the two cells below-left and
// below-right, and if all three are blocked it has come to rest. That is the
// entire physics, and it is enough to produce heaps that slump at an angle of
// repose, grains that pour round obstacles, and piles that collapse when
// undermined. Written from that description; no existing implementation was
// read or transliterated.
//
// Grains are emitted from a handful of fixed nozzles at the top whose color
// advances slowly, so a heap ends up banded in the order its sand arrived —
// the pile records its own history, like the layers in a jar of colored sand.
package sand

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// emitter is one nozzle: a column, and the color it is currently pouring.
type emitter struct {
	x   int
	hue byte
}

// Sand is the animation. The zero value is not usable; call New.
type Sand struct {
	w, h int

	// grid is one byte per pixel: 0 is empty air, anything else is a grain and
	// the value is its palette index. A grain keeps the index it was emitted
	// with for as long as it exists, which is what makes the bands permanent.
	//
	// There is no second buffer. Unlike Life, the rules here are not a
	// simultaneous update: a grain moves into a cell it has just checked is
	// empty, and the next grain must see it there. Double buffering sand would
	// let two grains move into the same cell and one of them would vanish.
	grid []byte

	emit  []emitter
	rng   *rand.Rand
	steps int

	// acc is unspent elapsed time. A settle sweep is discrete — a grain moves
	// a whole cell or none — so a frame runs a whole number of sweeps and
	// carries the remainder, and the sand falls at the same speed whatever the
	// frame rate is.
	acc float64

	// StepsPerSecond is how many settle sweeps run each second, which is
	// exactly the speed a falling grain travels in cells per second. It is a
	// rate in real time, not a count per frame: sand that fell faster on a
	// faster terminal would be a bug, not a feature.
	StepsPerSecond float64

	// Streams is how many nozzles pour sand. Zero means none, which leaves the
	// grid to whatever the caller has put in it. Read at Resize.
	Streams int

	// EmitChance is the probability that a given nozzle drops a grain on a
	// given step. Below one the stream breaks into a trickle, which looks far
	// more like pouring sand than a solid rod of it does.
	EmitChance float64

	// BandSteps is how many steps a nozzle keeps a color before advancing,
	// and BandStep is how far it advances. Together they set how thick the
	// stripes in a heap are.
	BandSteps int
	BandStep  int

	// DrainRate is how many grains leak out through the floor per step once
	// the heap has backed up to the nozzles.
	DrainRate int

	// Palette can be replaced before the first frame. It is indexed by the
	// grain's stored value.
	Palette canvas.Palette
}

// DefaultPalette is a closed ramp: the color at index 255 is the color at
// index 0. A nozzle's hue only ever creeps forward and wraps past the end, and
// an open ramp would snap from one end of the spectrum to the other there,
// putting a hard seam in the middle of an otherwise smooth band.
var DefaultPalette = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 255, G: 200, B: 104},
	canvas.Stop{At: 0.25, R: 232, G: 116, B: 64},
	canvas.Stop{At: 0.50, R: 184, G: 72, B: 136},
	canvas.Stop{At: 0.75, R: 88, G: 148, B: 208},
	canvas.Stop{At: 1.00, R: 255, G: 200, B: 104},
)

// New returns a sandbox. seed of 0 takes a fixed sequence, which makes tests
// repeatable; anything else varies where the grains scatter.
func New(seed int64) *Sand {
	return &Sand{
		rng:            rand.New(rand.NewSource(seed)),
		StepsPerSecond: 30,
		Streams:        5,
		EmitChance:     0.55,
		BandSteps:      24,
		BandStep:       11,
		DrainRate:      3,
		Palette:        DefaultPalette,
	}
}

// Resize allocates the grid and spaces the nozzles across the top. Called by
// canvas.Run before the first frame.
func (s *Sand) Resize(w, h int) {
	s.w, s.h = w, h
	s.grid = make([]byte, w*h)
	s.steps, s.acc = 0, 0

	n := s.Streams
	if n < 0 || w == 0 {
		n = 0
	}
	s.emit = make([]emitter, 0, n)
	for k := 0; k < n; k++ {
		// Nozzles sit at even fractions of the width rather than at the edges,
		// so every heap has room to slump on both sides.
		s.emit = append(s.emit, emitter{
			x:   (k + 1) * w / (n + 1),
			hue: hue(k * 256 / n),
		})
	}
}

// hue folds an arbitrary integer into a legal grain value. Zero is reserved
// for empty air, so a grain can never be colored with it.
func hue(v int) byte {
	b := byte(v)
	if b == 0 {
		b = 1
	}
	return b
}

// maxStepsPerFrame caps how much one frame will catch up by, so that a hitch
// or a high StepsPerSecond cannot turn a single frame into an unbounded run of
// sweeps over the whole grid.
const maxStepsPerFrame = 16

// Frame advances the sand by dt seconds and draws it.
func (s *Sand) Frame(surf *canvas.Surface, dt float64) {
	s.advance(dt)
	for y := 0; y < s.h; y++ {
		row := y * s.w
		for x := 0; x < s.w; x++ {
			if v := s.grid[row+x]; v != 0 {
				surf.Set(x, y, s.Palette[v])
			} else {
				surf.Set(x, y, tcell.ColorDefault)
			}
		}
	}
}

// advance runs whole steps out of the elapsed time. A step is indivisible —
// half a step would move a grain half a cell, and there are no half cells — so
// the leftover time is carried into the next frame rather than rounded away.
func (s *Sand) advance(dt float64) {
	if s.StepsPerSecond <= 0 {
		return
	}
	interval := 1 / s.StepsPerSecond
	s.acc += dt
	for n := 0; s.acc >= interval; n++ {
		if n >= maxStepsPerFrame {
			// Too far behind to catch up. Dropping the backlog costs a moment
			// of slow sand; carrying it would make every later frame worse.
			s.acc = 0
			return
		}
		s.step()
		s.acc -= interval
	}
}

func (s *Sand) step() {
	if s.w == 0 || s.h == 0 {
		return
	}
	s.spawn()
	s.settle()
	s.drain()
	s.steps++
}

// settle moves every grain that can move, once.
func (s *Sand) settle() {
	w := s.w
	for y := s.h - 2; y >= 0; y-- {
		// Rows are swept from the bottom up, and this is the part of the
		// algorithm that is always got wrong first. A grain moves into the row
		// below it. Sweeping top-down, that row has not been processed yet, so
		// the grain is found again immediately and moved again, and again, and
		// it teleports to the floor within a single frame — the sand does not
		// fall, it snaps down, and nothing ever appears in mid-air. Sweeping
		// bottom-up, a grain always lands in a row that is already finished
		// with, so it moves exactly one cell per frame and the fall is visible.
		//
		// The horizontal sweep alternates direction. Two grains competing for
		// the same empty cell are settled by whichever is visited first, and
		// always scanning the same way biases every one of those ties to the
		// same side, which makes heaps lean.
		lo, hi, dx := 0, w, 1
		if (y+s.steps)&1 == 1 {
			lo, hi, dx = w-1, -1, -1
		}
		row := y * w
		for x := lo; x != hi; x += dx {
			i := row + x
			v := s.grid[i]
			if v == 0 {
				continue
			}
			if s.grid[i+w] == 0 {
				s.grid[i+w] = v
				s.grid[i] = 0
				continue
			}
			// Blocked straight down: try the diagonals. Which one is tried
			// first is a coin flip per grain, because a fixed preference makes
			// every heap drift steadily in that direction instead of piling up
			// symmetrically.
			first := 1
			if s.rng.Intn(2) == 0 {
				first = -1
			}
			if !s.slide(i, x, first) {
				s.slide(i, x, -first)
			}
		}
	}
}

// slide tries to move the grain at i one cell down and dx across, reporting
// whether it moved. The grid has walls, not wrapping: sand that ran off one
// side and reappeared on the other would not read as sand at all.
func (s *Sand) slide(i, x, dx int) bool {
	nx := x + dx
	if nx < 0 || nx >= s.w {
		return false
	}
	t := i + s.w + dx
	if s.grid[t] != 0 {
		return false
	}
	s.grid[t] = s.grid[i]
	s.grid[i] = 0
	return true
}

// spawn drops new grains from the nozzles and advances the band colors.
func (s *Sand) spawn() {
	for k := range s.emit {
		e := &s.emit[k]
		if s.rng.Float64() > s.EmitChance {
			continue
		}
		// Grains land within a cell either side of the nozzle so a stream has
		// some width to it; a single-pixel column pours as a rigid thread.
		x := e.x + s.rng.Intn(3) - 1
		if x < 0 || x >= s.w {
			continue
		}
		if s.grid[x] == 0 {
			s.grid[x] = e.hue
		}
	}
	if s.BandSteps > 0 && s.steps%s.BandSteps == 0 {
		for k := range s.emit {
			s.emit[k].hue = hue(int(s.emit[k].hue) + s.BandStep)
		}
	}
}

// drain opens a hole in the floor once the sand has backed up to a nozzle.
//
// Without it the animation has an end: the box fills, every grain is blocked
// on all three sides, and the screen becomes a still image that never changes
// again. Leaking grains out of the bottom turns the box into an hourglass and
// keeps the whole column in motion indefinitely, and undermining the heap from
// below causes the collapses that are the best thing to watch here.
func (s *Sand) drain() {
	if s.DrainRate <= 0 || !s.backedUp() {
		return
	}
	floor := (s.h - 1) * s.w
	for n := 0; n < s.DrainRate; n++ {
		s.grid[floor+s.rng.Intn(s.w)] = 0
	}
}

// backedUp reports whether any nozzle is buried.
func (s *Sand) backedUp() bool {
	for _, e := range s.emit {
		if s.grid[e.x] != 0 {
			return true
		}
	}
	return false
}

// Run pours sand on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
