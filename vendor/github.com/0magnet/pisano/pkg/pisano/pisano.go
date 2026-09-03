// Package pisano computes Pisano periods: the repeating block you get when an
// integer sequence is reduced modulo m.
//
// The Fibonacci numbers mod m always repeat, and the length of the repeat is
// the Pisano period. So do the Lucas and triangular numbers. The primes do not,
// which is why period detection here is driven by generator state rather than
// by any assumption about the recurrence: a sequence declares whether it has a
// finite state, and one that does not simply never reports a period.
//
// Nothing in this package draws anything. The renderers in svg.go and turtle.go
// both consume a Period, so a design can never disagree with the arithmetic it
// came from.
package pisano

import "fmt"

// Iter yields successive terms of a sequence, already reduced modulo m.
type Iter interface {
	// Next returns the next term mod m and advances.
	Next() int

	// State returns a key identifying the generator's current state: two
	// calls returning the same key have identical futures, so a repeat marks
	// the start of a cycle. ok is false for sequences with no finite state,
	// where no period can ever be detected.
	State() (key uint64, ok bool)
}

// Sequence is an integer sequence that can be walked modulo m.
type Sequence interface {
	Name() string
	Start(m int) Iter
}

// Period is one sequence reduced mod one modulus.
type Period struct {
	Seq     string
	Modulus int

	// Head is the run-in before the sequence starts repeating. Fibonacci and
	// Lucas are purely periodic so it is empty for them, but a sequence need
	// not be, and a design drawn from Terms alone would be wrong if it were.
	Head []int

	// Terms is the repeating block. When Bounded is false no repeat was
	// found and this holds the prefix that was examined instead.
	Terms []int

	// Bounded reports whether Terms is a genuine period rather than a
	// truncated prefix.
	Bounded bool

	// Cap is the term limit that was applied.
	Cap int
}

// Len is the Pisano period: the length of the repeating block.
func (p Period) Len() int { return len(p.Terms) }

// Zeros counts the zeros in the repeating block. For the Fibonacci numbers
// this is always 1, 2 or 4, and it is the quantity the video correlates with
// whether the circular design comes out symmetrical.
func (p Period) Zeros() int {
	n := 0
	for _, t := range p.Terms {
		if t == 0 {
			n++
		}
	}
	return n
}

func (p Period) String() string {
	if !p.Bounded {
		return fmt.Sprintf("%s mod %d: no period within %d terms", p.Seq, p.Modulus, p.Cap)
	}
	return fmt.Sprintf("%s mod %d: period %d, %d zero(s)", p.Seq, p.Modulus, p.Len(), p.Zeros())
}

// DefaultCap bounds period search for a given modulus. The Pisano period of
// the Fibonacci numbers never exceeds 6m, and the triangular numbers repeat
// within 2m, so this is slack for every bounded sequence here; for the primes
// it is simply how many terms get drawn.
func DefaultCap(m int) int {
	c := 8*m + 64
	if c < 1024 {
		c = 1024
	}
	return c
}

// Compute reduces s modulo m and finds the repeating block. A cap of 0 means
// DefaultCap.
func Compute(s Sequence, m, cap int) Period {
	if m < 1 {
		m = 1
	}
	if cap <= 0 {
		cap = DefaultCap(m)
	}

	p := Period{Seq: s.Name(), Modulus: m, Cap: cap}
	it := s.Start(m)
	seen := make(map[uint64]int)
	var terms []int

	for i := 0; i < cap; i++ {
		if key, ok := it.State(); ok {
			if j, dup := seen[key]; dup {
				// The state at index i already occurred at index j, so
				// terms[j:i] repeats forever and terms[:j] is the run-in.
				p.Head = append([]int(nil), terms[:j]...)
				p.Terms = append([]int(nil), terms[j:]...)
				p.Bounded = true
				return p
			}
			seen[key] = i
		}
		terms = append(terms, it.Next())
	}

	p.Terms = terms
	return p
}

// --- Fibonacci-like sequences -----------------------------------------------

// linearSeq is any sequence obeying x[n] = x[n-1] + x[n-2], which covers
// Fibonacci, Lucas and every scaled Fibonacci. Only the two seeds differ, so
// they share one iterator: mod m the state is the pair of pending terms, giving
// at most m*m states and therefore a guaranteed period.
type linearSeq struct {
	name string
	a, b int
}

func (s linearSeq) Name() string { return s.name }

func (s linearSeq) Start(m int) Iter {
	return &linearIter{m: m, x: pmod(s.a, m), y: pmod(s.b, m)}
}

type linearIter struct{ m, x, y int }

func (it *linearIter) Next() int {
	v := it.x
	it.x, it.y = it.y, (it.x+it.y)%it.m
	return v
}

func (it *linearIter) State() (uint64, bool) {
	return uint64(it.x)*uint64(it.m) + uint64(it.y), true //nolint:gosec // a residue mod m, non-negative by construction
}

// Fibonacci is 0, 1, 1, 2, 3, 5, 8, ...
func Fibonacci() Sequence { return linearSeq{"fib", 0, 1} }

// Lucas is 2, 1, 3, 4, 7, 11, ... — the same recurrence from different seeds.
// Its designs turn out to be near-duplicates of each other at moduli that are
// themselves Lucas numbers, mirroring what the Fibonacci designs do.
func Lucas() Sequence { return linearSeq{"lucas", 2, 1} }

// Scaled is the Fibonacci sequence multiplied through by k. Its designs
// reproduce the plain Fibonacci ones at every kth modulus and interleave new
// ones between them, which is the "design m/k" labeling: m is the modulus and
// k the multiplier. k is not a modulus and this is not "mod m/k" — a modulus
// is always an integer.
func Scaled(k int) Sequence {
	return linearSeq{fmt.Sprintf("fib*%d", k), 0, k}
}

// --- The natural numbers -----------------------------------------------------

// natSeq is 0, 1, 2, 3, ... — the plain number line. It is the reference the
// whole construction is introduced against: mod m it cycles through every
// remainder in order, so its circular designs are regular polygons and its
// turtle path alternates left and right forever. Everything interesting about
// the Fibonacci designs is a departure from these.
type natSeq struct{}

func (natSeq) Name() string { return "nat" }

func (natSeq) Start(m int) Iter { return &natIter{m: m} }

type natIter struct{ m, n int }

func (it *natIter) Next() int {
	v := it.n
	it.n = (it.n + 1) % it.m
	return v
}

func (it *natIter) State() (uint64, bool) { return uint64(it.n), true } //nolint:gosec // a residue mod m, non-negative by construction

// Naturals is 0, 1, 2, 3, 4, ...
func Naturals() Sequence { return natSeq{} }

// --- Triangular numbers ------------------------------------------------------

// triSeq is 0, 1, 3, 6, 10, ...: the difference between successive terms grows
// by one each step. Mod m the future depends only on (n mod m, T mod m), so it
// is periodic for the same reason the linear ones are, just with a different
// state.
type triSeq struct{}

func (triSeq) Name() string { return "tri" }

func (triSeq) Start(m int) Iter { return &triIter{m: m} }

type triIter struct{ m, n, t int }

func (it *triIter) Next() int {
	v := it.t
	it.n = (it.n + 1) % it.m
	it.t = (it.t + it.n) % it.m
	return v
}

func (it *triIter) State() (uint64, bool) {
	return uint64(it.n)*uint64(it.m) + uint64(it.t), true //nolint:gosec // a residue mod m, non-negative by construction
}

// Triangular is 0, 1, 3, 6, 10, 15, ...
func Triangular() Sequence { return triSeq{} }

// --- Primes ------------------------------------------------------------------

// primeSeq has no finite state — knowing the last prime and its residue tells
// you nothing about the next one — so its residues never provably repeat. It is
// here precisely to show what a non-periodic sequence looks like in this frame.
type primeSeq struct{}

func (primeSeq) Name() string { return "prime" }

func (primeSeq) Start(m int) Iter { return &primeIter{m: m, last: 1} }

type primeIter struct{ m, last int }

func (it *primeIter) Next() int {
	for n := it.last + 1; ; n++ {
		if isPrime(n) {
			it.last = n
			return n % it.m
		}
	}
}

func (it *primeIter) State() (uint64, bool) { return 0, false }

// Primes is 2, 3, 5, 7, 11, ...
func Primes() Sequence { return primeSeq{} }

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for d := 3; d*d <= n; d += 2 {
		if n%d == 0 {
			return false
		}
	}
	return true
}

// pmod is Go's % forced non-negative, so negative seeds still land in range.
func pmod(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}
