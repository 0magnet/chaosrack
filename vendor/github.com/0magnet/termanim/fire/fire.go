// Package fire is a burning flame.
//
// The method is the one every terminal fire demo uses and is old enough to be
// folklore: keep a grid of heat, seed a row of noise below the bottom of the
// screen, and let each cell cool toward the average of the cells beneath it.
// It is written here from that description. libcaca's cacafire and aalib's
// aafire are what it is meant to look like, but no code is taken from either.
package fire

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Fire is the animation. The zero value is not usable; call New.
type Fire struct {
	w, h int
	// heat is h+1 rows: the extra row at the bottom is the fuel and is never
	// drawn. Without it the lowest visible row has nothing to average from and
	// the flame has no root.
	heat [][]byte
	rng  *rand.Rand

	// acc carries the fraction of a simulation step left over from the last
	// frame. The cooling rule is a discrete step — a cell becomes the average
	// of its neighbours — so it cannot be run by a fractional amount; instead
	// elapsed time accumulates and whole steps are taken from it.
	acc float64

	// StepRate is simulation steps per second. The flame was tuned at one
	// step per frame at 30 fps, so 30 keeps its old speed while the screen
	// redraws more often.
	StepRate float64

	// Reach is how far up the surface the flame climbs before it burns out, as
	// a fraction. Zero means the default.
	//
	// It has to be a fraction rather than a fixed rate of cooling, or the flame
	// stands the same number of rows tall whatever it is drawn in: filling a
	// short terminal and sitting in the bottom of a browser window with a
	// screen's worth of black above it.
	Reach float64

	// decay is how much heat a row loses per step, worked out from Reach when
	// the surface is sized. A table rather than an expression because the
	// expression was a division per cell per step, and there are as many cells
	// as pixels.
	decay []int

	// Palette can be replaced before the first frame to burn a different
	// colour. Green fire is a perfectly good screensaver.
	Palette canvas.Palette
}

// New returns a fire. seed of 0 takes a fixed sequence, which makes tests
// repeatable; anything else varies the flicker.
func New(seed int64) *Fire {
	return &Fire{
		rng:      rand.New(rand.NewSource(seed)),
		Palette:  canvas.Fire,
		StepRate: 30,
	}
}

// Resize allocates the heat grid. Called by canvas.Run before the first frame.
func (f *Fire) Resize(w, h int) {
	f.w, f.h = w, h
	f.heat = make([][]byte, h+1)
	for y := range f.heat {
		f.heat[y] = make([]byte, w)
	}
	f.decay = decayTable(h, f.Reach)
}

// defaultReach leaves the top quarter to smoke. A flame that climbs the whole
// surface has nowhere to thin out and reads as a wall.
const defaultReach = 0.75

// decayTable works out how fast each row cools so the flame burns out `reach`
// of the way up.
//
// Cooling rises with height, which is what makes the flame taper rather than
// stop dead. Heat leaves the fuel at 255 and loses decay[y] per row, so the
// flame burns out where the losses have added up to 255:
//
//	base·d + k·d²/2h = 255, for a flame d rows tall and decay 1 + (h-y)·k/h
//
// Solving that for k is what ties the height of the flame to the height of the
// surface rather than to a fixed rate of cooling.
//
// base is not 1, because subtracting decay is not the only heat a row loses.
// Averaging loses some too — a cell settles toward the row below it minus about
// a third more than the decay, and the fuel row is a third gaps, so the heat
// arriving from below is well under 255 to begin with. base absorbs all of
// that, and it is measured rather than derived: the closed form below gets the
// shape right, which is what makes the flame scale with the surface, but not
// the constant.
//
// So the flame does not land on exactly Reach at every size. It is within
// half a screen of it across the range of sizes these run at, and it tracks the
// surface instead of standing at a fixed number of rows, which is the thing
// that was actually wrong.
func decayTable(h int, reach float64) []int {
	if h <= 0 {
		return nil
	}
	if reach <= 0 || reach > 1 {
		reach = defaultReach
	}
	const base = 2.5
	d := reach * float64(h)

	// Cooling cannot go below the one unit a row that makes the flame finite,
	// so on a surface tall enough that even that burns 255 away before the top
	// the flame simply reaches as far as it can. That is about 127 rows.
	var k float64
	if base*d < 255 {
		k = 2 * float64(h) * (255 - base*d) / (d * d)
	}

	t := make([]int, h)
	for y := range t {
		t[y] = 1 + int(float64(h-y)*k/float64(h))
	}
	return t
}

// Frame advances the simulation and draws it.
func (f *Fire) Frame(s *canvas.Surface, dt float64) {
	rate := f.StepRate
	if rate <= 0 {
		rate = 30
	}
	f.acc += dt * rate
	// Cap the catch-up. canvas already clamps dt, but a rule that costs a full
	// grid sweep should never run an unbounded number of times in one frame
	// just because the process was descheduled.
	if f.acc > 4 {
		f.acc = 4
	}
	for f.acc >= 1 {
		f.step()
		f.acc--
	}
	for y := 0; y < f.h; y++ {
		row := f.heat[y]
		for x := 0; x < f.w; x++ {
			if h := row[x]; h > 0 {
				s.Set(x, y, f.Palette[h])
			} else {
				s.Set(x, y, tcell.ColorDefault)
			}
		}
	}
}

func (f *Fire) step() {
	if f.w == 0 || f.h == 0 {
		return
	}

	// Re-seed the fuel row. Most of it burns hot; the gaps are what make the
	// flame flicker and split rather than stand there as a solid wall.
	fuel := f.heat[f.h]
	for x := 0; x < f.w; x++ {
		if f.rng.Intn(10) < 7 {
			fuel[x] = byte(180 + f.rng.Intn(76))
		} else {
			fuel[x] = 0
		}
	}

	// Cool upward. Each cell becomes the average of the three cells below it
	// and itself. Including the diagonals is what makes the flame lean and
	// curl instead of rising in straight columns.
	for y := 0; y < f.h; y++ {
		row, below := f.heat[y], f.heat[y+1]
		// Cooling rises with height, which is what makes the flame taper
		// instead of stopping dead. See decayTable.
		decay := f.decay[y]
		for x := 0; x < f.w; x++ {
			l, r := x-1, x+1
			if l < 0 {
				l = 0
			}
			if r >= f.w {
				r = f.w - 1
			}
			// Rounded to nearest, not truncated. Truncating biases every cell
			// down by half a unit on average, and a cell is the average of the
			// row below it, so that bias compounds up the screen as a second
			// cooling term nothing accounts for. It cost the flame about a third
			// of its height and put a hard ceiling on how tall it could be at
			// all, which no amount of tuning the decay could lift.
			v := (int(below[l]) + int(below[x]) + int(below[r]) + int(row[x]) + 2) / 4
			if v > decay {
				v -= decay
			} else {
				v = 0
			}
			row[x] = byte(v)
		}
	}
}

// Run draws fire on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
