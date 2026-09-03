package pisano

import "sort"

// Chord is a line between two of the m points spaced around the circle. The
// endpoints are stored in ascending order because the drawing is undirected:
// stepping 3 -> 1 lays down the same line as 1 -> 3, and counting it twice
// would break the symmetry tests below.
type Chord struct{ A, B int }

// Design is the circular figure for one modulus: m points evenly spaced around
// a circle, joined in the order the remainders appear.
type Design struct {
	Seq     string
	Modulus int
	Period  int
	Zeros   int
	Chords  []Chord

	// Bounded carries through from the Period. When it is false the figure is
	// a prefix that happened to be drawn, not a closed design, and Period and
	// Zeros describe the scan rather than any repeating block.
	Bounded bool

	set map[Chord]bool
}

// Circular builds the design for a period. Successive remainders are joined,
// and because the sequence loops the last term is joined back to the first —
// the figure is closed, not a path with two loose ends.
func Circular(p Period) Design {
	d := Design{
		Seq:     p.Seq,
		Modulus: p.Modulus,
		Period:  p.Len(),
		Zeros:   p.Zeros(),
		Bounded: p.Bounded,
		set:     make(map[Chord]bool),
	}

	terms := p.Terms
	n := len(terms)
	if n == 0 {
		return d
	}

	last := n - 1
	if !p.Bounded {
		// No period was found, so there is no wrap-around to draw: joining
		// the final term back to the first would assert a loop that is not
		// there. Draw the prefix as an open path instead.
		last = n - 2
	}
	for i := 0; i <= last; i++ {
		d.add(terms[i], terms[(i+1)%n])
	}

	d.Chords = make([]Chord, 0, len(d.set))
	for c := range d.set {
		d.Chords = append(d.Chords, c)
	}
	sort.Slice(d.Chords, func(i, j int) bool {
		if d.Chords[i].A != d.Chords[j].A {
			return d.Chords[i].A < d.Chords[j].A
		}
		return d.Chords[i].B < d.Chords[j].B
	})
	return d
}

func (d *Design) add(a, b int) {
	if a == b {
		// A term repeating itself is a point, not a line. The Fibonacci
		// sequence opens 0,1,1 so this happens in every single design.
		return
	}
	if a > b {
		a, b = b, a
	}
	d.set[Chord{a, b}] = true
}

// Has reports whether the figure contains a chord, in either direction.
func (d Design) Has(a, b int) bool {
	if a > b {
		a, b = b, a
	}
	return d.set[Chord{a, b}]
}

// Reflections lists the mirror axes of the figure. A regular m-gon's vertex set
// has m reflections, each of the form k -> s-k (mod m) for some s; this returns
// every s that maps the chord set onto itself.
//
// This is the test behind the video's open question. It correlates the count of
// zeros in the period — always 1, 2 or 4 for Fibonacci — with whether the
// design comes out symmetrical, and Sweep in sweep.go runs it in bulk to see
// whether that correlation actually holds.
func (d Design) Reflections() []int {
	m := d.Modulus
	if m < 2 || len(d.Chords) == 0 {
		return nil
	}
	var axes []int
	for s := 0; s < m; s++ {
		ok := true
		for _, c := range d.Chords {
			if !d.Has(pmod(s-c.A, m), pmod(s-c.B, m)) {
				ok = false
				break
			}
		}
		if ok {
			axes = append(axes, s)
		}
	}
	return axes
}

// Symmetric reports whether the figure has any mirror axis.
func (d Design) Symmetric() bool { return len(d.Reflections()) > 0 }

// RotationOrder is how many times the figure maps onto itself under rotation,
// 1 meaning only the identity. It is not what the video calls symmetry, but it
// separates figures that merely look busy from ones with real structure.
func (d Design) RotationOrder() int {
	m := d.Modulus
	if m < 2 || len(d.Chords) == 0 {
		return 1
	}
	for step := 1; step < m; step++ {
		if m%step != 0 {
			continue
		}
		ok := true
		for _, c := range d.Chords {
			if !d.Has(pmod(c.A+step, m), pmod(c.B+step, m)) {
				ok = false
				break
			}
		}
		if ok {
			return m / step
		}
	}
	return 1
}

// Complete reports whether the figure uses every possible chord, as the video
// notes the mod-5 design does.
func (d Design) Complete() bool {
	m := d.Modulus
	return m > 1 && len(d.Chords) == m*(m-1)/2
}
