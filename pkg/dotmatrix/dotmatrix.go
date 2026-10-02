// Package dotmatrix draws text the way a character LCD does: every
// character a 5×7 cell of dots, one dot column between cells, and every dot
// of the display there whether it is lit or not.
//
// It exists for the knob bank. A bank knob does a different job for every
// model — σ on Lorenz, a on Aizawa — so the legend over it cannot be
// printed on the panel. Synthesizers that faced the same problem put a
// small display over each knob, and the display they used was this one: an
// HD44780-style character module, whose font this follows, with the Greek
// letters added as the user-defined characters the HD44780 kept eight slots
// for.
//
// The output is SVG path data in dot units, one path for the lit dots and one
// for the dark ones, so a page can draw a whole display as two elements and
// color it with CSS.
package dotmatrix

import (
	"strconv"
	"strings"
)

// Cell geometry: a character is Cols dots wide and Rows high, and characters
// are Gap dots apart.
const (
	Cols = 5
	Rows = 7
	Gap  = 1
)

// font is each character's rows, top first, the leftmost dot the high bit
// of the five.
var font = map[rune][Rows]uint8{
	' ': {},
	'0': {0b01110, 0b10001, 0b10011, 0b10101, 0b11001, 0b10001, 0b01110},
	'1': {0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'2': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b01000, 0b11111},
	'3': {0b11111, 0b00010, 0b00100, 0b00010, 0b00001, 0b10001, 0b01110},
	'4': {0b00010, 0b00110, 0b01010, 0b10010, 0b11111, 0b00010, 0b00010},
	'5': {0b11111, 0b10000, 0b11110, 0b00001, 0b00001, 0b10001, 0b01110},
	'6': {0b00110, 0b01000, 0b10000, 0b11110, 0b10001, 0b10001, 0b01110},
	'7': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	'8': {0b01110, 0b10001, 0b10001, 0b01110, 0b10001, 0b10001, 0b01110},
	'9': {0b01110, 0b10001, 0b10001, 0b01111, 0b00001, 0b00010, 0b01100},

	'A': {0b01110, 0b10001, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001},
	'B': {0b11110, 0b10001, 0b10001, 0b11110, 0b10001, 0b10001, 0b11110},
	'C': {0b01110, 0b10001, 0b10000, 0b10000, 0b10000, 0b10001, 0b01110},
	'D': {0b11100, 0b10010, 0b10001, 0b10001, 0b10001, 0b10010, 0b11100},
	'E': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b11111},
	'F': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b10000},
	'G': {0b01110, 0b10001, 0b10000, 0b10111, 0b10001, 0b10001, 0b01111},
	'H': {0b10001, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'I': {0b01110, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'J': {0b00111, 0b00010, 0b00010, 0b00010, 0b00010, 0b10010, 0b01100},
	'K': {0b10001, 0b10010, 0b10100, 0b11000, 0b10100, 0b10010, 0b10001},
	'L': {0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b11111},
	'M': {0b10001, 0b11011, 0b10101, 0b10101, 0b10001, 0b10001, 0b10001},
	'N': {0b10001, 0b10001, 0b11001, 0b10101, 0b10011, 0b10001, 0b10001},
	'O': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'P': {0b11110, 0b10001, 0b10001, 0b11110, 0b10000, 0b10000, 0b10000},
	'Q': {0b01110, 0b10001, 0b10001, 0b10001, 0b10101, 0b10010, 0b01101},
	'R': {0b11110, 0b10001, 0b10001, 0b11110, 0b10100, 0b10010, 0b10001},
	'S': {0b01111, 0b10000, 0b10000, 0b01110, 0b00001, 0b00001, 0b11110},
	'T': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100},
	'U': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'V': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01010, 0b00100},
	'W': {0b10001, 0b10001, 0b10001, 0b10101, 0b10101, 0b10101, 0b01010},
	'X': {0b10001, 0b10001, 0b01010, 0b00100, 0b01010, 0b10001, 0b10001},
	'Y': {0b10001, 0b10001, 0b10001, 0b01010, 0b00100, 0b00100, 0b00100},
	'Z': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b10000, 0b11111},

	// Lowercase as the HD44780 draws it in a 5×7 cell: the descenders of
	// g, j, p, q and y are folded up into the cell rather than hanging below.
	'a': {0b00000, 0b00000, 0b01110, 0b00001, 0b01111, 0b10001, 0b01111},
	'b': {0b10000, 0b10000, 0b10110, 0b11001, 0b10001, 0b10001, 0b11110},
	'c': {0b00000, 0b00000, 0b01110, 0b10000, 0b10000, 0b10001, 0b01110},
	'd': {0b00001, 0b00001, 0b01101, 0b10011, 0b10001, 0b10001, 0b01111},
	'e': {0b00000, 0b00000, 0b01110, 0b10001, 0b11111, 0b10000, 0b01110},
	'f': {0b00110, 0b01001, 0b01000, 0b11100, 0b01000, 0b01000, 0b01000},
	'g': {0b00000, 0b01111, 0b10001, 0b10001, 0b01111, 0b00001, 0b01110},
	'h': {0b10000, 0b10000, 0b10110, 0b11001, 0b10001, 0b10001, 0b10001},
	'i': {0b00100, 0b00000, 0b01100, 0b00100, 0b00100, 0b00100, 0b01110},
	'j': {0b00010, 0b00000, 0b00110, 0b00010, 0b00010, 0b10010, 0b01100},
	'k': {0b10000, 0b10000, 0b10010, 0b10100, 0b11000, 0b10100, 0b10010},
	'l': {0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'm': {0b00000, 0b00000, 0b11010, 0b10101, 0b10101, 0b10001, 0b10001},
	'n': {0b00000, 0b00000, 0b10110, 0b11001, 0b10001, 0b10001, 0b10001},
	'o': {0b00000, 0b00000, 0b01110, 0b10001, 0b10001, 0b10001, 0b01110},
	'p': {0b00000, 0b00000, 0b11110, 0b10001, 0b11110, 0b10000, 0b10000},
	'q': {0b00000, 0b00000, 0b01101, 0b10011, 0b01111, 0b00001, 0b00001},
	'r': {0b00000, 0b00000, 0b10110, 0b11001, 0b10000, 0b10000, 0b10000},
	's': {0b00000, 0b00000, 0b01110, 0b10000, 0b01110, 0b00001, 0b11110},
	't': {0b01000, 0b01000, 0b11100, 0b01000, 0b01000, 0b01001, 0b00110},
	'u': {0b00000, 0b00000, 0b10001, 0b10001, 0b10001, 0b10011, 0b01101},
	'v': {0b00000, 0b00000, 0b10001, 0b10001, 0b10001, 0b01010, 0b00100},
	'w': {0b00000, 0b00000, 0b10001, 0b10001, 0b10101, 0b10101, 0b01010},
	'x': {0b00000, 0b00000, 0b10001, 0b01010, 0b00100, 0b01010, 0b10001},
	'y': {0b00000, 0b00000, 0b10001, 0b10001, 0b01111, 0b00001, 0b01110},
	'z': {0b00000, 0b00000, 0b11111, 0b00010, 0b00100, 0b01000, 0b11111},
	// The scope's timebase reads in microseconds.
	'µ': {0b00000, 0b00000, 0b10001, 0b10001, 0b10011, 0b11101, 0b10000},

	// The user-defined characters: the Greek that the constants are named in.
	'α': {0b00000, 0b00000, 0b01001, 0b10101, 0b10010, 0b10010, 0b01101},
	'β': {0b01110, 0b10001, 0b10001, 0b11110, 0b10001, 0b11110, 0b10000},
	'γ': {0b00000, 0b10001, 0b10001, 0b01010, 0b00100, 0b01010, 0b00100},
	'δ': {0b01110, 0b01000, 0b00100, 0b01110, 0b10001, 0b10001, 0b01110},
	'ε': {0b00000, 0b00000, 0b01111, 0b10000, 0b01110, 0b10000, 0b01111},
	'θ': {0b01110, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b01110},
	'λ': {0b01000, 0b00100, 0b00100, 0b01010, 0b01010, 0b10001, 0b10001},
	'μ': {0b00000, 0b00000, 0b10001, 0b10001, 0b10011, 0b11101, 0b10000},
	'π': {0b00000, 0b00000, 0b11111, 0b01010, 0b01010, 0b01010, 0b10011},
	'ρ': {0b00000, 0b00000, 0b01110, 0b10001, 0b10001, 0b11110, 0b10000},
	'σ': {0b00000, 0b00000, 0b01111, 0b10010, 0b10001, 0b10001, 0b01110},
	'τ': {0b00000, 0b00000, 0b11111, 0b00100, 0b00100, 0b00100, 0b00010},
	'φ': {0b00100, 0b01110, 0b10101, 0b10101, 0b01110, 0b00100, 0b00100},
	'ω': {0b00000, 0b00000, 0b01010, 0b10001, 0b10101, 0b10101, 0b01010},
	'Δ': {0b00100, 0b00100, 0b01010, 0b01010, 0b10001, 0b10001, 0b11111},
	'Σ': {0b11111, 0b10000, 0b01000, 0b00100, 0b01000, 0b10000, 0b11111},
	'Ω': {0b01110, 0b10001, 0b10001, 0b10001, 0b01010, 0b01010, 0b11011},

	'-': {0b00000, 0b00000, 0b00000, 0b11111, 0b00000, 0b00000, 0b00000},
	'.': {0b00000, 0b00000, 0b00000, 0b00000, 0b00000, 0b01100, 0b01100},
	'/': {0b00000, 0b00001, 0b00010, 0b00100, 0b01000, 0b10000, 0b00000},
	'+': {0b00000, 0b00100, 0b00100, 0b11111, 0b00100, 0b00100, 0b00000},
	'&': {0b01100, 0b10010, 0b10100, 0b01000, 0b10101, 0b10010, 0b01101},
	'%': {0b11000, 0b11001, 0b00010, 0b00100, 0b01000, 0b10011, 0b00011},
	':': {0b00000, 0b01100, 0b01100, 0b00000, 0b01100, 0b01100, 0b00000},
	'<': {0b00010, 0b00100, 0b01000, 0b10000, 0b01000, 0b00100, 0b00010},
	'>': {0b01000, 0b00100, 0b00010, 0b00001, 0b00010, 0b00100, 0b01000},
	'?': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b00000, 0b00100},
}

// Has reports whether the font draws r, rather than the '?' it falls back to.
func Has(r rune) bool {
	_, ok := font[r]
	return ok
}

// Width is how many dots wide a display of n characters is.
func Width(n int) int {
	if n < 1 {
		return 0
	}
	return n*(Cols+Gap) - Gap
}

// Paths draws text on a display n characters wide, left-aligned the way a
// character module writes, cut at n. It returns SVG path data for the lit
// dots and for the dark ones, in dot units: the display is Width(n) by Rows,
// and each dot is a square slightly smaller than its pitch so the grid shows.
func Paths(text string, n int) (lit, dark string) {
	var l, d strings.Builder
	rs := []rune(text)
	for i := range n {
		var g [Rows]uint8
		if i < len(rs) {
			var ok bool
			if g, ok = font[rs[i]]; !ok {
				g = font['?']
			}
		}
		for row := range Rows {
			for col := range Cols {
				b := &d
				if g[row]&(1<<(Cols-1-col)) != 0 {
					b = &l
				}
				x := i*(Cols+Gap) + col
				b.WriteString("M" + strconv.Itoa(x) + ".1 " + strconv.Itoa(row) + ".1h.8v.8h-.8z")
			}
		}
	}
	return l.String(), d.String()
}

// Height is how many dots tall a vertical display of n characters is.
func Height(n int) int {
	if n < 1 {
		return 0
	}
	return n*(Rows+Gap) - Gap
}

// PathsVertical is Paths for a display whose characters stand one under the
// other, top first — a legend set down the edge of a panel, where a vertical
// label went. The display is Cols by Height(n) dots; the characters are
// upright, as the stacked letters of a printed vertical label are.
func PathsVertical(text string, n int) (lit, dark string) {
	var l, d strings.Builder
	rs := []rune(text)
	for i := range n {
		var g [Rows]uint8
		if i < len(rs) {
			var ok bool
			if g, ok = font[rs[i]]; !ok {
				g = font['?']
			}
		}
		for row := range Rows {
			for col := range Cols {
				b := &d
				if g[row]&(1<<(Cols-1-col)) != 0 {
					b = &l
				}
				y := i*(Rows+Gap) + row
				b.WriteString("M" + strconv.Itoa(col) + ".1 " + strconv.Itoa(y) + ".1h.8v.8h-.8z")
			}
		}
	}
	return l.String(), d.String()
}
