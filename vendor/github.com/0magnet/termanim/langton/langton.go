// Package langton is Langton's Ant.
//
// The whole machine is two rules. An ant sits on a cell of a grid facing one
// of four directions: on a white cell it turns right, paints the cell black
// and steps forward; on a black cell it turns left, paints it white and steps
// forward. That is all of it, and out of it comes the famous behaviour — about
// ten thousand steps of what looks like pure noise, and then, with no warning
// and for no reason anyone has managed to prove in general, the ant falls into
// a 104-step cycle that translates it across the plane forever, building a
// diagonal "highway".
//
// This is written from those two rules; no existing implementation was read or
// transliterated. The grid wraps, so a highway does not escape but drives back
// into its own old territory, which knocks the ant out of the cycle and starts
// the chaos again. Several ants share the board and paint in different
// colours: they cross each other's trails constantly, and the interference is
// far better to watch than a single ant.
package langton

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Facing is stored as 0 up, 1 right, 2 down, 3 left, so a right turn is +1 and
// a left turn is -1 modulo four.
var (
	stepX = [4]int{0, 1, 0, -1}
	stepY = [4]int{-1, 0, 1, 0}
)

type ant struct {
	x, y int
	dir  int
}

// Langton is the animation. The zero value is not usable; call New.
type Langton struct {
	w, h int

	// cell is 0 for a white cell, or 1+the index of the ant that last painted
	// it black. Folding ownership into the state byte is what lets each ant
	// have its own colour without a second grid.
	cell []byte
	// stamp is the step count at which each cell was last flipped. Drawing
	// straight from cell would give one flat slab of colour per ant with no
	// sense of what the ant is doing now; fading by how long ago a cell was
	// touched makes the live front of the pattern glow and the old work recede.
	stamp []uint32
	// now counts ant moves. It is unsigned so that the age subtraction below
	// stays correct when it wraps, which takes days at these rates.
	now uint32

	// acc is unspent elapsed time. An ant move is discrete, so a frame runs a
	// whole number of them and carries the remainder, which keeps the ants
	// walking at the same speed whatever the frame rate is.
	acc float64

	ants []ant
	pals []canvas.Palette
	rng  *rand.Rand

	// Ants is how many ants share the board. Read at Resize.
	Ants int

	// StepsPerSecond is how fast the ants walk, in moves per second. One move
	// per frame would be unwatchable — the interesting structure needs tens of
	// thousands of moves — so a frame is a burst of them, sized by how long
	// the frame actually took rather than by a fixed count.
	StepsPerSecond float64

	// Fade is how many steps a cell dims by one palette index, and
	// MinIntensity is the floor. The floor keeps the ant's older work visible
	// instead of letting the picture erase itself from behind.
	Fade         uint32
	MinIntensity int
}

// antColours are the base hues ants paint with, in order. Ants past the end of
// the list wrap round and reuse a hue, which only happens with a lot of ants.
var antColours = [][3]int{
	{255, 96, 72},
	{88, 208, 255},
	{255, 214, 96},
	{144, 255, 128},
	{224, 128, 255},
	{255, 152, 64},
}

// New returns an ant colony. seed of 0 takes a fixed sequence, which makes
// tests repeatable; anything else moves the ants' starting positions.
func New(seed int64) *Langton {
	return &Langton{
		rng:            rand.New(rand.NewSource(seed)),
		Ants:           3,
		StepsPerSecond: 7500,
		Fade:           150,
		MinIntensity:   55,
	}
}

// Resize allocates the grid, places the ants and builds one colour ramp per
// ant. Called by canvas.Run before the first frame.
func (l *Langton) Resize(w, h int) {
	l.w, l.h = w, h
	l.cell = make([]byte, w*h)
	l.stamp = make([]uint32, w*h)
	l.now, l.acc = 0, 0

	n := l.Ants
	if n < 1 {
		n = 1
	}
	l.ants = make([]ant, n)
	l.pals = make([]canvas.Palette, n)
	for i := range l.ants {
		// A zero-sized board still has to produce placeable ants; step and
		// Frame both no-op on it.
		l.ants[i] = ant{dir: l.rng.Intn(4)}
		if w > 0 && h > 0 {
			l.ants[i].x = l.rng.Intn(w)
			l.ants[i].y = l.rng.Intn(h)
		}
		c := antColours[i%len(antColours)]
		// Each ramp runs from a very dark version of the hue to a washed-out
		// bright one, so intensity reads as recency while the hue stays the
		// signature of which ant did the work.
		l.pals[i] = canvas.NewPalette(
			canvas.Stop{At: 0.00, R: c[0] / 7, G: c[1] / 7, B: c[2] / 7},
			canvas.Stop{At: 0.60, R: c[0], G: c[1], B: c[2]},
			canvas.Stop{At: 1.00, R: (c[0] + 510) / 3, G: (c[1] + 510) / 3, B: (c[2] + 510) / 3},
		)
	}
}

// maxStepsPerFrame caps how much one frame will catch up by. At the default
// rate a fully clamped dt is 750 moves, so this is slack rather than a limit
// that normally bites; it exists so a caller's large StepsPerSecond cannot
// turn a single hitch into an unbounded burst.
const maxStepsPerFrame = 4000

// Frame walks the ants for dt seconds and draws the board.
func (l *Langton) Frame(s *canvas.Surface, dt float64) {
	l.advance(dt)
	for y := 0; y < l.h; y++ {
		row := y * l.w
		for x := 0; x < l.w; x++ {
			i := row + x
			owner := l.cell[i]
			if owner == 0 {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			s.Set(x, y, l.pals[int(owner-1)%len(l.pals)][l.intensity(i)])
		}
	}
}

// advance runs whole ant moves out of the elapsed time, carrying the leftover
// so a rate that does not divide into the frame rate still averages out.
func (l *Langton) advance(dt float64) {
	if l.StepsPerSecond <= 0 {
		return
	}
	interval := 1 / l.StepsPerSecond
	l.acc += dt
	for n := 0; l.acc >= interval; n++ {
		if n >= maxStepsPerFrame {
			// Too far behind to catch up; drop the backlog rather than let it
			// compound into ever longer frames.
			l.acc = 0
			return
		}
		l.step()
		l.acc -= interval
	}
}

// intensity maps how long ago a cell was flipped to a palette index.
func (l *Langton) intensity(i int) int {
	age := int(l.now-l.stamp[i]) / int(l.Fade)
	v := 255 - age
	if v < l.MinIntensity {
		v = l.MinIntensity
	}
	return v
}

// step moves every ant once.
func (l *Langton) step() {
	if l.w == 0 || l.h == 0 {
		return
	}
	l.now++
	for k := range l.ants {
		a := &l.ants[k]
		i := a.y*l.w + a.x
		if l.cell[i] == 0 {
			// White: turn right and leave the cell black, owned by this ant.
			a.dir = (a.dir + 1) & 3
			l.cell[i] = byte(k + 1)
		} else {
			// Black: turn left and leave the cell white. Note the ant does not
			// check who painted it — an ant happily erases another's work, and
			// that is where the interference between colonies comes from.
			a.dir = (a.dir + 3) & 3
			l.cell[i] = 0
		}
		l.stamp[i] = l.now

		a.x += stepX[a.dir]
		a.y += stepY[a.dir]
		if a.x < 0 {
			a.x = l.w - 1
		} else if a.x >= l.w {
			a.x = 0
		}
		if a.y < 0 {
			a.y = l.h - 1
		} else if a.y >= l.h {
			a.y = 0
		}
	}
}

// Run drives the ants on the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
