package attractor

// Fitting generators onto module panels.
//
// A module is a hardware unit. It has one panel, it takes the same power and
// the same signals as every other module, and what makes it THAT module is
// what is printed on it and wired behind it. That is an elegant arrangement
// and it is why a rack is repairable, but it is also why a rack is mostly
// air: a unit dedicated to one generator with three constants spends a whole
// panel on three knobs.
//
// A card per model said the truth about the models and the wrong thing about
// the hardware — fourteen units for fourteen systems, most of them a knob
// wide. The fix is not a taller rack. Underflow does not care how tall the
// frame is.
//
// So a module carries UP TO THREE generators. Three because a panel is three
// control rows deep, so three is the number of generators that can each own
// at least one row, and a unit holding three related systems is as honest a
// piece of hardware as a unit holding one — the same inputs, the same
// outputs, one panel, three circuits behind it. Each generator gets a
// rectangle of the panel with its name on it, and the rectangles tile.
//
// Chua's four constants make a two-by-two square; Thomas and Halvorsen have
// one each and take the two positions along the bottom beneath it. That is
// one module, three generators, six control positions, no blank panel.

// genTile is one generator's rectangle on a module panel, in control
// positions: col and row are where its top-left corner sits, w and h how far
// it reaches.
type genTile struct {
	Mode string
	Col  int
	Row  int
	W    int
	H    int
}

// Cells is how many control positions the tile covers. A generator with
// fewer constants than that leaves the remainder of its own rectangle blank,
// which is the generator's business rather than the panel's.
func (t genTile) Cells() int { return t.W * t.H }

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

// tileShape is the rectangle a generator with n constants takes.
//
// Constants read in a block, so the shape is as square as the count allows
// rather than a single long row: four constants are a square, not a line of
// four, because a square is what the eye takes in at once and what leaves
// the rest of the panel usable.
func tileShape(n int) (w, h int) {
	switch {
	case n <= 0:
		return 0, 0
	case n == 1:
		return 1, 1
	case n == 2:
		return 1, 2
	case n == 3:
		return 1, 3
	case n == 4:
		return 2, 2
	case n <= 6:
		return 2, 3
	}
	// Beyond six it is full-height columns, as many as the count needs.
	return (n + modRows - 1) / modRows, modRows
}

// packGenerators lays generators onto module panels, in order.
//
// Greedy and first-fit: each generator takes the leftmost, then topmost,
// rectangle it fits in on the current panel. When it does not fit, or the
// panel already carries three, a new module starts. Order is preserved, so
// the rack still reads in the order the catalog lists.
//
// maxCols bounds a single module, because a module that is wider than a bay
// cannot be put in one.
func packGenerators(gens []genSpec, maxCols int) [][]genTile {
	if maxCols < 1 {
		maxCols = 1
	}
	var out [][]genTile
	var cur []genTile
	occupied := map[[2]int]bool{}
	reset := func() {
		cur = nil
		occupied = map[[2]int]bool{}
	}
	for _, g := range gens {
		w, h := tileShape(g.Constants)
		if w == 0 {
			continue // nothing to put on a panel
		}
		col, row, ok := firstFit(occupied, w, h, maxCols)
		if !ok || len(cur) >= maxGensPerModule {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			reset()
			col, row, ok = firstFit(occupied, w, h, maxCols)
			if !ok {
				// Wider than a whole panel: it gets one to itself and
				// overhangs, which is visible and therefore right.
				col, row = 0, 0
			}
		}
		for dc := 0; dc < w; dc++ {
			for dr := 0; dr < h; dr++ {
				occupied[[2]int{col + dc, row + dr}] = true
			}
		}
		cur = append(cur, genTile{Mode: g.Mode, Col: col, Row: row, W: w, H: h})
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// genSpec is what the packer needs to know about a generator: which model it
// is, and how many constants of its own it has.
type genSpec struct {
	Mode      string
	Constants int
}

// firstFit finds the leftmost-then-topmost free rectangle of w by h.
//
// Column-major, because a panel fills left to right: the reader's eye goes
// down a column and then across, which is the order every other module in
// this rack lays its controls out in.
func firstFit(occupied map[[2]int]bool, w, h, maxCols int) (col, row int, ok bool) {
	for c := 0; c+w <= maxCols; c++ {
		for r := 0; r+h <= modRows; r++ {
			if freeAt(occupied, c, r, w, h) {
				return c, r, true
			}
		}
	}
	return 0, 0, false
}

// freeAt reports whether every position of the rectangle is unoccupied.
func freeAt(occupied map[[2]int]bool, col, row, w, h int) bool {
	for dc := 0; dc < w; dc++ {
		for dr := 0; dr < h; dr++ {
			if occupied[[2]int{col + dc, row + dr}] {
				return false
			}
		}
	}
	return true
}

// moduleCols is how many control columns a packed module needs.
func moduleCols(tiles []genTile) int {
	n := 0
	for _, t := range tiles {
		if r := t.Col + t.W; r > n {
			n = r
		}
	}
	return n
}
