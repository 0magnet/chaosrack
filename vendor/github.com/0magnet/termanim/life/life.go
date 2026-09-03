// Package life is Conway's Game of Life.
//
// The rules are the published ones and are four lines long: a live cell with
// two or three live neighbours survives, a dead cell with exactly three live
// neighbours is born, everything else is dead next generation. B3/S23. The
// grid here is a torus, so gliders that leave one edge come back in at the
// other instead of falling off the world.
//
// This is written from those rules; no existing implementation was read or
// transliterated. What is added on top is entirely about making it watchable:
// cells are coloured by how long they have been alive, and the simulation
// notices when it has stopped being interesting and reseeds itself.
package life

import (
	"bytes"
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Life is the animation. The zero value is not usable; call New.
type Life struct {
	w, h int

	// cur holds an age per cell rather than a bit: 0 is dead, n means alive and
	// alive for n generations, saturating at 255. Life drawn as one flat colour
	// reads as television static — every cell looks the same and the eye cannot
	// tell a stable block from a boiling region. Age gives the pattern a
	// history: births flash bright, still lifes settle into the dark end of the
	// ramp, and the shape of the activity becomes visible.
	//
	// next is the buffer the following generation is written into. Life is
	// defined as a simultaneous update — every cell reads the same generation —
	// so writing new states into the grid being read would let a cell see its
	// neighbour's future. Double buffering is not an optimisation here, it is
	// the rule.
	cur, next []byte

	// mask1 and mask2 are the alive/dead bitmaps of the previous two
	// generations, used by the stagnation check. maskNew is the scratch buffer
	// the three rotate through so no generation allocates.
	mask1, mask2, maskNew []byte

	pop     int // population of the last generation, -1 when unknown
	popSame int // generations the population has not changed for
	cycle   int // generations the grid has matched the one two back
	reseeds int // how many times stagnation has been broken, for tests
	gens    int // generations computed since Resize

	// acc is unspent elapsed time. Generations are discrete: a frame runs a
	// whole number of them and carries the remainder to the next frame, which
	// is what keeps the board evolving at the same rate whatever the frame
	// rate happens to be.
	acc float64

	rng *rand.Rand

	// GensPerSecond is how fast the board evolves, in generations per second.
	// It is a rate in real time rather than a count per frame: the frame rate
	// is whatever the terminal and the machine can manage, and a generation is
	// far too visible a unit to let it change with it.
	GensPerSecond float64

	// Density is the fraction of cells made live when seeding or reseeding.
	// Around a third is the classic soup: dense enough to produce structure,
	// sparse enough that it does not immediately die of overcrowding.
	Density float64

	// AgeFade is how many palette steps a cell dims for each generation it
	// survives, and MinIntensity is the floor it stops at. The floor matters:
	// fading all the way to black would erase the still lifes that most of a
	// settled board is made of.
	AgeFade      int
	MinIntensity int

	// CycleStallGens reseeds once the grid has been identical to the grid two
	// generations ago this many times running. That catches still lifes and
	// period-2 oscillators, which is what a soup almost always decays into.
	//
	// PopStallGens is the slower net: a constant population for this long means
	// nothing is being created or destroyed, which catches longer cycles and
	// lone gliders sailing round the torus forever.
	CycleStallGens int
	PopStallGens   int

	// Palette can be replaced before the first frame. It is indexed by
	// intensity, so index 255 is a cell born this generation.
	Palette canvas.Palette
}

// DefaultPalette runs from a deep blue for cells that have been sitting there
// for a long time up to near white for a cell born this generation. Reading it
// as heat is the point: activity is bright, sediment is dark.
var DefaultPalette = canvas.NewPalette(
	canvas.Stop{At: 0.00, R: 8, G: 16, B: 56},
	canvas.Stop{At: 0.45, R: 0, G: 96, B: 168},
	canvas.Stop{At: 0.78, R: 48, G: 208, B: 224},
	canvas.Stop{At: 1.00, R: 236, G: 255, B: 255},
)

// New returns a Life. seed of 0 takes a fixed sequence, which makes tests
// repeatable; anything else varies the soup.
func New(seed int64) *Life {
	return &Life{
		rng:            rand.New(rand.NewSource(seed)),
		pop:            -1,
		GensPerSecond:  30,
		Density:        0.32,
		AgeFade:        7,
		MinIntensity:   36,
		CycleStallGens: 12,
		PopStallGens:   150,
		Palette:        DefaultPalette,
	}
}

// Resize allocates the grids and seeds a fresh soup. Called by canvas.Run
// before the first frame and again on every size change.
func (l *Life) Resize(w, h int) {
	l.w, l.h = w, h
	n := w * h
	l.cur = make([]byte, n)
	l.next = make([]byte, n)
	l.mask1 = make([]byte, n)
	l.mask2 = make([]byte, n)
	l.maskNew = make([]byte, n)
	l.pop, l.popSame, l.cycle = -1, 0, 0
	l.gens, l.acc = 0, 0
	l.seed(0, 0, w, h)
	l.poison()
}

// seed fills a rectangle with random live cells, wrapping at the edges like
// everything else here.
func (l *Life) seed(x0, y0, w, h int) {
	if l.w == 0 || l.h == 0 {
		return
	}
	for dy := 0; dy < h; dy++ {
		y := (y0 + dy) % l.h
		for dx := 0; dx < w; dx++ {
			x := (x0 + dx) % l.w
			if l.rng.Float64() < l.Density {
				l.cur[y*l.w+x] = 1
			} else {
				l.cur[y*l.w+x] = 0
			}
		}
	}
}

// poison writes a value into the history bitmaps that a real bitmap can never
// hold, since those only ever contain 0 or 1. Without it a reseed would be
// compared against the pattern it just replaced and could re-trigger straight
// away.
func (l *Life) poison() {
	for i := range l.mask1 {
		l.mask1[i] = 0xff
		l.mask2[i] = 0xfe
	}
}

// maxGensPerFrame caps how much a single frame will catch up by. The frame
// loop already clamps dt, but a rate set high by a caller must not be able to
// turn one slow frame into an unbounded pile of generations; better to drop
// the backlog and stay responsive.
const maxGensPerFrame = 8

// Frame advances the board by dt seconds' worth of generations and draws it.
func (l *Life) Frame(s *canvas.Surface, dt float64) {
	l.advance(dt)
	for y := 0; y < l.h; y++ {
		row := y * l.w
		for x := 0; x < l.w; x++ {
			if age := l.cur[row+x]; age > 0 {
				s.Set(x, y, l.Palette[l.intensity(age)])
			} else {
				s.Set(x, y, tcell.ColorDefault)
			}
		}
	}
}

// advance runs whole generations out of the elapsed time. A generation is not
// divisible — there is no such thing as running a third of one — so the
// leftover time is carried rather than rounded away, and a rate that is not a
// multiple of the frame rate still comes out right on average.
func (l *Life) advance(dt float64) {
	if l.GensPerSecond <= 0 {
		return
	}
	interval := 1 / l.GensPerSecond
	l.acc += dt
	for n := 0; l.acc >= interval; n++ {
		if n >= maxGensPerFrame {
			// Too far behind to catch up. Throwing the debt away keeps the
			// frame time bounded; carrying it would make every later frame
			// worse than the last.
			l.acc = 0
			return
		}
		l.step()
		l.acc -= interval
	}
}

// intensity maps age to a palette index: brightest the generation a cell is
// born, dimming to a floor as it survives.
func (l *Life) intensity(age byte) int {
	v := 255 - int(age)*l.AgeFade
	if v < l.MinIntensity {
		v = l.MinIntensity
	}
	return v
}

func (l *Life) step() {
	if l.w == 0 || l.h == 0 {
		return
	}
	w, h := l.w, l.h
	pop := 0
	for y := 0; y < h; y++ {
		// Neighbour rows are resolved once per row rather than once per cell.
		// The wrap is what makes the grid a torus.
		up, dn := y-1, y+1
		if up < 0 {
			up = h - 1
		}
		if dn >= h {
			dn = 0
		}
		ru, rc, rd := up*w, y*w, dn*w
		for x := 0; x < w; x++ {
			lf, rt := x-1, x+1
			if lf < 0 {
				lf = w - 1
			}
			if rt >= w {
				rt = 0
			}
			n := 0
			if l.cur[ru+lf] > 0 {
				n++
			}
			if l.cur[ru+x] > 0 {
				n++
			}
			if l.cur[ru+rt] > 0 {
				n++
			}
			if l.cur[rc+lf] > 0 {
				n++
			}
			if l.cur[rc+rt] > 0 {
				n++
			}
			if l.cur[rd+lf] > 0 {
				n++
			}
			if l.cur[rd+x] > 0 {
				n++
			}
			if l.cur[rd+rt] > 0 {
				n++
			}

			i := rc + x
			age := l.cur[i]
			switch {
			case age > 0 && (n == 2 || n == 3):
				// Survives. Age saturates rather than wrapping, or an ancient
				// still life would flash bright every 256 generations.
				if age < 255 {
					age++
				}
			case age == 0 && n == 3:
				age = 1
			default:
				age = 0
			}
			l.next[i] = age
			if age > 0 {
				pop++
			}
		}
	}
	l.cur, l.next = l.next, l.cur
	l.gens++
	l.checkStall(pop)
}

// checkStall is the reason this package is more than the four rules.
//
// A random soup does not run forever. Within a few hundred generations it
// burns down to a scattering of still lifes and period-2 blinkers, and from
// then on the screen is a frozen photograph — technically the animation is
// still running, but nothing on it moves again, ever. Two cheap tests catch
// that. The first compares the live/dead bitmap against the one from two
// generations ago: still lifes and any period-1 or period-2 oscillator match
// it exactly, every generation. The second watches for a population that has
// not changed at all for a long time, which catches longer cycles and a lone
// glider circling the torus.
//
// The cure is deliberately partial. Reseeding a random rectangle rather than
// the whole board leaves the existing structures standing and lets the new
// soup crash into them, which looks like the animation recovering rather than
// like someone pressing reset.
func (l *Life) checkStall(pop int) {
	if pop == l.pop {
		l.popSame++
	} else {
		l.popSame = 0
	}
	l.pop = pop

	for i, v := range l.cur {
		if v > 0 {
			l.maskNew[i] = 1
		} else {
			l.maskNew[i] = 0
		}
	}
	if bytes.Equal(l.maskNew, l.mask2) {
		l.cycle++
	} else {
		l.cycle = 0
	}
	// Rotate the three buffers: mask1 becomes the generation just computed,
	// mask2 the one before it, and the buffer mask2 held is recycled as
	// scratch for next time. Nothing is allocated.
	l.mask2, l.mask1, l.maskNew = l.mask1, l.maskNew, l.mask2

	if l.cycle >= l.CycleStallGens || l.popSame >= l.PopStallGens {
		l.reseed()
	}
}

// reseed drops fresh soup into a random patch, roughly a third of the board in
// each direction, and clears the stagnation state.
func (l *Life) reseed() {
	if l.w == 0 || l.h == 0 {
		return
	}
	rw, rh := l.w/3+2, l.h/3+2
	l.seed(l.rng.Intn(l.w), l.rng.Intn(l.h), rw, rh)
	l.poison()
	l.pop, l.popSame, l.cycle = -1, 0, 0
	l.reseeds++
}

// Run plays Life on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
