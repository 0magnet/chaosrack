package rackhost

import (
	"image"
	"image/color"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// A terminal's screen as a picture, a cell at a time: what the page's
// terminal draws with a font, drawn here with basicfont's 7 by 13 glyphs, and
// what a terminal program draws pictures with — block elements, box lines and
// braille — drawn as the shapes they stand for, so a program's pixel art and
// its frames come out solid rather than in a font's idea of them.

// cellW and cellH are a cell's size in the picture: basicfont's glyph, and a
// row more so a half block splits it evenly.
const cellW, cellH = 7, 14

// cellPicture is a picture of cols by rows cells.
func cellPicture(cols, rows int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
}

// paintCell paints the cell at column x, row y: rune ch in fg over bg.
func paintCell(m *image.RGBA, x, y int, ch rune, fg, bg color.RGBA) {
	x0, y0 := x*cellW, y*cellH
	fill := func(fx0, fy0, fx1, fy1 int, c color.RGBA) {
		for py := y0 + fy0; py < y0+fy1; py++ {
			for px := x0 + fx0; px < x0+fx1; px++ {
				m.SetRGBA(px, py, c)
			}
		}
	}
	fill(0, 0, cellW, cellH, bg)
	switch {
	case ch <= ' ':
	case ch >= 0x2580 && ch <= 0x259f:
		paintBlock(ch, fg, bg, fill)
	case ch >= 0x2800 && ch <= 0x28ff:
		paintBraille(ch, fg, fill)
	case boxLines(ch) != 0:
		paintBox(boxLines(ch), fg, fill)
	default:
		d := font.Drawer{Dst: m, Src: image.NewUniform(fg), Face: basicfont.Face7x13,
			Dot: fixed.P(x0, y0+basicfont.Face7x13.Ascent+1)}
		if _, ok := basicfont.Face7x13.GlyphAdvance(ch); !ok {
			// A glyph the font lacks is a mark where it stands, not a blank: a
			// program drew something there.
			fill(2, 5, 5, 9, fg)
			return
		}
		d.DrawString(string(ch))
	}
}

// paintBlock paints a block element, U+2580..U+259F.
func paintBlock(ch rune, fg, bg color.RGBA, fill func(x0, y0, x1, y1 int, c color.RGBA)) {
	const hw, hh = cellW / 2, cellH / 2
	switch {
	case ch == '▀':
		fill(0, 0, cellW, hh, fg)
	case ch >= '▁' && ch <= '█': // lower eighths, to the full block
		h := int(ch-'▀') * cellH / 8
		fill(0, cellH-h, cellW, cellH, fg)
	case ch >= '▉' && ch <= '▏': // left eighths
		w := int('▏'-ch+1) * cellW / 8
		fill(0, 0, max(w, 1), cellH, fg)
	case ch == '▐':
		fill(hw, 0, cellW, cellH, fg)
	case ch >= '░' && ch <= '▓': // shades: a quarter, half and three quarters of fg
		k := uint32(ch-'░'+1) * 64
		mix := func(a, b uint8) uint8 { return uint8((uint32(a)*k + uint32(b)*(256-k)) / 256) } //nolint:gosec // a blend of two bytes is a byte
		fill(0, 0, cellW, cellH, color.RGBA{mix(fg.R, bg.R), mix(fg.G, bg.G), mix(fg.B, bg.B), 255})
	case ch == '▔':
		fill(0, 0, cellW, cellH/8, fg)
	case ch == '▕':
		fill(cellW-1, 0, cellW, cellH, fg)
	default: // quadrants, U+2596..U+259F: which of the four are lit
		q := quadrants[ch-0x2596]
		for i, on := range [4]bool{q&1 != 0, q&2 != 0, q&4 != 0, q&8 != 0} {
			if on {
				qx, qy := i%2*hw, i/2*hh
				fill(qx, qy, qx+hw+i%2*(cellW-2*hw), qy+hh, fg)
			}
		}
	}
}

// quadrants are U+2596..U+259F's lit quarters: 1 upper left, 2 upper right,
// 4 lower left, 8 lower right.
var quadrants = [10]uint8{4, 8, 1, 1 | 4 | 8, 1 | 8, 1 | 2 | 4, 1 | 2 | 8, 2, 2 | 4, 2 | 4 | 8}

// paintBraille paints a braille pattern: eight dots, two across and four
// down, its low byte saying which are raised.
func paintBraille(ch rune, fg color.RGBA, fill func(x0, y0, x1, y1 int, c color.RGBA)) {
	bits := ch - 0x2800
	// Dots 1-3 down the left, 4-6 down the right, then 7 and 8 under them.
	at := [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}
	for i, p := range at {
		if bits&(1<<i) != 0 {
			dx, dy := 1+p[0]*3, 1+p[1]*cellH/4
			fill(dx, dy, dx+2, dy+2, fg)
		}
	}
}

// Box line directions, as boxLines reports them.
const (
	lineUp = 1 << iota
	lineDown
	lineLeft
	lineRight
)

// boxLines is which way box-drawing character ch runs from its middle, or 0
// for one that is not drawn as lines. Heavy, double and rounded lines are
// drawn as light ones.
func boxLines(ch rune) int {
	switch ch {
	case '─', '━', '═', '╌', '╍', '┄', '┅', '┈', '┉':
		return lineLeft | lineRight
	case '│', '┃', '║', '╎', '╏', '┆', '┇', '┊', '┋':
		return lineUp | lineDown
	case '┌', '┏', '╔', '╭', '┍', '┎', '╒', '╓':
		return lineDown | lineRight
	case '┐', '┓', '╗', '╮', '┑', '┒', '╕', '╖':
		return lineDown | lineLeft
	case '└', '┗', '╚', '╰', '┕', '┖', '╘', '╙':
		return lineUp | lineRight
	case '┘', '┛', '╝', '╯', '┙', '┚', '╛', '╜':
		return lineUp | lineLeft
	case '├', '┣', '╠', '┝', '┞', '┟', '┠', '┡', '┢', '╞', '╟':
		return lineUp | lineDown | lineRight
	case '┤', '┫', '╣', '┥', '┦', '┧', '┨', '┩', '┪', '╡', '╢':
		return lineUp | lineDown | lineLeft
	case '┬', '┳', '╦', '┭', '┮', '┯', '┰', '┱', '┲', '╤', '╥':
		return lineDown | lineLeft | lineRight
	case '┴', '┻', '╩', '┵', '┶', '┷', '┸', '┹', '┺', '╧', '╨':
		return lineUp | lineLeft | lineRight
	case '┼', '╋', '╬', '┽', '┾', '┿', '╀', '╁', '╂', '╃', '╄', '╅', '╆', '╇', '╈', '╉', '╊', '╪', '╫':
		return lineUp | lineDown | lineLeft | lineRight
	case '╴', '╸':
		return lineLeft
	case '╶', '╺':
		return lineRight
	case '╵', '╹':
		return lineUp
	case '╷', '╻':
		return lineDown
	}
	return 0
}

// paintBox paints box lines running the ways dirs says from the middle.
func paintBox(dirs int, fg color.RGBA, fill func(x0, y0, x1, y1 int, c color.RGBA)) {
	const mx, my = cellW / 2, cellH / 2
	if dirs&lineUp != 0 {
		fill(mx, 0, mx+1, my+1, fg)
	}
	if dirs&lineDown != 0 {
		fill(mx, my, mx+1, cellH, fg)
	}
	if dirs&lineLeft != 0 {
		fill(0, my, mx+1, my+1, fg)
	}
	if dirs&lineRight != 0 {
		fill(mx, my, cellW, my+1, fg)
	}
}
