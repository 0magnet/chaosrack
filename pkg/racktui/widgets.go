package racktui

import (
	"fmt"
	"strconv"
)

// What a control SAYS, as opposed to what it looks like.
//
// The looking is pkg/panelart's now. This file used to hold a second set of
// widgets that drew a knob out of box-drawing characters and an arrow —
// drawKnob, drawLamp, drawDetents — and they are gone: a dial rasterized from
// its own value and blitted as half blocks is the same thing done properly,
// and keeping a glyph version beside it would be a second answer to "which way
// is this knob pointing".
//
// What is left is the arithmetic, which no renderer should each have its own
// copy of.

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
