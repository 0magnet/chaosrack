// Package wolfram is the elementary cellular automata.
//
// One row of cells, each one alive or dead. To make the next row, look at
// every cell together with its two neighbors — three bits, eight possible
// arrangements — and read off whether the cell lives from a table of eight
// answers. Eight bits of table is the entire program, which is why there are
// exactly 256 of these and why they are numbered rather than named: the rule
// number *is* the table.
//
// Each new row is drawn along the bottom and the picture scrolls up, so the
// vertical axis is time and what is on screen is the automaton's own history.
// That is the only reason any of this is watchable. A single row of blinking
// cells says nothing; the same row stacked into a column shows the structure
// the rule generates, and the structure is the point.
//
// The rules worth watching, which is the set cycled through here:
//
//	 30  chaos out of one lit cell — the left half falls into stripes while
//	     the right half stays statistically random. It was Mathematica's
//	     random number generator for years, and it grows on the shell of the
//	     sea snail Conus textile.
//	 90  the Sierpinski triangle, exactly. The rule is the exclusive or of
//	     the two neighbors, so the lit cells of row n are the odd entries of
//	     row n of Pascal's triangle.
//	110  the interesting one. Started from noise it settles into a striped
//	     background carrying gliders that collide and interact, and those
//	     collisions were shown by Matthew Cook to be able to carry out any
//	     computation: this rule, with three bits of neighborhood and eight
//	     bits of table, is Turing complete.
//	150  the exclusive or of all three cells, which nests like 90 but denser.
//	 22  Sierpinski's ragged cousin — nested, but the triangles never close.
//	 54  gliders on a period-four background, the near miss to 110.
//	 60  half of 90: one neighbor only, so the triangle leans.
//	105  the complement of 150, which fills instead of thinning.
//
// The rows wrap at the edges, so a pattern that grows past the width of the
// window drives back into itself rather than being clipped. That interference
// is not in the textbook pictures, which are drawn on an unbounded line, and
// it is worth knowing you are looking at it.
//
// Written from the definition of the rules, not from an implementation of
// them. Stephen Wolfram's numbering and his catalog of which rules do what
// are the source of the choice of rules here.
package wolfram

import (
	"math/rand"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/termanim/canvas"
)

// RuleSpec is one entry in the cycle: a rule number and what to start it from.
//
// The start matters as much as the rule. Rule 90 from a single lit cell is the
// Sierpinski triangle and from noise is a gray hash; rule 110 is the other way
// round, dull from a single cell and full of colliding gliders from noise. A
// rule shown from the wrong start looks like nothing at all.
type RuleSpec struct {
	// Number is the rule, 0 to 255. Bit n of it is what a cell becomes when
	// its neighborhood, read left-center-right as a binary number, is n.
	Number uint8
	// Random starts from a row of noise rather than a single lit cell in the
	// middle.
	Random bool
	// Name is what the rule is known for, for whoever reads the list.
	Name string
}

// Interesting is the sequence of rules cycled through, in order. See the
// package comment for what each one does.
var Interesting = []RuleSpec{
	{Number: 30, Name: "chaos"},
	{Number: 90, Name: "Sierpinski"},
	{Number: 110, Random: true, Name: "Turing complete"},
	{Number: 150, Name: "nested"},
	{Number: 22, Name: "ragged nesting"},
	{Number: 54, Random: true, Name: "gliders"},
	{Number: 60, Name: "leaning triangle"},
	{Number: 105, Name: "filling"},
}

// ruleColors are the hues successive rules are drawn in. A rule change is a
// discontinuity in what the picture is doing, and giving each one its own
// color makes the boundary between them legible as the old work scrolls away
// above the new.
var ruleColors = [][3]int{
	{120, 255, 170},
	{255, 200, 96},
	{140, 190, 255},
	{255, 128, 176},
	{200, 255, 120},
	{255, 160, 96},
	{170, 150, 255},
	{96, 232, 232},
}

// Wolfram is the animation. The zero value is not usable; call New.
type Wolfram struct {
	w, h int

	// cells is the picture, one byte per pixel, as a ring of rows. Scrolling
	// by moving an index rather than by copying every row up one is not a
	// micro-optimization: at thirty rows a second on a full screen, copying
	// would be the most expensive thing in the package by a wide margin, and
	// it would allocate or memmove a whole surface for a one-row change.
	cells []byte
	// pal is which rule's color each row of the ring was drawn in, so rows
	// made by a rule that has since been replaced keep their own color as
	// they scroll away.
	pal []int
	// head is the ring index of the newest row, which is drawn at the bottom.
	head int

	// acc is unspent elapsed time. A row is discrete, so a frame emits a whole
	// number of them and carries the remainder, which keeps the scroll at the
	// same speed whatever the frame rate is.
	acc float64

	rows      int // rows emitted since Resize
	rowInRule int // rows emitted by the current spec
	specIdx   int

	pals []canvas.Palette
	rng  *rand.Rand

	// Rule is the elementary rule currently running, 0 to 255. Setting it
	// takes effect on the next row; it is overwritten when the cycle moves on
	// to the next spec, so to pin one rule forever set Rules to a single
	// entry instead.
	Rule uint8

	// Rules is the sequence cycled through, each entry running for RowsPerRule
	// rows before the next takes over and reseeds. A single-entry list runs
	// that one rule forever, restarting it whenever RowsPerRule is up.
	Rules []RuleSpec

	// RowsPerSecond is how fast the picture scrolls, in rows a second. It is a
	// rate in real time rather than a row a frame, so the same automaton
	// unfolds at the same speed on a terminal that cannot keep up.
	RowsPerSecond float64

	// RowsPerRule is how long each rule runs before the next one. Zero means
	// twice the height of the surface, which is long enough that a rule fills
	// the screen and is then seen scrolling away complete.
	RowsPerRule int

	// Density is the fraction of cells lit in a random start. A half-and-half
	// row is the usual choice and the one rule 110 was studied from.
	Density float64

	// MinIntensity is how dim the oldest row on screen is, from 0 to 255. The
	// newest row is always at full brightness: the leading edge is where the
	// rule is actually working, and letting it stand out of its own history is
	// what makes the picture read as something being generated rather than a
	// static texture.
	MinIntensity int
}

// New returns an automaton. seed of 0 gives a fixed noise row, which makes
// tests repeatable; the rules and their order do not depend on the seed.
func New(seed int64) *Wolfram {
	return &Wolfram{
		rng:           rand.New(rand.NewSource(seed)), //nolint:gosec
		Rules:         Interesting,
		Rule:          Interesting[0].Number,
		RowsPerSecond: 26,
		Density:       0.5,
		MinIntensity:  70,
	}
}

// Resize allocates the picture and starts the first rule. Called before the
// first frame and on every resize.
func (a *Wolfram) Resize(w, h int) {
	a.w, a.h = w, h
	a.cells = make([]byte, w*h)
	a.pal = make([]int, h)
	a.head = 0
	a.acc = 0
	a.rows, a.rowInRule, a.specIdx = 0, 0, 0

	if len(a.Rules) == 0 {
		a.Rules = Interesting
	}
	a.pals = make([]canvas.Palette, len(a.Rules))
	for i := range a.pals {
		c := ruleColors[i%len(ruleColors)]
		// Each ramp runs from a dark version of the hue up to a washed-out
		// bright one. Intensity carries age and hue carries which rule, so
		// neither has to be read off the other.
		a.pals[i] = canvas.NewPalette(
			canvas.Stop{At: 0.00, R: c[0] / 8, G: c[1] / 8, B: c[2] / 8},
			canvas.Stop{At: 0.65, R: c[0], G: c[1], B: c[2]},
			canvas.Stop{At: 1.00, R: (c[0] + 510) / 3, G: (c[1] + 510) / 3, B: (c[2] + 510) / 3},
		)
	}
	a.Rule = a.Rules[0].Number
	if w > 0 && h > 0 {
		a.seed(a.Rules[0])
	}
}

// rowsPerRule is RowsPerRule with the default filled in.
func (a *Wolfram) rowsPerRule() int {
	if a.RowsPerRule > 0 {
		return a.RowsPerRule
	}
	if a.h > 0 {
		return a.h * 2
	}
	return 1
}

// seed writes a fresh starting row over the newest row.
func (a *Wolfram) seed(spec RuleSpec) {
	row := a.cells[a.head*a.w : (a.head+1)*a.w]
	for i := range row {
		row[i] = 0
	}
	if spec.Random {
		for i := range row {
			if a.rng.Float64() < a.Density {
				row[i] = 1
			}
		}
	} else {
		row[a.w/2] = 1
	}
	a.pal[a.head] = a.specIdx
}

// maxRowsPerFrame caps how much one frame will catch up by. A fully clamped
// frame at the default rate is three rows, so this is slack; it exists so a
// large RowsPerSecond cannot turn a single hitch into an unbounded burst.
const maxRowsPerFrame = 512

// Frame emits rows for dt seconds and draws the picture.
func (a *Wolfram) Frame(s *canvas.Surface, dt float64) {
	// Two rows is the minimum the ring can be: at one, the row being read
	// and the row being written are the same memory.
	if a.w == 0 || a.h < 2 {
		return
	}
	if a.RowsPerSecond > 0 {
		interval := 1 / a.RowsPerSecond
		a.acc += dt
		for n := 0; a.acc >= interval; n++ {
			if n >= maxRowsPerFrame {
				a.acc = 0
				break
			}
			a.step()
			a.acc -= interval
		}
	}
	a.draw(s)
}

// step emits one row, switching to the next rule when the current one has had
// its turn.
func (a *Wolfram) step() {
	cur := a.cells[a.head*a.w : (a.head+1)*a.w]
	a.head = (a.head + 1) % a.h
	next := a.cells[a.head*a.w : (a.head+1)*a.w]

	a.rowInRule++
	if a.rowInRule >= a.rowsPerRule() {
		a.rowInRule = 0
		a.specIdx = (a.specIdx + 1) % len(a.Rules)
		a.Rule = a.Rules[a.specIdx].Number
		a.seed(a.Rules[a.specIdx])
		a.rows++
		return
	}

	rule := a.Rule
	w := a.w
	for x := 0; x < w; x++ {
		// The neighborhood read as a three-bit number, left the high bit, and
		// the rule shifted right by it. That is the definition of the rule
		// number and there is nothing else to the automaton.
		i := cur[(x-1+w)%w]<<2 | cur[x]<<1 | cur[(x+1)%w]
		next[x] = (rule >> i) & 1
	}
	a.pal[a.head] = a.specIdx
	a.rows++
}

// draw paints the ring onto the surface, newest row at the bottom.
func (a *Wolfram) draw(s *canvas.Surface) {
	span := 255 - a.MinIntensity
	if span < 0 {
		span = 0
	}
	den := a.h - 1
	if den < 1 {
		den = 1
	}
	for y := 0; y < a.h; y++ {
		// Age in rows is exactly the distance up from the bottom, so no
		// timestamp per row is needed: where a row is on screen is how old it
		// is.
		age := a.h - 1 - y
		v := 255 - age*span/den
		r := (a.head - age + a.h*2) % a.h
		row := a.cells[r*a.w : (r+1)*a.w]
		p := &a.pals[a.pal[r]%len(a.pals)]
		for x, c := range row {
			if c == 0 {
				s.Set(x, y, tcell.ColorDefault)
				continue
			}
			s.Set(x, y, p[v])
		}
	}
}

// Run scrolls elementary cellular automata up the screen until the user quits.
func Run(screen tcell.Screen, seed int64) error {
	return canvas.Run(screen, New(seed), canvas.Options{})
}
