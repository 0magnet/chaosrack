package pisano

import (
	"fmt"
	"sort"
	"strings"
)

// Row is one modulus, measured every way the video's open questions need.
type Row struct {
	Modulus     int
	Period      int
	Zeros       int
	Chords      int
	Reflections int
	Rotation    int
	Complete    bool
	Shape       Shape
	Canon       string // set only when duplicate detection was asked for
}

func (r Row) Symmetric() bool { return r.Reflections > 0 }

// SweepOptions controls how much work a sweep does per modulus.
type SweepOptions struct {
	Max    int  // highest modulus to examine
	Dupes  bool // canonicalise turtle paths so identical designs can be grouped
	Passes int  // passes to canonicalise for open turtle paths
}

// Sweep measures a sequence across moduli 1..Max.
func Sweep(s Sequence, opt SweepOptions) []Row {
	if opt.Max < 1 {
		opt.Max = 1
	}
	if opt.Passes < 1 {
		opt.Passes = 2
	}

	rows := make([]Row, 0, opt.Max)
	for m := 1; m <= opt.Max; m++ {
		p := Compute(s, m, 0)
		d := Circular(p)
		r := Row{
			Modulus:     m,
			Period:      p.Len(),
			Zeros:       p.Zeros(),
			Chords:      len(d.Chords),
			Reflections: len(d.Reflections()),
			Rotation:    d.RotationOrder(),
			Complete:    d.Complete(),
			Shape:       Classify(p.Terms),
		}
		if opt.Dupes {
			pts, _ := PathOf(p, r.Shape.Passes(opt.Passes))
			r.Canon = CanonicalPath(pts)
		}
		rows = append(rows, r)
	}
	return rows
}

// ZeroSymmetry cross-tabulates the zero count against whether the circular
// design is symmetrical.
//
// The video's claim is that four zeros always give a symmetrical design, one
// zero always gives an asymmetrical one, and two zeros go either way. It says
// proofs exist for why the count is only ever 1, 2 or 4, but not for the
// correlation. Running it is not a proof either — but a single counterexample
// would settle it, and none turning up over thousands of moduli is worth
// knowing before anyone spends time trying to prove it.
func ZeroSymmetry(rows []Row) string {
	type key struct {
		zeros int
		sym   bool
	}
	count := map[key]int{}
	zeroKinds := map[int]bool{}
	for _, r := range rows {
		if r.Modulus < 3 {
			// One or two points on a circle cannot carry a figure that has
			// anything to be symmetrical about.
			continue
		}
		count[key{r.Zeros, r.Symmetric()}]++
		zeroKinds[r.Zeros] = true
	}

	kinds := make([]int, 0, len(zeroKinds))
	for z := range zeroKinds {
		kinds = append(kinds, z)
	}
	sort.Ints(kinds)

	var b strings.Builder
	fmt.Fprintf(&b, "%-7s  %10s  %12s\n", "ZEROS", "SYMMETRIC", "ASYMMETRIC")
	fmt.Fprintf(&b, "%-7s  %10s  %12s\n", "-----", "---------", "----------")
	for _, z := range kinds {
		fmt.Fprintf(&b, "%-7d  %10d  %12d\n", z, count[key{z, true}], count[key{z, false}])
	}

	var bad []string
	for _, r := range rows {
		if r.Modulus < 3 {
			continue
		}
		if r.Zeros == 4 && !r.Symmetric() {
			bad = append(bad, fmt.Sprintf("m=%d has 4 zeros but is asymmetrical", r.Modulus))
		}
		if r.Zeros == 1 && r.Symmetric() {
			bad = append(bad, fmt.Sprintf("m=%d has 1 zero but is symmetrical", r.Modulus))
		}
	}
	fmt.Fprintln(&b)
	if len(bad) == 0 {
		fmt.Fprintf(&b, "no counterexamples: 4 zeros => symmetrical and 1 zero => asymmetrical\n"+
			"held for every modulus 3..%d\n", rows[len(rows)-1].Modulus)
	} else {
		fmt.Fprintf(&b, "%d counterexample(s):\n", len(bad))
		for _, s := range bad {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	}
	return b.String()
}

// TurtleShapes reports how the turtle paths divide between closed and open, and
// what net turn decides it.
func TurtleShapes(rows []Row) string {
	byTurn := map[int]int{}
	closed, open := 0, 0
	for _, r := range rows {
		byTurn[r.Shape.Turn]++
		if r.Shape.Closed {
			closed++
		} else {
			open++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "closed %d, open %d, of %d moduli\n\n", closed, open, len(rows))
	fmt.Fprintf(&b, "%-12s  %8s  %s\n", "NET TURN", "COUNT", "RESULT")
	fmt.Fprintf(&b, "%-12s  %8s  %s\n", "--------", "-----", "------")
	for turn := 0; turn < 4; turn++ {
		n, ok := byTurn[turn]
		if !ok {
			continue
		}
		result := "closes after 4 passes"
		switch turn {
		case 0:
			result = "closes only if drift is zero, else open"
		case 2:
			result = "closes after 2 passes"
		}
		fmt.Fprintf(&b, "%-12s  %8d  %s\n", fmt.Sprintf("%d×90°", turn), n, result)
	}
	return b.String()
}

// Duplicates groups moduli whose turtle paths trace the same figure up to
// position and orientation. Requires a sweep run with Dupes set.
func Duplicates(rows []Row) string {
	groups := map[string][]int{}
	for _, r := range rows {
		if r.Canon == "" {
			continue
		}
		groups[r.Canon] = append(groups[r.Canon], r.Modulus)
	}
	if len(groups) == 0 {
		return "duplicate detection was not enabled for this sweep\n"
	}

	var shared [][]int
	for _, ms := range groups {
		if len(ms) > 1 {
			shared = append(shared, ms)
		}
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i][0] < shared[j][0] })

	var b strings.Builder
	fmt.Fprintf(&b, "%d distinct figures across %d moduli\n", len(groups), len(rows))
	if len(shared) == 0 {
		fmt.Fprint(&b, "every modulus traced a different figure\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d figure(s) shared by more than one modulus:\n", len(shared))
	for _, ms := range shared {
		parts := make([]string, len(ms))
		for i, m := range ms {
			parts[i] = fmt.Sprint(m)
		}
		fmt.Fprintf(&b, "  %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}

// Table is the per-modulus listing.
func Table(rows []Row) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%5s  %7s  %5s  %7s  %4s  %4s  %s\n",
		"MOD", "PERIOD", "ZEROS", "CHORDS", "SYM", "ROT", "TURTLE")
	fmt.Fprintf(&b, "%5s  %7s  %5s  %7s  %4s  %4s  %s\n",
		"---", "------", "-----", "------", "---", "---", "------")
	for _, r := range rows {
		sym := "-"
		if r.Symmetric() {
			sym = fmt.Sprint(r.Reflections)
		}
		turtle := "open"
		if r.Shape.Closed {
			turtle = fmt.Sprintf("closed/%d", r.Shape.Periods)
		}
		fmt.Fprintf(&b, "%5d  %7d  %5d  %7d  %4s  %4d  %s\n",
			r.Modulus, r.Period, r.Zeros, r.Chords, sym, r.Rotation, turtle)
	}
	return b.String()
}
