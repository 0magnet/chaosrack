package racktui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Drawing a control as the control, rather than as a row in a table.
//
// A rack is not a settings list. Its whole argument is that a knob you can see
// the position of, beside the reading it produces, is a different instrument
// from a field with a number in it — which is the argument every panel in
// chaosrack makes and the one the terminal panel was not making while it drew
// a table.
//
// A terminal can do this. It has 24-bit color, a cell grid, and enough of
// Unicode to draw a dial: the pointer is one of eight glyphs around a ring,
// which is exactly the resolution a real pointer knob is read at from arm's
// length. What it cannot do is sub-cell placement, so the dial is drawn at the
// size the grid allows rather than the size that would look best — three rows,
// five columns, and the indicator lands on one of eight compass points.

// knobPointer is the pointer at each of eight positions, anticlockwise-most
// first, running the way a panel pot sweeps: about 270 degrees with the dead
// zone at the bottom, not a full circle.
//
// ARROWS AND NOT GEOMETRIC SHAPES, which is a lesson from drawing this on the
// rack's own terminal. The obvious set — ◟ ◁ ◤ △ ◥ ▷ ◞ ▽ placed around the
// ring — is prettier and renders as BLANK in the monospace stack a browser
// terminal falls back to, so every dial came up as an empty box. Arrows are in
// every font that has ever been used for a terminal, and they say the same
// thing from one cell: the direction the pointer is pointing.
var knobPointer = [8]rune{'↙', '←', '↖', '↑', '↗', '→', '↘', '↓'}

func drawKnob(frac float64) []string {
	if math.IsNaN(frac) || frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	rows := [][]rune{
		[]rune("╭───╮"),
		[]rune("│   │"),
		[]rune("╰───╯"),
	}
	// Seven steps between eight positions, so both ends are reachable.
	i := int(frac*7 + 0.5)
	if i > 7 {
		i = 7
	}
	// One cell, at the middle of the dial, where a pointer knob has its
	// pointer. A ring of eight positions would need sub-cell placement the
	// grid does not have.
	rows[1][2] = knobPointer[i]
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = string(r)
	}
	return out
}

// drawLamp is a switch: lit or dark, with its name beside it.
func drawLamp(on bool, label string) string {
	if on {
		return "◉ " + label
	}
	return "○ " + label
}

// drawDetents is a rotary switch's positions, the chosen one marked. A switch
// with a handful of stops reads better as its stops than as a dial: the point
// of a detent is that you can see which one you are on.
func drawDetents(opts []string, cur string, width int) string {
	if len(opts) == 0 {
		return cur
	}
	var b strings.Builder
	for _, o := range opts {
		mark := "·"
		if o == cur {
			mark = "▪"
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(mark + o)
	}
	return clip(b.String(), width)
}

// ledText is a reading, in the rack's own house style: right-aligned in a
// fixed field so the digits do not move as the value changes, which is the
// whole reason a real meter has a fixed number of them.
func ledText(v string, width int) string {
	if v == "" {
		v = "--"
	}
	if len(v) > width {
		v = v[len(v)-width:]
	}
	return strings.Repeat(" ", width-len(v)) + v
}

// fracOf is where a control's value sits in its travel, 0..1, for the dial.
// A control with no range has none and reads as fully anticlockwise.
func fracOf(c Control) float64 {
	if c.IsSelect {
		if len(c.Options) < 2 {
			return 0
		}
		for i, o := range c.Options {
			if o == c.Value {
				return float64(i) / float64(len(c.Options)-1)
			}
		}
		return 0
	}
	if c.Max == c.Min {
		return 0
	}
	v, err := strconv.ParseFloat(c.Value, 64)
	if err != nil {
		v = c.Def
	}
	return (v - c.Min) / (c.Max - c.Min)
}

// shortNum trims a value for a narrow LED without lying about it: a number
// keeps its significant end, which for a reading is the digits.
func shortNum(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		for _, prec := range []int{3, 2, 1, 0} {
			t := strconv.FormatFloat(f, 'f', prec, 64)
			if len(t) <= width {
				return t
			}
		}
		return fmt.Sprintf("%.*g", width-2, f)
	}
	return s[:width]
}

// padRunes pads to w CELLS, not bytes: every glyph the panel draws with is
// multi-byte, so a byte-wise pad leaves the columns ragged.
func padRunes(s string, w int) string {
	n := len([]rune(s))
	if n >= w {
		return string([]rune(s)[:w])
	}
	return s + strings.Repeat(" ", w-n)
}
