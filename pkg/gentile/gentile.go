// Package gentile fits the signal generators onto module panels.
package gentile

import (
	"slices"
	"sort"
)

// Fitting generators onto module panels.
//
// A module is a hardware unit. It has one panel, it takes the same power and
// the same signals as every other module, and what makes it THAT module is
// what is printed on it and wired behind it. That is an elegant arrangement
// and it is why a rack is repairable, but it is also why a rack is mostly
// air: a unit dedicated to one generator with two constants spends a whole
// panel on two knobs.
//
// So a module carries UP TO THREE generators, and the reason to put two of
// them on one panel is that between them they FILL it. A panel is three
// control rows deep, so a column is three positions and a module is whole
// columns. A generator with three constants, or six, or nine, already fills
// its columns exactly: there is no space left in it to share, and combining
// it with another would say those two belong together when the only thing
// they would have in common is a panel. Those stand alone.
//
// The rest are combined so that the counts add up to whole columns. Four
// and two make a module two columns wide: the four across the top two rows,
// the two along the bottom, side by side. One and two make a single column;
// so do one and one and one. Five and one fill two columns; two and two and
// two fill them as three bands. What is not allowed is a module with a hole
// in it, which is the thing this replaces.
//
// Positions are filled in READING ORDER — across the top row, then the next
// — because that is the order the combinations above describe and the only
// order in which a generator's run is contiguous however the counts fall.

// modRows is how many control rows a module panel has. The grid every other
// module in the rack uses is three deep, and this is that number: a
// generator layout that wanted four would not be a module any more.
const modRows = 3

// maxGensPerModule is the most generators one unit carries.
//
// Three, so that each can own a row. It is a statement about the hardware
// rather than a packing parameter — a unit with eight generators on it is
// not a module, it is a mainframe with the cards soldered in.
const maxGensPerModule = 3

// dpLimit is how many combinable generators one category can have before
// the search for the best grouping is replaced by a good one. The exact
// search is exponential; the largest category has nine, and the fallback
// below reaches the same minimum waste anyway — it just may use one module
// more than it had to.
const dpLimit = 14

// Spec is what the packer needs to know about a generator: which model it
// is, and how many constants of its own it has.
type Spec struct {
	Mode      string
	Constants int
}

// Tile is one generator's run of control positions on a module panel.
//
// A run rather than a rectangle. A rectangle cannot hold five positions in a
// three-row panel, and rounding five up to a six-cell rectangle puts the
// hole back — which is what this is here to avoid. Reading order makes any
// count contiguous, and most runs come out rectangular anyway: four in a
// two-column module is the top two rows, two is a row.
type Tile struct {
	Mode  string
	Start int // first position, counting across the top row and down
	N     int // how many positions
}

// Module is one hardware unit's panel: how wide it is, and what is on it.
type Module struct {
	Cols  int
	Tiles []Tile
}

// Cells is the panel's capacity in control positions.
func (m Module) Cells() int { return m.Cols * modRows }

// Used is how many of them carry a knob.
func (m Module) Used() int {
	n := 0
	for _, t := range m.Tiles {
		n += t.N
	}
	return n
}

// ColRow is where a position sits: across first, then down.
func (m Module) ColRow(pos int) (col, row int) {
	if m.Cols < 1 {
		return 0, 0
	}
	return pos % m.Cols, pos / m.Cols
}

// Pack lays generators onto module panels.
//
// maxCols bounds a module, because a module wider than a bay cannot be put
// in one. A single generator that will not fit in a bay on its own gets a
// panel of its own and overhangs it, which is visible and therefore right.
func Pack(gens []Spec, maxCols int) []Module {
	if maxCols < 1 {
		maxCols = 1
	}
	var groups [][]int
	var mix []int
	for i, g := range gens {
		switch {
		case g.Constants <= 0:
			// Nothing to put on a panel. The model is still in the bay's
			// selector; it just has no constants of its own to mount.
		case g.Constants%modRows == 0:
			groups = append(groups, []int{i})
		default:
			mix = append(mix, i)
		}
	}
	groups = append(groups, fillColumns(gens, mix, maxCols)...)

	// Modules come out in catalog order, by the first generator on each, so
	// the rack still reads in the order the catalog lists even though the
	// grouping is free to reach past a neighbor to fill a panel.
	sort.SliceStable(groups, func(a, b int) bool { return groups[a][0] < groups[b][0] })

	out := make([]Module, 0, len(groups))
	for _, g := range groups {
		out = append(out, layOut(gens, g, maxCols))
	}
	return out
}

// layOut turns one group of generators into a panel.
func layOut(gens []Spec, group []int, maxCols int) Module {
	sum := 0
	for _, i := range group {
		sum += gens[i].Constants
	}
	cols := (sum + modRows - 1) / modRows
	if cols > maxCols && len(group) > 1 {
		cols = maxCols // only reachable if a caller hands us an oversized group
	}
	m := Module{Cols: cols}
	pos := 0
	for _, i := range group {
		m.Tiles = append(m.Tiles, Tile{Mode: gens[i].Mode, Start: pos, N: gens[i].Constants})
		pos += gens[i].Constants
	}
	return m
}

// fillColumns groups the generators that do not fill their own columns, so
// that between them they fill the panels they share.
//
// It minimizes wasted positions first and the number of modules second: of
// two groupings that leave the same amount of blank panel, the one with
// fewer units is the one a rack would actually be built as.
func fillColumns(gens []Spec, idx []int, maxCols int) [][]int {
	n := len(idx)
	if n == 0 {
		return nil
	}
	if n > dpLimit {
		return byResidue(gens, idx, maxCols)
	}
	const inf = 1 << 30
	// Three objectives in order: leave as little blank panel as possible,
	// then build as few units as possible, then put whatever blank there is
	// on the LAST panel rather than the first. The third is why a group is
	// charged its blank positions once for every generator still unplaced:
	// a hole early costs more than the same hole at the end, so the full
	// panels come first and the ragged one is the last in the bay.
	type cost struct{ waste, mods, late int }
	better := func(a, b cost) bool {
		if a.waste != b.waste {
			return a.waste < b.waste
		}
		if a.mods != b.mods {
			return a.mods < b.mods
		}
		return a.late < b.late
	}
	full := 1 << n
	dp := make([]cost, full)
	from := make([]int, full)
	pick := make([][]int, full)
	for i := range dp {
		dp[i] = cost{inf, inf, inf}
	}
	dp[0] = cost{}
	for mask := range full {
		if dp[mask].waste == inf {
			continue
		}
		// The lowest generator not yet placed must be in the next group,
		// which is what keeps this a partition rather than a power set.
		low := 0
		for low < n && mask&(1<<low) != 0 {
			low++
		}
		if low == n {
			continue
		}
		try := func(g ...int) {
			sum, next := 0, mask
			for _, k := range g {
				sum += gens[idx[k]].Constants
				next |= 1 << k
			}
			cols := (sum + modRows - 1) / modRows
			if cols > maxCols && len(g) > 1 {
				return // a group nobody could mount in one bay
			}
			blank := cols*modRows - sum
			left := 0
			for k := range n {
				if next&(1<<k) == 0 {
					left++
				}
			}
			c := cost{dp[mask].waste + blank, dp[mask].mods + 1, dp[mask].late + blank*left}
			if better(c, dp[next]) {
				dp[next], from[next] = c, mask
				pick[next] = append([]int(nil), g...)
			}
		}
		// One, two or three: maxGensPerModule is the whole of the search
		// space, which is why there is no loop over group size here.
		try(low)
		for j := low + 1; j < n; j++ {
			if mask&(1<<j) != 0 {
				continue
			}
			try(low, j)
			for k := j + 1; k < n; k++ {
				if mask&(1<<k) != 0 {
					continue
				}
				try(low, j, k)
			}
		}
	}
	var out [][]int
	for mask := full - 1; mask != 0; mask = from[mask] {
		g := make([]int, 0, len(pick[mask]))
		for _, k := range pick[mask] {
			g = append(g, idx[k])
		}
		sort.Ints(g)
		out = append(out, g)
	}
	return out
}

// byResidue is the fallback for a category too large to search exhaustively.
//
// A group fills its columns exactly when the counts sum to a multiple of
// three, and a count that is not a multiple of three leaves a remainder of
// one or two. One and two make three; three ones make three; so do three
// twos. That is every combination there is, so pairing the two piles and
// then taking what is left in threes reaches the least possible waste.
func byResidue(gens []Spec, idx []int, maxCols int) [][]int {
	var ones, twos []int
	for _, i := range idx {
		if gens[i].Constants%modRows == 1 {
			ones = append(ones, i)
		} else {
			twos = append(twos, i)
		}
	}
	var out [][]int
	fits := func(g []int) bool {
		sum := 0
		for _, i := range g {
			sum += gens[i].Constants
		}
		return (sum+modRows-1)/modRows <= maxCols || len(g) == 1
	}
	emit := func(g []int) {
		sort.Ints(g)
		if !fits(g) {
			for _, i := range g {
				out = append(out, []int{i})
			}
			return
		}
		out = append(out, g)
	}
	for len(ones) > 0 && len(twos) > 0 {
		emit([]int{ones[0], twos[0]})
		ones, twos = ones[1:], twos[1:]
	}
	rest := slices.Concat(ones, twos)
	for len(rest) >= maxGensPerModule {
		emit(append([]int(nil), rest[:maxGensPerModule]...))
		rest = rest[maxGensPerModule:]
	}
	if len(rest) > 0 {
		emit(append([]int(nil), rest...))
	}
	return out
}
