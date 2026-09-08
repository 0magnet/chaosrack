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
	// of its neighbors — so it cannot be run by a fractional amount; instead
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
	// color. Green fire is a perfectly good screensaver.
	Palette canvas.Palette

	// AudioGain scales how hard sound drives the flame. 1 is the tuned
	// amount; 0 ignores audio entirely even with a source attached, which is
	// the knob to reach for rather than detaching the source.
	AudioGain float64

	// audio is the last sound handed in and env smooths it. See Listen.
	audio canvas.Audio
	env   canvas.Envelope

	// fuelGap is the heat the gaps in the fuel row carry this frame, worked
	// out from the envelope once per frame rather than once per cell. Zero is
	// silence, and zero is exactly what those gaps held before there was any
	// audio at all.
	fuelGap byte
}

// New returns a fire. seed of 0 takes a fixed sequence, which makes tests
// repeatable; anything else varies the flicker.
func New(seed int64) *Fire {
	f := &Fire{
		rng:       rand.New(rand.NewSource(seed)), //nolint:gosec
		Palette:   canvas.Fire,
		StepRate:  30,
		AudioGain: 1,
	}
	// A flame answers a beat quickly and dies back slowly, which is what a
	// real one does when something is thrown on it. 40 ms up is the package
	// default and the fastest that still ignores a lone bad sample; the flame
	// gets no less, because a leap that arrives late has missed the beat. The
	// decay is longer than the default because heat takes time to leave a
	// fire, and a flame that snapped back down between kicks reads as a strobe
	// rather than as burning.
	f.env.Attack = 0.040
	f.env.Decay = 0.30
	return f
}

// Listen takes the sound of the coming frame. See canvas.AudioListener.
//
// The value is only stored. It is smoothed in Frame, where dt is known, so the
// envelope advances by elapsed time like everything else here rather than by
// however often the host happens to call this.
func (f *Fire) Listen(a canvas.Audio) { f.audio = a }

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
	// The sound of this frame decides how much fuel there is; see step. Done
	// once per frame and not once per simulation step, because the envelope is
	// in seconds and a step is not.
	f.fuelGap = f.gapHeat(dt)

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

// gapHeat advances the audio envelope by dt and returns the heat a beat puts
// into the gaps of the fuel row.
//
// The mapping is: loudness fills the holes in the fuel.
//
// The fuel row is deliberately about a third gaps — that is what makes the
// flame flicker and split instead of standing there as a wall — and how much
// heat reaches the visible rows is governed by how much of that row is lit,
// because every cell is the average of the three beneath it. So closing the
// gaps is the one lever that makes the whole flame surge together, root and
// tip, rather than brightening it in place. It also has room to move: the lit
// cells are already at 180..255 and cannot go hotter, while the gaps are at
// zero and can go all the way. At full level the fuel row is solid heat and
// the flame leaps; in between the gaps glow and the flame thickens.
//
// The alternative considered was raising the lit cells' floor, which is a
// change of a few percent because they are near saturation already, and
// looked like nothing.
func (f *Fire) gapHeat(dt float64) byte {
	f.env.Step(f.audio, dt)
	g := f.env.Level() * f.AudioGain
	if g <= 0 {
		// Silence, and the common case: no source attached at all. The gaps
		// are empty, which is what they always were.
		return 0
	}
	if g > 1 {
		g = 1
	}
	return byte(g * 255) //nolint:gosec // g is clamped to 0..1 just above
}

func (f *Fire) step() {
	if f.w == 0 || f.h == 0 {
		return
	}

	// Re-seed the fuel row. Most of it burns hot; the gaps are what make the
	// flame flicker and split rather than stand there as a solid wall — except
	// on a beat, when they fill in and the flame leaps. See gapHeat.
	//
	// The random draws are unchanged and in the same order whatever the sound
	// is doing, so a given seed produces the same flicker with audio as
	// without; only what lands in the gaps differs.
	fuel := f.heat[f.h]
	for x := 0; x < f.w; x++ {
		if f.rng.Intn(10) < 7 {
			fuel[x] = byte(180 + f.rng.Intn(76)) //nolint:gosec
		} else {
			fuel[x] = f.fuelGap
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
