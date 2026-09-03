// Package pipes is the pipes screensaver: coloured runs of box drawing that
// crawl across the terminal, turn corners, and eventually fill it.
//
// Written from the effect rather than from any implementation of it — pipes.sh
// and its relatives were not consulted, and nothing here is derived from them.
//
// The effect is a random walk that leaves a trail, and the whole of it is one
// lookup. A pipe enters a cell travelling in some direction and leaves it
// travelling in another; the glyph that belongs in that cell is the one joining
// the side it came in through to the side it went out by. Get that table wrong
// and every corner is a visible break in the pipe, which is the one thing the
// eye notices immediately.
package pipes

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// Dir is a direction of travel on the cell grid.
type Dir int

// The four directions. Up is toward row zero, as on the screen.
const (
	Up Dir = iota
	Right
	Down
	Left
)

// delta is the step each direction takes, indexed by Dir.
var delta = [4][2]int{
	Up:    {0, -1},
	Right: {1, 0},
	Down:  {0, 1},
	Left:  {-1, 0},
}

// opposite is the side a pipe travelling this way entered through. A pipe
// moving Right came in through the Left side of the cell it is standing on;
// that is the fact the corner table is built from.
func (d Dir) opposite() Dir { return (d + 2) % 4 }

// turn returns the direction ninety degrees to one side. clockwise picks which.
func (d Dir) turn(clockwise bool) Dir {
	if clockwise {
		return (d + 1) % 4
	}
	return (d + 3) % 4
}

// Set is one family of box-drawing glyphs. The elbows are named for the two
// sides of the cell they connect, which is exactly how they are chosen: an
// elbow joining the Up side to the Right side is UpRight, └.
type Set struct {
	Horizontal rune // ─ joins Left and Right
	Vertical   rune // │ joins Up and Down
	UpRight    rune // └
	UpLeft     rune // ┘
	DownRight  rune // ┌
	DownLeft   rune // ┐
}

// Light and Heavy are the two weights of box drawing. Both are in every
// terminal font worth using, and mixing them lets thin and thick pipes share a
// screen without either looking like a rendering accident.
var (
	Light = Set{Horizontal: '─', Vertical: '│', UpRight: '└', UpLeft: '┘', DownRight: '┌', DownLeft: '┐'}
	Heavy = Set{Horizontal: '━', Vertical: '┃', UpRight: '┗', UpLeft: '┛', DownRight: '┏', DownLeft: '┓'}
)

// sides is the set of cell sides a glyph connects, one bit per Dir.
func sides(a, b Dir) uint8 { return 1<<uint(a) | 1<<uint(b) }

// glyph returns the character for a cell that was entered travelling in and
// left travelling out.
//
// The cell connects two sides: the one the pipe came in through, which is the
// opposite of in, and the one it left by, which is out. Reducing the pair to a
// set of sides means the table needs six entries rather than sixteen, and it
// cannot disagree with itself about, say, a pipe going up then right versus one
// going left then down — both occupy the same corner and both must draw └.
func (s Set) glyph(in, out Dir) rune {
	switch sides(in.opposite(), out) {
	case sides(Up, Down):
		return s.Vertical
	case sides(Left, Right):
		return s.Horizontal
	case sides(Up, Right):
		return s.UpRight
	case sides(Up, Left):
		return s.UpLeft
	case sides(Down, Right):
		return s.DownRight
	case sides(Down, Left):
		return s.DownLeft
	}
	// Both sides are the same, which is a pipe doubling back on itself. Nothing
	// here generates that; if something ever does, draw a straight piece on the
	// axis it is leaving by rather than a blank.
	if out == Up || out == Down {
		return s.Vertical
	}
	return s.Horizontal
}

// Colours are the hues a pipe can take. They are bright and well separated so
// that two pipes crossing are still telling apart.
var Colours = []tcell.Color{
	tcell.NewRGBColor(255, 90, 90),
	tcell.NewRGBColor(90, 255, 130),
	tcell.NewRGBColor(110, 170, 255),
	tcell.NewRGBColor(255, 210, 80),
	tcell.NewRGBColor(210, 130, 255),
	tcell.NewRGBColor(120, 240, 240),
	tcell.NewRGBColor(255, 160, 60),
}

// pipe is one growing run.
type pipe struct {
	x, y   int
	dir    Dir
	set    Set
	colour tcell.Color
}

// cell is what has been painted at one position.
type cell struct {
	r      rune
	colour tcell.Color
}

// Pipes is the animation. The zero value is not usable; call New.
type Pipes struct {
	cols, rows int
	// buf is the painted screen. Pipes accumulate rather than being redrawn
	// from their history, so the trail has to live somewhere; it also makes the
	// fill count exact instead of a guess.
	buf    []cell
	pipe   []pipe
	filled int
	// acc is unspent time, in seconds. Pipes grow by whole cells at a fixed
	// rate; whatever is left over after the last whole step is carried into the
	// next frame rather than thrown away, which is what keeps the speed the
	// same however often frames arrive.
	acc float64
	// cleared counts the times the screen has been wiped and started over.
	cleared int
	rng     *rand.Rand

	// Count is how many pipes grow at once. Zero scales with the window, which
	// keeps a large terminal from taking minutes to fill.
	Count int
	// StepsPerSecond is how many cells each pipe grows per second. Twenty is a
	// crawl the eye can follow along; it is deliberately not tied to the frame
	// rate, which is free to be three times that for the sake of smoothness.
	StepsPerSecond float64
	// TurnChance is the chance per step of turning a corner, as a reciprocal:
	// 7 means roughly one step in seven. Much lower and the pipes are a maze of
	// stubs; much higher and they are straight lines that never bend.
	TurnChance int
	// ColourChance is the chance of changing colour at a corner, as a
	// reciprocal. Changing only at corners is what makes it read as a joint in
	// the plumbing rather than as the pipe flickering.
	ColourChance int
	// FillFraction is how much of the screen must be covered before it is wiped
	// and started again. Pipes cross their own trails constantly, so waiting for
	// the whole screen would take far longer than it looks like it should.
	FillFraction float64
	// Sets are the glyph weights a new pipe may be drawn in.
	Sets []Set
	// Colours are the hues a new pipe may take.
	Colours []tcell.Color
}

// New returns a pipes animation. seed of 0 gives a fixed sequence, which makes
// tests repeatable.
func New(seed int64) *Pipes {
	return &Pipes{
		rng:            rand.New(rand.NewSource(seed)),
		StepsPerSecond: 20,
		TurnChance:     7,
		ColourChance:   3,
		FillFraction:   0.55,
		Sets:           []Set{Light, Heavy},
		Colours:        Colours,
	}
}

// Resize allocates the painted screen and starts a fresh set of pipes.
func (p *Pipes) Resize(cols, rows int) {
	p.cols, p.rows = cols, rows
	p.buf = make([]cell, cols*rows)
	p.reset()
}

// count is the number of pipes to run in this window.
func (p *Pipes) count() int {
	if p.Count > 0 {
		return p.Count
	}
	// One pipe per six hundred cells, which fills an eighty by twenty-four
	// terminal at about the same rate as a large one.
	n := p.cols * p.rows / 600
	if n < 3 {
		n = 3
	}
	return n
}

// reset wipes the screen and respawns every pipe.
func (p *Pipes) reset() {
	for i := range p.buf {
		p.buf[i] = cell{}
	}
	p.filled = 0
	p.pipe = make([]pipe, p.count())
	for i := range p.pipe {
		// The first pipes start anywhere, not at an edge: a screensaver that
		// always begins with everything marching in from the border announces
		// its own restart.
		p.pipe[i] = p.newPipe(true)
	}
}

func (p *Pipes) newPipe(anywhere bool) pipe {
	q := pipe{
		dir:    Dir(p.rng.Intn(4)),
		set:    p.Sets[p.rng.Intn(len(p.Sets))],
		colour: p.Colours[p.rng.Intn(len(p.Colours))],
	}
	if anywhere {
		q.x, q.y = p.rng.Intn(p.cols), p.rng.Intn(p.rows)
		return q
	}
	// Otherwise enter from the edge behind the direction of travel, so the pipe
	// walks across the screen rather than immediately back off it.
	switch q.dir {
	case Up:
		q.x, q.y = p.rng.Intn(p.cols), p.rows-1
	case Down:
		q.x, q.y = p.rng.Intn(p.cols), 0
	case Right:
		q.x, q.y = 0, p.rng.Intn(p.rows)
	case Left:
		q.x, q.y = p.cols-1, p.rng.Intn(p.rows)
	}
	return q
}

// put paints one cell, keeping the fill count honest.
func (p *Pipes) put(x, y int, r rune, c tcell.Color) {
	if x < 0 || y < 0 || x >= p.cols || y >= p.rows {
		return
	}
	i := y*p.cols + x
	if p.buf[i].r == 0 {
		p.filled++
	}
	p.buf[i] = cell{r: r, colour: c}
}

// step draws the cell the pipe is standing on and moves it one cell on.
func (p *Pipes) step(q *pipe) {
	out := q.dir
	if p.rng.Intn(p.TurnChance) == 0 {
		out = q.dir.turn(p.rng.Intn(2) == 0)
		if p.rng.Intn(p.ColourChance) == 0 {
			q.colour = p.Colours[p.rng.Intn(len(p.Colours))]
		}
	}
	p.put(q.x, q.y, q.set.glyph(q.dir, out), q.colour)
	q.dir = out
	q.x += delta[out][0]
	q.y += delta[out][1]
	if q.x < 0 || q.y < 0 || q.x >= p.cols || q.y >= p.rows {
		*q = p.newPipe(false)
	}
}

// maxBacklog is the most unspent time the accumulator will hold, in seconds.
//
// A frame that arrives after a long stall — a backgrounded tab, a machine that
// hung — must not be paid back by growing every pipe across the screen in one
// go. Dropping the backlog costs a moment of drift and keeps the work done in
// any one frame bounded.
const maxBacklog = 0.25

// due consumes whole steps' worth of time from the accumulator.
func (p *Pipes) due(dt float64) int {
	rate := p.StepsPerSecond
	if rate <= 0 {
		rate = 20
	}
	p.acc += dt
	if p.acc > maxBacklog {
		p.acc = maxBacklog
	}
	n := int(p.acc * rate)
	p.acc -= float64(n) / rate
	return n
}

// Frame grows every pipe by the cells the elapsed time is worth and repaints.
// dt is seconds since the last frame.
func (p *Pipes) Frame(screen tcell.Screen, cols, rows int, dt float64) {
	if p.cols == 0 || p.rows == 0 {
		return
	}
	for n := p.due(dt); n > 0; n-- {
		// The wipe is a threshold on how much is painted, not something that
		// happens at a rate, so it is tested per step rather than per frame:
		// at sixty frames a second several steps can fall in one frame and the
		// screen should not be allowed to overfill between them.
		if float64(p.filled) >= p.FillFraction*float64(p.cols*p.rows) {
			p.cleared++
			p.reset()
		}
		for i := range p.pipe {
			p.step(&p.pipe[i])
		}
	}
	p.draw(screen)
}

func (p *Pipes) draw(screen tcell.Screen) {
	for y := 0; y < p.rows; y++ {
		for x := 0; x < p.cols; x++ {
			c := p.buf[y*p.cols+x]
			if c.r == 0 {
				screen.Put(x, y, canvas.Blank, tcell.StyleDefault) //nolint:errcheck // one cell cannot fail
				continue
			}
			canvas.PutRune(screen, x, y, c.r, tcell.StyleDefault.Foreground(c.colour))
		}
	}
}

// Run grows pipes across the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.RunCells(screen, New(seed), canvas.Options{})
}
