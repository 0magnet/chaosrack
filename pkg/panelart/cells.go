package panelart

import (
	"image"
	"image/color"
)

// HalfBlock is U+2580, UPPER HALF BLOCK: the top half painted in the
// foreground color and the bottom half left as the background, which makes
// one cell two stacked pixels.
const HalfBlock = '▀'

// Cell is one character cell of a rendered image: the glyph is always
// HalfBlock, so only the two colors vary.
type Cell struct {
	Top, Bottom color.RGBA
}

// DefaultCellAspect is how much taller a character cell is than it is wide,
// when nobody has measured.
//
// Two is the right guess and not a safe assumption. Console fonts are usually
// exactly 1:2 — 8x16, 9x18, 10x20 — but a terminal draws with whatever face it
// was given at whatever line height, and the real numbers vary widely: 6.6x11
// (1.67) in a browser's default monospace, 5.39x12.8 (2.37) in xterm-go at a
// nine-pixel font. The gap is not subtle. Sampling a circle at 1.67 when the
// cell is really 2.37 draws a visible vertical ellipse, which is exactly what
// the first two attempts at this did.
//
// So: measure where you can, and pass it in. This is only the fallback.
const DefaultCellAspect = 2.0

// cellAspect is what the drawing actually uses.
var cellAspect = DefaultCellAspect

// SetCellAspect tells the package the real shape of a cell. A value that is
// not positive restores the default, so a caller that could not measure can
// pass what it got without checking.
func SetCellAspect(a float64) {
	if a <= 0 {
		a = DefaultCellAspect
	}
	cellAspect = a
}

// CellAspect is the shape in use.
func CellAspect() float64 { return cellAspect }

// RowsFor is how many character rows an image of the given pixel size needs
// to keep its shape, at a cell of the given aspect.
//
// The arithmetic, once, because getting it wrong is invisible in the code and
// obvious on the screen. A cell is two pixels tall and one wide, so the
// image's pixels are square only when the vertical sample rate is aspect/2
// times the horizontal one.
func RowsFor(w, h, cols int, aspect float64) int {
	if w <= 0 || h <= 0 || cols <= 0 {
		return 0
	}
	if aspect <= 0 {
		aspect = cellAspect
	}
	// Rounded, not truncated. A 10-column knob wants 10/1.67 = 5.99 rows;
	// truncating gives 5, which is 55 screen pixels against 66 wide, and the
	// circle comes out as a visible vertical ellipse. This one call is the
	// whole reason the first prototype's knobs were egg-shaped.
	rows := int(float64(cols)*float64(h)/float64(w)/aspect + 0.5)
	if rows < 1 {
		rows = 1
	}
	return rows
}

// Render samples an image into cols x rows cells, each cell averaging the
// source pixels its top half covers and, separately, its bottom half.
//
// Box averaging rather than nearest: a knob's pointer is one pixel wide at
// these sizes, and nearest-neighbor makes it flicker in and out as the value
// changes, which reads as a fault in the instrument rather than as a knob
// being turned.
func Render(src image.Image, cols, rows int) []Cell {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil
	}
	subRows := rows * 2
	out := make([]Cell, cols*rows)
	at := func(cx, sy int) color.RGBA {
		x0 := b.Min.X + cx*b.Dx()/cols
		x1 := b.Min.X + (cx+1)*b.Dx()/cols
		y0 := b.Min.Y + sy*b.Dy()/subRows
		y1 := b.Min.Y + (sy+1)*b.Dy()/subRows
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}
		var r, g, bl, n uint32
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				cr, cg, cb, _ := src.At(x, y).RGBA()
				r += cr >> 8
				g += cg >> 8
				bl += cb >> 8
				n++
			}
		}
		if n == 0 {
			return color.RGBA{A: 255}
		}
		return color.RGBA{R: byteOf(r / n), G: byteOf(g / n), B: byteOf(bl / n), A: 255}
	}
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			out[cy*cols+cx] = Cell{Top: at(cx, cy*2), Bottom: at(cx, cy*2+1)}
		}
	}
	return out
}

// KnobCells is the whole of what a caller needs: a knob at cols wide, already
// in cells.
//
// The pixel buffer is made at two pixels per subcell rather than one. Drawing
// a 10-pixel circle and sampling it 1:1 gives a circle made of ten pixels,
// which is a stair; drawing it at 20 and averaging down gives the same circle
// with its edge spread over the cells it actually passes through, and at
// these sizes that difference is most of what makes it look round.
func KnobCells(cols int, frac float64, detents int, p Palette) (cells []Cell, rows int) {
	if cols < 1 {
		return nil, 0
	}
	rows = RowsFor(cols, cols, cols, cellAspect)
	// The buffer is SQUARE and the sample grid is not, which is the right way
	// round: the knob is a true circle in a square area, and Render maps that
	// area onto cols x 2*rows subcells, each of which is the shape a subcell
	// actually is on screen. Drawing an ellipse to compensate would be doing
	// the same correction twice.
	//
	// ss oversamples so the box average has something to average. The
	// vertical sample rate is the higher of the two (2/cellAspect), so
	// 4 rather than 2 keeps even that side supersampled at small sizes, and
	// the buffer is still only a few thousand pixels.
	const ss = 4
	size := cols * ss
	im := Knob(size, frac, detents, p)
	return Render(im, cols, rows), rows
}

// byteOf narrows a channel average. The sum of n values of at most 255,
// divided by n, cannot exceed 255 — but that is an argument, not a proof the
// compiler can check, and a silent wrap here would show up as a bright pixel
// in the middle of a dark panel.
func byteOf(v uint32) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// LampCells is a lamp at cols wide, already in cells — the switch's
// counterpart to KnobCells.
//
// A switch is not a dial with two detents. It has a state, and a panel shows
// a state with a lamp: lit or dark, no pointer to read. Drawing it as a dial
// would put a knob where the instrument has a toggle.
func LampCells(cols int, on bool, p Palette) (cells []Cell, rows int) {
	if cols < 1 {
		return nil, 0
	}
	rows = RowsFor(cols, cols, cols, cellAspect)
	const ss = 4
	return Render(Lamp(cols*ss, on, p), cols, rows), rows
}
