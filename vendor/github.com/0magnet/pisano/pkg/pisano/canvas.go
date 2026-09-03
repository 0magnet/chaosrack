package pisano

import "strings"

// Canvas is a grid of cells that can be drawn into in any order. It is adapted
// from the wire canvas in tinygo-stuff/schematic, with three changes that
// matter for turtle paths:
//
//   - a cell stores which of its four edges carry track, not a finished glyph.
//     The schematic version resolved a corner at the moment it drew one, which
//     cannot express a path crossing its own earlier self — and these paths do
//     that constantly.
//   - the origin can be negative. A turtle wanders wherever it likes, so the
//     canvas is told its bounds up front and translates into them.
//   - a horizontal unit is two columns wide, because terminal cells are about
//     twice as tall as they are wide and a one-for-one grid comes out squashed.
type Canvas struct {
	w, h       int
	minX, minY int
	mask       []uint8
	color      []string
}

// Edge bits. A cell's mask is the set of directions in which track leaves it.
const (
	edgeN uint8 = 1 << iota
	edgeE
	edgeS
	edgeW
)

// NewCanvas covers the inclusive cell range given.
func NewCanvas(minX, minY, maxX, maxY int) *Canvas {
	w, h := maxX-minX+1, maxY-minY+1
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{
		w: w, h: h, minX: minX, minY: minY,
		mask:  make([]uint8, w*h),
		color: make([]string, w*h),
	}
}

func (c *Canvas) idx(x, y int) (int, bool) {
	x -= c.minX
	y -= c.minY
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return 0, false
	}
	return y*c.w + x, true
}

// mark adds edges to a cell. Marks accumulate, so a path crossing itself gets a
// junction rather than overwriting what was there.
func (c *Canvas) mark(x, y int, edges uint8, col string) {
	i, ok := c.idx(x, y)
	if !ok {
		return
	}
	c.mask[i] |= edges
	if col != "" {
		c.color[i] = col
	}
}

// Segment draws a straight run of one cell width between two cells that share a
// row or a column, marking the edges at every cell along the way.
func (c *Canvas) Segment(x0, y0, x1, y1 int, col string) {
	switch {
	case y0 == y1:
		lo, hi := minMax(x0, x1)
		for x := lo; x <= hi; x++ {
			var e uint8
			if x > lo {
				e |= edgeW
			}
			if x < hi {
				e |= edgeE
			}
			c.mark(x, y0, e, col)
		}
	case x0 == x1:
		lo, hi := minMax(y0, y1)
		for y := lo; y <= hi; y++ {
			var e uint8
			if y > lo {
				e |= edgeN
			}
			if y < hi {
				e |= edgeS
			}
			c.mark(x0, y, e, col)
		}
	}
}

// glyphs maps an edge mask to a box-drawing rune. Every one of the sixteen
// combinations has an entry, which is what lets a self-crossing path draw
// correctly: the schematic's elbow function had no way to name a T-junction and
// fell back to a placeholder.
var glyphs = [16]rune{
	0:                             ' ',
	edgeN:                         '╵',
	edgeE:                         '╶',
	edgeS:                         '╷',
	edgeW:                         '╴',
	edgeN | edgeS:                 '│',
	edgeE | edgeW:                 '─',
	edgeN | edgeE:                 '└',
	edgeN | edgeW:                 '┘',
	edgeS | edgeE:                 '┌',
	edgeS | edgeW:                 '┐',
	edgeN | edgeE | edgeW:         '┴',
	edgeS | edgeE | edgeW:         '┬',
	edgeN | edgeS | edgeE:         '├',
	edgeN | edgeS | edgeW:         '┤',
	edgeN | edgeS | edgeE | edgeW: '┼',
}

// String renders the canvas, optionally with ANSI color. Trailing blanks are
// dropped by finding the last drawn cell rather than by trimming the finished
// string: once escape codes are in the line, its length no longer tells you
// anything about what is on screen. That mismatch is exactly what drove the
// original schematic to interrogate the terminal for its cursor position.
func (c *Canvas) String(colorize bool) string {
	var b strings.Builder
	for y := 0; y < c.h; y++ {
		end := -1
		for x := c.w - 1; x >= 0; x-- {
			if c.mask[y*c.w+x]&0x0f != 0 {
				end = x
				break
			}
		}
		cur := ""
		for x := 0; x <= end; x++ {
			i := y*c.w + x
			if colorize {
				if col := c.color[i]; col != cur {
					if cur != "" {
						b.WriteString("\x1b[0m")
					}
					b.WriteString(col)
					cur = col
				}
			}
			b.WriteRune(glyphs[c.mask[i]&0x0f])
		}
		if colorize && cur != "" {
			b.WriteString("\x1b[0m")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func minMax(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}
