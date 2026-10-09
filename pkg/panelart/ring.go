package panelart

import (
	"image/color"
	"math"
)

// The P-unit's encoder, in Braille.
//
// A P-unit's knob is endless: it has no pointer, and what says where it is is
// the LED ring round it. That makes it the one control a terminal can draw
// better than a dial. A ring of LEDs is a ring of DOTS, and Braille is a 2x4
// grid of dots in every cell, so the ring is drawn in dots — as the panel's
// is — rather than as a rasterized circle that needs twelve columns before it
// reads as one.
//
// A Braille cell has one color, so a cell holding both a lit dot and an unlit
// one shows only the lit one. The ring loses a dim dot here and there and
// never shows a lit one as dark, which is the error that would lie.

// Glyph is one character cell of a drawing that is not half blocks: a rune in
// a color on a color.
type Glyph struct {
	Ch     rune
	Fg, Bg color.RGBA
}

// RingRows is how many rows a ring cols wide needs to come out round.
func RingRows(cols int) int {
	return max(int(float64(cols)/cellAspect+0.5), 1)
}

// Ring draws a P-unit's encoder cols x rows: the LED ring lit from the start
// of the sweep up to frac, round a knob body. With detents > 1 the ring is a
// selector's instead: only the stretch round the current position is lit,
// which is how the panel shows a selector or a switch on the same part.
func Ring(cols, rows int, frac float64, detents int, p Palette) []Glyph {
	dw, dh := 2*cols, 4*rows // the dot grid
	type dot uint8
	const (
		none dot = iota
		off
		body
		on
	)
	grid := make([]dot, dw*dh)
	// A dot is half a cell wide and a quarter of one tall, so in cell widths
	// it is 0.5 x aspect/4. Everything round is drawn in those units.
	sx, sy := 0.5, cellAspect/4
	cx, cy := float64(dw-1)/2*sx, float64(dh-1)/2*sy
	r := math.Min(float64(dw)*sx, float64(dh)*sy)/2 - sy/2
	set := func(x, y float64, d dot) {
		ix, iy := int(math.Round(x/sx)), int(math.Round(y/sy))
		if ix < 0 || iy < 0 || ix >= dw || iy >= dh {
			return
		}
		if grid[iy*dw+ix] < d {
			grid[iy*dw+ix] = d
		}
	}
	// The body: every dot well inside the ring.
	for iy := range dh {
		for ix := range dw {
			if math.Hypot(float64(ix)*sx-cx, float64(iy)*sy-cy) <= r*0.55 {
				grid[iy*dw+ix] = body
			}
		}
	}
	frac = clamp01(frac)
	// The track, every dot round the sweep: separate lamps are specks at a
	// terminal's size, and a continuous ring lit to the value is what the eye
	// reads off the page's anyway. A selector lights the stretch round its
	// position rather than everything up to it.
	const steps = 240
	half := 0.0
	if detents > 1 {
		half = 0.5 / float64(detents-1)
	}
	for i := range steps + 1 {
		t := float64(i) / steps
		lit := frac > 0 && t <= frac+1e-9
		if detents > 1 {
			pos := math.Round(frac*float64(detents-1)) / float64(detents-1)
			lit = math.Abs(t-pos) <= half*0.8
		}
		d := off
		if lit {
			d = on
		}
		a := KnobAngle(t)
		set(cx+r*math.Cos(a), cy+r*math.Sin(a), d)
	}

	out := make([]Glyph, cols*rows)
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	for row := range rows {
		for col := range cols {
			var top dot
			for y := range 4 {
				for x := range 2 {
					top = max(top, grid[(row*4+y)*dw+col*2+x])
				}
			}
			g := Glyph{Ch: ' ', Bg: p.Panel}
			var ch rune
			for y := range 4 {
				for x := range 2 {
					d := grid[(row*4+y)*dw+col*2+x]
					// Only the dots of the material the cell is drawn in,
					// except that the unlit lamps ride along with the body:
					// both are dim, and a grey lamp still reads as a lamp.
					if d == top || (top == body && d == off) {
						if d != none {
							ch |= bits[y][x]
						}
					}
				}
			}
			if ch != 0 {
				g.Ch = 0x2800 + ch
			}
			switch top {
			case on:
				g.Fg = p.LEDOn
			case body:
				g.Fg = p.Shadow
			case off:
				g.Fg = scale(p.LEDOn, 0.45)
			}
			out[row*cols+col] = g
		}
	}
	return out
}
