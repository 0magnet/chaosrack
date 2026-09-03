package pisano

import (
	"fmt"
	"strings"
)

// Coloring a path.
//
// The first scheme was to tint by pass: first walk of the period in one color,
// second in the next. It reads well on a path that drifts, where each pass lands
// somewhere new. It reads badly on a closed one, which is where it matters most
// — the geometry there repeats every Shape.Periods passes and the palette every
// six, so the two run in lockstep and the figure returns to a coloring it has
// already been in after 6/gcd(6, Periods) circuits. For the Fibonacci moduli up
// to 200 that close at all, Periods is 1, 2 or 4, so all but thirteen of them
// only ever show three colorings, and a figure being walked again did not
// reliably look like anything was happening.
//
// A color can carry more than that. The walk knows, for every step it takes,
// which way the turtle was facing, which way it turned, which term of the
// sequence moved it, how far into the circuit it is, and whether it has been
// down that piece of path before and in which direction. Each of those is a
// different question about the same figure, and each makes a different thing
// visible, so each is a mode.

// TintMode says what a color means.
type TintMode int

const (
	// TintStep colors a step by what the path does to it: the pass that first
	// walks it, then forward through the palette each time it is walked again
	// the same way, and back each time it is walked against that. A closed
	// figure retraced identically steps through the whole palette, one color
	// per circuit, whatever Periods is; a path that doubles back shows where,
	// because those steps move the other way while their neighbors move
	// forward.
	TintStep TintMode = iota

	// TintPass colors a step by which walk of the period laid it down. The
	// plainest reading, and the right one when what you want to see is the
	// passes themselves — an open path drifting one motif at a time.
	TintPass

	// TintVisits colors a step by how many times it has been walked, counting
	// both directions. It answers "how much of this figure is drawn twice?",
	// which for these paths is usually most of it.
	TintVisits

	// TintHeading colors a step by the direction the turtle was facing: four
	// colors, one per compass point. It makes the fourfold symmetry of a path
	// that closes on a quarter turn immediate, and shows an open path's motif
	// repeating in the same orientation each pass.
	TintHeading

	// TintTurn colors a step by the turn that produced it — left or right,
	// which is odd or even. Two colors, and the least interpreted view there
	// is: it draws the arithmetic itself, the sequence of parities, onto the
	// figure.
	TintTurn

	// TintTerm colors a step by the residue that produced it. The figure is
	// made of the reduced sequence, and this is the only mode that shows the
	// values rather than what they did.
	TintTerm

	// TintAge colors a step by how far into the circuit it is, sweeping the
	// palette once per circuit. It shows the order the figure was drawn in,
	// which nothing else here does — and in the viewer it becomes a band of
	// color traveling around a closed path.
	TintAge
)

// tintNames is the flag vocabulary, in the order --help should list them.
var tintNames = []struct {
	name string
	mode TintMode
}{
	{"step", TintStep},
	{"pass", TintPass},
	{"visits", TintVisits},
	{"heading", TintHeading},
	{"turn", TintTurn},
	{"term", TintTerm},
	{"age", TintAge},
}

// TintNames lists the modes for a flag description.
func TintNames() string {
	out := make([]string, len(tintNames))
	for i, t := range tintNames {
		out[i] = t.name
	}
	return strings.Join(out, ", ")
}

// TintModes is every mode, for a viewer that cycles them.
func TintModes() []TintMode {
	out := make([]TintMode, len(tintNames))
	for i, t := range tintNames {
		out[i] = t.mode
	}
	return out
}

// ParseTint reads the flag.
func ParseTint(s string) (TintMode, error) {
	if s == "" {
		return TintStep, nil
	}
	for _, t := range tintNames {
		if t.name == s {
			return t.mode, nil
		}
	}
	return TintStep, fmt.Errorf("unknown tint %q: %s", s, TintNames())
}

func (t TintMode) String() string {
	for _, e := range tintNames {
		if e.mode == t {
			return e.name
		}
	}
	return "step"
}

// segKey identifies a unit step between two lattice points, without regard to
// which way it was walked: the two directions are the same piece of path, which
// is the whole point.
//
// It names the lower of the two endpoints and the axis the step runs along, so
// it covers a walk in the plane and one in space equally — a plane walk is one
// with z always zero and the axis never 2.
type segKey struct {
	x, y, z int
	axis    uint8 // 0 = x, 1 = y, 2 = z
}

type segState struct {
	dir    int // +1 or -1, the way it was last walked
	idx    int // the color it was last given
	visits int
}

// Tinter assigns a color index to each step of a walk.
//
// It has memory because two of the modes need it: what color a step gets can
// depend on everything walked before it. One belongs to one walk; a second walk
// of the same figure wants a fresh one, or it carries the first walk's colors
// in.
type Tinter struct {
	mode TintMode
	n    int // palette size
	span int // steps per sweep of the palette, for TintAge
	seen map[segKey]segState
}

// NewTinter makes a tinter for a palette of n colors. span is how many steps a
// full sweep of the palette covers in TintAge — one circuit of the figure, so
// that the band of color goes round once per lap.
func NewTinter(mode TintMode, n, span int) *Tinter {
	t := &Tinter{mode: mode, n: max(n, 1), span: max(span, 1)}
	if mode == TintStep || mode == TintVisits {
		t.seen = map[segKey]segState{}
	}
	return t
}

// tintStep is everything a tint mode can ask about one move, whether the walk was
// in the plane or in space. Gathering it in one place is what lets the seven
// modes mean the same thing in both.
type tintStep struct {
	key     segKey
	dir     int  // which way along key it was walked
	unit    bool // whether it was a unit step at all, so key means anything
	term    int
	pass    int
	heading int // the direction it moved in: 4 of them in the plane, 6 in space
	index   int
}

// Tint is the color index for one step, already folded into the palette, or -1
// for a step that should be left uncolored.
func (t *Tinter) Tint(s Step) int {
	key, dir, unit := stepKey(s.From, s.To)
	return t.tint(tintStep{key: key, dir: dir, unit: unit,
		term: s.Term, pass: s.Pass, heading: s.Dir, index: s.Index})
}

// Tint3 is Tint for a walk in space. Every mode carries over unchanged except
// heading, which has six directions to name instead of four.
func (t *Tinter) Tint3(s Step3) int {
	key, dir, unit := stepKey3(s.From, s.To)
	return t.tint(tintStep{key: key, dir: dir, unit: unit,
		term: s.Term, pass: s.Pass, heading: heading3(s.Dir.H), index: s.Index})
}

func (t *Tinter) tint(s tintStep) int {
	switch t.mode {
	case TintPass:
		if s.pass < 0 {
			return -1 // run-in: not part of any pass, so not part of the figure
		}
		return TintIndex(s.pass, t.n)

	case TintStep:
		idx, color := t.record(s)
		if !color {
			return -1
		}
		return TintIndex(idx, t.n)

	case TintVisits:
		return TintIndex(t.visits(s)-1, t.n)

	case TintHeading:
		return TintIndex(s.heading, t.n)

	case TintTurn:
		// Odd turns left, even turns right; a zero term never produced a step.
		if s.term%2 != 0 {
			return TintIndex(0, t.n)
		}
		return TintIndex(t.n/2, t.n)

	case TintTerm:
		return TintIndex(s.term, t.n)

	case TintAge:
		return TintIndex(s.index*t.n/t.span, t.n)
	}
	return 0
}

// heading3 numbers the six directions a turtle in space can face, so that
// opposite directions are not the same color: +x, -x, +y, -y, +z, -z.
func heading3(h Pt3) int {
	switch {
	case h.X > 0:
		return 0
	case h.X < 0:
		return 1
	case h.Y > 0:
		return 2
	case h.Y < 0:
		return 3
	case h.Z > 0:
		return 4
	}
	return 5
}

// record is TintStep's counter: forward a color for a step walked again the
// same way, back one for a step walked against the way it went before. It
// reports whether the step should be colored at all, which is separate from
// the counter being negative — a counter runs backwards perfectly happily, and
// only the run-in goes uncolored.
func (t *Tinter) record(s tintStep) (int, bool) {
	if !s.unit {
		return s.pass, s.pass >= 0 // not a unit step; nothing to remember
	}
	if st, seen := t.seen[s.key]; seen {
		idx := st.idx + 1
		if s.dir != st.dir {
			idx = st.idx - 1
		}
		t.seen[s.key] = segState{dir: s.dir, idx: idx, visits: st.visits + 1}
		return idx, true
	}
	// The first walk of a step takes the color of the pass that walked it, so
	// a figure with nothing retraced comes out tinted by pass.
	t.seen[s.key] = segState{dir: s.dir, idx: max(s.pass, 0), visits: 1}
	return s.pass, s.pass >= 0
}

// visits counts how many times a step has been walked, this one included.
func (t *Tinter) visits(s tintStep) int {
	if !s.unit {
		return 1
	}
	st := t.seen[s.key]
	st.visits++
	st.dir = s.dir
	t.seen[s.key] = st
	return st.visits
}

// Len is how many distinct steps have been remembered, which bounds what a walk
// costs: the size of the figure for a closed path however long it is watched,
// and the length of the path for an open one.
func (t *Tinter) Len() int { return len(t.seen) }

// stepKey names the piece of path between two adjacent lattice points, and
// which way it was walked. Both endpoints name the same key, so walking it back
// is recognized as walking the same step.
func stepKey(a, b Pt) (segKey, int, bool) {
	return stepKey3(Pt3{a.X, a.Y, 0}, Pt3{b.X, b.Y, 0})
}

// stepKey3 is stepKey in space. The plane is the case where nothing ever moves
// along z, so the one function serves both.
func stepKey3(a, b Pt3) (segKey, int, bool) {
	switch {
	case a.Y == b.Y && a.Z == b.Z && abs(a.X-b.X) == 1:
		return segKey{x: min(a.X, b.X), y: a.Y, z: a.Z, axis: 0}, sign(b.X - a.X), true
	case a.X == b.X && a.Z == b.Z && abs(a.Y-b.Y) == 1:
		return segKey{x: a.X, y: min(a.Y, b.Y), z: a.Z, axis: 1}, sign(b.Y - a.Y), true
	case a.X == b.X && a.Y == b.Y && abs(a.Z-b.Z) == 1:
		return segKey{x: a.X, y: a.Y, z: min(a.Z, b.Z), axis: 2}, sign(b.Z - a.Z), true
	}
	return segKey{}, 0, false
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}

// TintIndex folds a color index into a palette of n, so that going backwards
// past the start of the palette arrives at the end of it.
func TintIndex(idx, n int) int {
	if n <= 0 {
		return 0
	}
	return ((idx % n) + n) % n
}
