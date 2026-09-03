package pisano

import "fmt"

// The turtle reads each term as one of three things, and these are the values
// an unreduced sequence is presented as. They are chosen so Turtle.Step needs
// no special case: 1 is odd and turns left, 2 is even and turns right, 0 stays.
const (
	MoveStay  = 0
	MoveLeft  = 1
	MoveRight = 2
)

// Unreduced presents a sequence with no modulus applied.
//
// This is not the same as reducing modulo 2, and the difference is the whole
// reason it exists. Mod 2 sends every even term to the residue 0, which the
// turtle reads as "stay"; unreduced, only a term that is genuinely zero stays
// and every other even term turns right. The Fibonacci numbers open
// 0, 1, 1, 2, 3, 5, 8 — even, odd, odd, even — so unreduced they give the
// left, left, right cycle behind the figure that looks like a plus sign, while
// mod 2 gives 0, 1, 1 and a different figure altogether.
//
// Only the turtle can use this: a design with no modulus has no finite set of
// points to space around a circle.
func Unreduced(s Sequence) (Sequence, error) {
	u, ok := s.(unreducible)
	if !ok {
		return nil, fmt.Errorf("%s cannot be walked without a modulus", s.Name())
	}
	return unreducedSeq{name: s.Name() + " (unreduced)", inner: u}, nil
}

// unreducible is implemented by sequences that can report their terms without
// reduction. Keeping it unexported means the three-value encoding above stays
// an implementation detail of this file.
type unreducible interface {
	unreduced() Iter
}

type unreducedSeq struct {
	name  string
	inner unreducible
}

func (u unreducedSeq) Name() string { return u.name }

// Start ignores its modulus: there isn't one. Compute still drives it, and
// still finds the repeating block, because these iterators carry just as
// finite a state as the reduced ones do.
func (u unreducedSeq) Start(int) Iter { return u.inner.unreduced() }

// UnreducedPeriod is the repeating block of moves for a sequence read without a
// modulus. The returned Period has Modulus 0 to mark that no reduction was
// applied, and its Terms are MoveStay/MoveLeft/MoveRight rather than residues.
func UnreducedPeriod(s Sequence) (Period, error) {
	u, err := Unreduced(s)
	if err != nil {
		return Period{}, err
	}
	p := Compute(u, 1, 0)
	p.Modulus = 0
	return p, nil
}

// --- Fibonacci-like ----------------------------------------------------------

func (s linearSeq) unreduced() Iter {
	return &linearMoves{
		px: pmod(s.a, 2), py: pmod(s.b, 2),
		vx: int64(s.a), vy: int64(s.b),
	}
}

// linearMoves tracks two things about each pending term: its parity, exactly,
// and whether it is zero. The values themselves are saturated, because a
// Fibonacci number outruns int64 within ninety terms and the only question ever
// asked of the value is whether it is zero — and once both pending terms are
// positive the sum of them never returns to zero, so the answer is settled
// forever after. Parity is carried separately for exactly this reason: it is
// the one property saturation would destroy.
type linearMoves struct {
	px, py int
	vx, vy int64
}

const satCap = int64(1) << 62

func (it *linearMoves) Next() int {
	m := MoveRight
	switch {
	case it.vx == 0:
		m = MoveStay
	case it.px == 1:
		m = MoveLeft
	}
	it.px, it.py = it.py, (it.px+it.py)%2
	v := it.vx + it.vy
	if v > satCap || v < 0 {
		v = satCap
	}
	it.vx, it.vy = it.vy, v
	return m
}

func (it *linearMoves) State() (uint64, bool) {
	var k uint64
	k = uint64(it.px)<<3 | uint64(it.py)<<2 //nolint:gosec // a residue mod m, non-negative by construction
	if it.vx == 0 {
		k |= 2
	}
	if it.vy == 0 {
		k |= 1
	}
	return k, true
}

// --- The natural numbers -----------------------------------------------------

func (natSeq) unreduced() Iter { return &natMoves{first: true} }

// natMoves is the reference walk: the number line alternates even and odd, so
// after the single genuine zero the turtle turns left, right, left, right
// forever. It is the case the video draws first, and it makes a staircase.
type natMoves struct {
	odd   bool
	first bool
}

func (it *natMoves) Next() int {
	if it.first {
		it.first, it.odd = false, true
		return MoveStay
	}
	m := MoveRight
	if it.odd {
		m = MoveLeft
	}
	it.odd = !it.odd
	return m
}

func (it *natMoves) State() (uint64, bool) {
	var k uint64
	if it.odd {
		k |= 2
	}
	if it.first {
		k |= 1
	}
	return k, true
}

// --- Triangular --------------------------------------------------------------

func (triSeq) unreduced() Iter { return &triMoves{first: true} }

// triMoves needs no saturation: the parity of the nth triangular number depends
// only on n mod 4 — the pattern is even, odd, odd, even — and the only genuine
// zero is the very first term. So first has to be part of the state: without
// it, n = 0 and n = 4 would look identical and the run-in would be lost.
type triMoves struct {
	r     int  // n mod 4
	first bool // still at n == 0, the one term that is actually zero
}

func (it *triMoves) Next() int {
	m := MoveRight
	switch {
	case it.first:
		m = MoveStay
	case it.r == 1 || it.r == 2:
		m = MoveLeft
	}
	it.r = (it.r + 1) % 4
	it.first = false
	return m
}

func (it *triMoves) State() (uint64, bool) {
	k := uint64(it.r) << 1 //nolint:gosec // a residue mod m, non-negative by construction
	if it.first {
		k |= 1
	}
	return k, true
}

// --- Primes -------------------------------------------------------------------

func (primeSeq) unreduced() Iter { return &primeMoves{} }

// primeMoves is the degenerate case: two is the only even prime and none of
// them is zero, so after the first term the turtle turns left forever and
// traces a unit square. Unreduced, the primes are the one sequence here whose
// residues never repeat but whose parities immediately do.
type primeMoves struct{ past2 bool }

func (it *primeMoves) Next() int {
	if !it.past2 {
		it.past2 = true
		return MoveRight
	}
	return MoveLeft
}

func (it *primeMoves) State() (uint64, bool) {
	if it.past2 {
		return 1, true
	}
	return 0, true
}
