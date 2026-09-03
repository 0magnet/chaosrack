// Package maze carves a maze and then solves it, both in view.
//
// Written from the technique rather than from any implementation of it.
//
// Carving is recursive backtracking, which is depth-first search with the
// visited set doing double duty as the maze: stand on a cell, pick an unvisited
// neighbour at random, knock down the wall between them and move there; when
// there is nowhere left to go, pop back down the stack and try again from an
// earlier cell. Every cell is entered exactly once, so the result is a perfect
// maze — one region, no loops, exactly one route between any two cells.
//
// Solving is breadth-first from one corner to the other, which finds the
// shortest route and, being breadth-first, spreads out from the start in a way
// that is worth watching. The route is then retraced from the start so the
// answer is drawn rather than merely appearing.
//
// This is a pixel animation: a maze is walls and corridors, not characters, and
// the half-block surface gives square pixels to draw them with. A cell is two
// pixels by two, with one pixel of wall between neighbours, so a cell costs
// three pixels of pitch and the grid ends on a closing wall.
package maze

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// pitch is the pixels one cell costs: two of corridor and one of wall. The
// whole grid is pitch*cells+1 pixels across, the +1 being the closing wall on
// the far side.
const pitch = 3

// The four directions, as bits in a cell's open set.
const (
	north = iota
	east
	south
	west
)

var step = [4][2]int{
	north: {0, -1},
	east:  {1, 0},
	south: {0, 1},
	west:  {-1, 0},
}

// back is the direction that undoes d. Knocking down a wall has to be recorded
// in both cells or the two disagree about whether there is a door there.
func back(d int) int { return (d + 2) % 4 }

// A cell is in exactly one of these states, and they are ordered: where two
// cells meet, the passage between them is drawn in the colour of the lesser of
// the two, so the solved route lights up its own corridors and nothing else.
const (
	unvisited uint8 = iota
	carved
	explored
	onPath
)

// phase is what the animation is doing.
type phase int

const (
	generating phase = iota
	solving
	tracing
	holding
)

// Maze is the animation. The zero value is not usable; call New.
type Maze struct {
	cw, ch int // the maze in cells
	ox, oy int // pixel offset that centres the grid in the surface

	open  []uint8 // which walls are down, one bit per direction
	state []uint8
	stack []int // the backtracking stack, in cell indices
	head  int   // the cell being carved from, or the last one popped

	queue []int // breadth-first frontier, used as a ring-free FIFO
	qhead int
	came  []int // came[c] is the cell the solver reached c from
	path  []int
	trace int // how much of the path has been drawn

	phase phase
	// hold is how long the solved maze has been up, in seconds.
	hold float64
	// acc is unspent time, carried between frames so the carving runs at the
	// same rate whatever the frame rate is.
	acc    float64
	carved int
	// mazes counts the mazes generated, so one run can be told from the next.
	mazes int
	rng   *rand.Rand

	// The three stage rates, in steps per second, derived from the durations
	// below and the size of the maze.
	genRate, solveRate, traceRate float64

	// GenSeconds, SolveSeconds and TraceSeconds are how long each stage should
	// take. The steps per second are derived from them and the size of the
	// maze, so a large window carves faster rather than taking minutes: the
	// animation lasts about as long whatever it is drawn on. Saying that in
	// seconds rather than in frames is what makes it true of any frame rate.
	GenSeconds, SolveSeconds, TraceSeconds float64
	// HoldSeconds is how long the solved maze is left up before the next one.
	HoldSeconds float64

	// Wall is the colour of everything not carved out yet — both the walls and
	// the ground the digger has not reached.
	Wall tcell.Color
	// Corridor is carved but unexplored floor. Dim: it is the backdrop the
	// bright parts are read against.
	Corridor tcell.Color
	// Head is the cell being carved right now, or the tip of the route being
	// traced. Bright, because following it is the whole point of animating this.
	Head tcell.Color
	// Explored is floor the solver has looked at.
	Explored tcell.Color
	// Path is the route from corner to corner.
	Path tcell.Color
}

// New returns a maze animation. seed of 0 gives a fixed maze, which makes tests
// repeatable.
func New(seed int64) *Maze {
	return &Maze{
		rng:          rand.New(rand.NewSource(seed)),
		GenSeconds:   5,
		SolveSeconds: 3,
		TraceSeconds: 1.5,
		HoldSeconds:  3,
		Wall:         tcell.NewRGBColor(28, 32, 54),
		Corridor:     tcell.NewRGBColor(70, 78, 110),
		Head:         tcell.NewRGBColor(255, 255, 255),
		Explored:     tcell.NewRGBColor(60, 130, 180),
		Path:         tcell.NewRGBColor(255, 210, 70),
	}
}

// Resize sizes the grid to the surface and starts a new maze.
func (m *Maze) Resize(w, h int) {
	// Whole cells only, and the grid is centred: a maze with a ragged strip of
	// nothing down one side looks like a drawing bug rather than a margin.
	m.cw = (w - 1) / pitch
	m.ch = (h - 1) / pitch
	if m.cw < 1 || m.ch < 1 {
		m.cw, m.ch = 0, 0
		return
	}
	m.ox = (w - (m.cw*pitch + 1)) / 2
	m.oy = (h - (m.ch*pitch + 1)) / 2

	n := m.cw * m.ch
	m.open = make([]uint8, n)
	m.state = make([]uint8, n)
	m.stack = make([]int, 0, n)
	m.queue = make([]int, 0, n)
	m.came = make([]int, n)
	m.path = make([]int, 0, n)

	// Pace each stage by the size of the maze. Carving touches every cell twice,
	// once on the way in and once on the way back out.
	m.genRate = rate(2*n, m.GenSeconds)
	m.solveRate = rate(n, m.SolveSeconds)

	m.restart()
}

// rate spreads work over a number of seconds, in steps per second. It never
// returns nothing, or a stage with no work in it would never end.
func rate(work int, seconds float64) float64 {
	if seconds <= 0 {
		seconds = 1
	}
	r := float64(work) / seconds
	if r < 1 {
		r = 1
	}
	return r
}

// maxBacklog is the most unspent time the accumulator will hold, in seconds. A
// frame arriving after a long stall must not carve the whole maze at once.
const maxBacklog = 0.25

// due consumes whole steps' worth of time from the accumulator at the given
// rate. The accumulator is shared by the stages: they never run at once, and
// carrying the remainder across a stage change is what stops each change of
// stage from losing a fraction of a step.
func (m *Maze) due(dt, rate float64) int {
	if rate <= 0 {
		rate = 1
	}
	m.acc += dt
	if m.acc > maxBacklog {
		m.acc = maxBacklog
	}
	n := int(m.acc * rate)
	m.acc -= float64(n) / rate
	return n
}

// restart wipes the grid and begins carving a new maze from the top left.
func (m *Maze) restart() {
	for i := range m.open {
		m.open[i] = 0
		m.state[i] = unvisited
		m.came[i] = -1
	}
	m.stack = m.stack[:0]
	m.queue = m.queue[:0]
	m.path = m.path[:0]
	m.qhead, m.trace = 0, 0
	m.hold = 0
	m.carved = 0
	m.phase = generating
	m.mazes++

	m.state[0] = carved
	m.carved = 1
	m.head = 0
	m.stack = append(m.stack, 0)
}

func (m *Maze) cell(cx, cy int) int { return cy*m.cw + cx }

// neighbour returns the cell one step in direction d, and whether it exists.
func (m *Maze) neighbour(c, d int) (int, bool) {
	cx, cy := c%m.cw, c/m.cw
	nx, ny := cx+step[d][0], cy+step[d][1]
	if nx < 0 || ny < 0 || nx >= m.cw || ny >= m.ch {
		return 0, false
	}
	return m.cell(nx, ny), true
}

// carveStep is one move of the depth-first carve.
func (m *Maze) carveStep() {
	if len(m.stack) == 0 {
		m.beginSolve()
		return
	}
	c := m.stack[len(m.stack)-1]
	m.head = c

	// Collect the neighbours that have not been dug into yet. Choosing among
	// them uniformly is what makes the corridors wander; always taking the first
	// gives long combs of parallel passages.
	var choice [4]int
	n := 0
	for d := 0; d < 4; d++ {
		if nb, ok := m.neighbour(c, d); ok && m.state[nb] == unvisited {
			choice[n] = d
			n++
		}
	}
	if n == 0 {
		// Dead end: back down the stack. Backtracking is drawn too, which is
		// why the head appears to retreat and try again.
		m.stack = m.stack[:len(m.stack)-1]
		return
	}
	d := choice[m.rng.Intn(n)]
	nb, _ := m.neighbour(c, d)
	m.open[c] |= 1 << uint(d)
	m.open[nb] |= 1 << uint(back(d))
	m.state[nb] = carved
	m.carved++
	m.stack = append(m.stack, nb)
	m.head = nb
}

// beginSolve switches to breadth-first search from the top left corner.
func (m *Maze) beginSolve() {
	m.phase = solving
	m.queue = append(m.queue[:0], 0)
	m.qhead = 0
	for i := range m.came {
		m.came[i] = -1
	}
	m.state[0] = explored
	m.head = 0
}

// solveStep expands one cell of the frontier.
func (m *Maze) solveStep() {
	goal := m.cw*m.ch - 1
	if m.qhead >= len(m.queue) {
		// Cannot happen in a perfect maze — every cell is reachable — but a
		// search that runs dry must not spin forever.
		m.beginTrace(goal)
		return
	}
	c := m.queue[m.qhead]
	m.qhead++
	m.head = c
	if c == goal {
		m.beginTrace(goal)
		return
	}
	for d := 0; d < 4; d++ {
		if m.open[c]&(1<<uint(d)) == 0 {
			continue // there is a wall this way
		}
		nb, ok := m.neighbour(c, d)
		if !ok || m.state[nb] == explored {
			continue
		}
		m.state[nb] = explored
		m.came[nb] = c
		m.queue = append(m.queue, nb)
	}
}

// beginTrace walks the came-from chain back from the goal and stores the route
// start first, so it can be drawn in the direction it would be walked.
func (m *Maze) beginTrace(goal int) {
	m.path = m.path[:0]
	for c := goal; c >= 0; c = m.came[c] {
		m.path = append(m.path, c)
		if c == 0 {
			break
		}
	}
	// Reverse in place: the chain was followed from the goal backwards.
	for i, j := 0, len(m.path)-1; i < j; i, j = i+1, j-1 {
		m.path[i], m.path[j] = m.path[j], m.path[i]
	}
	m.traceRate = rate(len(m.path), m.TraceSeconds)
	m.trace = 0
	m.phase = tracing
}

// traceStep reveals one more cell of the route.
func (m *Maze) traceStep() {
	if m.trace >= len(m.path) {
		m.phase = holding
		m.hold = 0
		return
	}
	c := m.path[m.trace]
	m.state[c] = onPath
	m.head = c
	m.trace++
}

// Frame advances the maze by whatever the elapsed time is worth and draws it.
// dt is seconds since the last frame.
func (m *Maze) Frame(s *canvas.Surface, dt float64) {
	if m.cw == 0 || m.ch == 0 {
		s.Clear()
		return
	}
	switch m.phase {
	case generating:
		for n := m.due(dt, m.genRate); n > 0 && m.phase == generating; n-- {
			m.carveStep()
		}
	case solving:
		for n := m.due(dt, m.solveRate); n > 0 && m.phase == solving; n-- {
			m.solveStep()
		}
	case tracing:
		for n := m.due(dt, m.traceRate); n > 0 && m.phase == tracing; n-- {
			m.traceStep()
		}
	case holding:
		m.hold += dt
		if m.hold > m.HoldSeconds {
			m.restart()
		}
	}
	m.draw(s)
}

// colourFor is the colour a cell in this state is drawn in.
func (m *Maze) colourFor(st uint8) tcell.Color {
	switch st {
	case carved:
		return m.Corridor
	case explored:
		return m.Explored
	case onPath:
		return m.Path
	}
	return m.Wall
}

// fill paints a rectangle of pixels.
func fill(s *canvas.Surface, x, y, w, h int, c tcell.Color) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			s.Set(x+dx, y+dy, c)
		}
	}
}

func (m *Maze) draw(s *canvas.Surface) {
	s.Clear()
	// Solid stone, then dig the corridors out of it. Drawing it the other way —
	// corridors on a blank background with walls stroked in — needs the walls
	// worked out cell by cell and gets the corners wrong.
	fill(s, m.ox, m.oy, m.cw*pitch+1, m.ch*pitch+1, m.Wall)

	for cy := 0; cy < m.ch; cy++ {
		for cx := 0; cx < m.cw; cx++ {
			c := m.cell(cx, cy)
			st := m.state[c]
			if st == unvisited {
				continue
			}
			x0 := m.ox + 1 + cx*pitch
			y0 := m.oy + 1 + cy*pitch
			fill(s, x0, y0, 2, 2, m.colourFor(st))

			// Open walls become two-pixel doorways joining the cells either
			// side. Only east and south are drawn, since the neighbour draws the
			// other half of every pair and doing both would paint each doorway
			// twice.
			if m.open[c]&(1<<east) != 0 {
				if nb, ok := m.neighbour(c, east); ok {
					fill(s, x0+2, y0, 1, 2, m.colourFor(min(st, m.state[nb])))
				}
			}
			if m.open[c]&(1<<south) != 0 {
				if nb, ok := m.neighbour(c, south); ok {
					fill(s, x0, y0+2, 2, 1, m.colourFor(min(st, m.state[nb])))
				}
			}
		}
	}

	// The head last, over everything, so it is never hidden by a doorway drawn
	// after it.
	if m.phase != holding {
		hx := m.ox + 1 + (m.head%m.cw)*pitch
		hy := m.oy + 1 + (m.head/m.cw)*pitch
		fill(s, hx, hy, 2, 2, m.Head)
	}
}

// Run carves and solves mazes until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
