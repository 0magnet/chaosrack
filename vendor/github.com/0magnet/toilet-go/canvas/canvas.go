// Package canvas supplies the libcaca canvas operations that toilet needs and
// that github.com/0magnet/img2txt-go/caca does not already provide: the
// content-preserving resize, the character and attribute accessors, blitting
// with a handle, cropping, the box and line primitives, the flip and rotate
// transforms and the UTF-8/ANSI canvas importer.
//
// Everything here follows libcaca 0.99.beta20 (canvas.c, string.c, attr.c,
// box.c, line.c, transform.c and codec/text.c). The cell storage itself is the
// one from the img2txt-go port, so the two agree on attributes and on every
// export codec.
//
// Original C implementation copyright © 2002—2021 Sam Hocevar <sam@hocevar.net>,
// released under the WTFPL.
package canvas

import "github.com/0magnet/img2txt-go/caca"

// Canvas is a libcaca canvas. It embeds the img2txt-go cell store and adds the
// per-frame cursor and handle that libcaca keeps alongside it.
type Canvas struct {
	*caca.Canvas

	// X and Y are the frame cursor. The UTF-8 importer reads it on entry and
	// writes it back on exit, which is how libcaca resumes a partial import.
	X, Y int

	// HandleX and HandleY are the frame handle, subtracted from the
	// destination coordinates when this canvas is used as a blit source.
	HandleX, HandleY int
}

// New returns a canvas of the given size. Like caca_create_canvas() it starts
// with the default foreground on a transparent background, so every cell is a
// space carrying that attribute.
func New(w, h int) *Canvas {
	cv := &Canvas{Canvas: &caca.Canvas{}}
	cv.SetColorANSI(caca.Default, caca.Transparent)
	cv.SetSize(w, h)
	return cv
}

// SetSize resizes the canvas, preserving as much of its contents as fits. This
// is caca_resize(); it shadows the img2txt-go method of the same name, which
// clears the canvas instead. The figfont renderer grows its canvas glyph by
// glyph and depends on the contents surviving.
func (cv *Canvas) SetSize(width, height int) {
	if width < 0 || height < 0 {
		return
	}

	oldW, oldH := cv.Width, cv.Height
	oldSize := oldW * oldH
	newSize := width * height

	chars, attrs := cv.Chars, cv.Attrs
	attr := cv.Attr()

	// Step 1: if the new area is bigger, grow the buffers first.
	if newSize > oldSize {
		chars = append(chars, make([]rune, newSize-oldSize)...)
		attrs = append(attrs, make([]uint32, newSize-oldSize)...)
	}

	// Step 2: move line data if the stride changed.
	switch {
	case width == oldW:
		// Nothing to do: the rows are already where they belong.
	case width > oldW:
		// Copy from the bottom up, so a row never overwrites the next.
		lines := height
		if oldH < lines {
			lines = oldH
		}
		for y := lines; y > 0; {
			y--
			for x := oldW; x > 0; {
				x--
				chars[y*width+x] = chars[y*oldW+x]
				attrs[y*width+x] = attrs[y*oldW+x]
			}
			for x := width - oldW; x > 0; {
				x--
				chars[y*width+oldW+x] = ' '
				attrs[y*width+oldW+x] = attr
			}
		}
	default:
		// The new width is smaller. Row zero is already in place.
		lines := height
		if oldH < lines {
			lines = oldH
		}
		for y := 1; y < lines; y++ {
			for x := 0; x < width; x++ {
				chars[y*width+x] = chars[y*oldW+x]
				attrs[y*width+x] = attrs[y*oldW+x]
			}
		}
	}

	// Step 3: blank the bottom of the new canvas.
	if height > oldH {
		for x := (height - oldH) * width; x > 0; {
			x--
			chars[oldH*width+x] = ' '
			attrs[oldH*width+x] = attr
		}
	}

	// Step 4: if the new area is smaller, shrink the buffers now.
	if newSize < oldSize {
		chars = chars[:newSize]
		attrs = attrs[:newSize]
	}

	cv.Chars, cv.Attrs = chars, attrs
	cv.Width, cv.Height = width, height

	// libcaca clamps the frame cursor with a strict comparison, so a resize
	// to zero pulls it back to the origin.
	if cv.X > width {
		cv.X = width
	}
	if cv.Y > height {
		cv.Y = height
	}
}

// GetChar returns the character at the given cell, or a space if the
// coordinates fall outside the canvas.
func (cv *Canvas) GetChar(x, y int) rune {
	if x < 0 || x >= cv.Width || y < 0 || y >= cv.Height {
		return ' '
	}
	return cv.Chars[x+y*cv.Width]
}

// GetAttr returns the attribute of the given cell, or the current attribute if
// the coordinates fall outside the canvas.
func (cv *Canvas) GetAttr(x, y int) uint32 {
	if x < 0 || x >= cv.Width || y < 0 || y >= cv.Height {
		return cv.Attr()
	}
	return cv.Attrs[x+y*cv.Width]
}

// PutChar writes a character with the current attribute and returns the number
// of cells it took, one or two. It shadows the img2txt-go method, which does
// not know about fullwidth glyphs.
func (cv *Canvas) PutChar(x, y int, ch rune) int {
	if uint32(ch) == caca.MagicFullwidth {
		return 1
	}

	fullwidth := caca.IsFullwidth(ch)
	ret := 1
	if fullwidth {
		ret = 2
	}

	if x >= cv.Width || y < 0 || y >= cv.Height {
		return ret
	}

	if x == -1 && fullwidth {
		x, ch, fullwidth = 0, ' ', false
	} else if x < 0 {
		return ret
	}

	i := x + y*cv.Width
	attr := cv.Attr()

	// Overwriting the right half of a fullwidth glyph blanks its left half.
	if x != 0 && uint32(cv.Chars[i]) == caca.MagicFullwidth {
		cv.Chars[i-1] = ' '
	}

	if fullwidth {
		if x+1 == cv.Width {
			ch = ' '
		} else {
			// Overwriting the left half blanks the right half.
			if x+2 < cv.Width && uint32(cv.Chars[i+2]) == caca.MagicFullwidth {
				cv.Chars[i+2] = ' '
			}
			cv.Chars[i+1] = rune(caca.MagicFullwidth)
			cv.Attrs[i+1] = attr
		}
	} else if x+1 != cv.Width && uint32(cv.Chars[i+1]) == caca.MagicFullwidth {
		cv.Chars[i+1] = ' '
	}

	cv.Chars[i] = ch
	cv.Attrs[i] = attr

	return ret
}

// PutAttr sets the attribute of one cell. Values below 0x10 replace the style
// bits only, as in caca_put_attr().
func (cv *Canvas) PutAttr(x, y int, attr uint32) {
	if x < 0 || x >= cv.Width || y < 0 || y >= cv.Height {
		return
	}

	i := x + y*cv.Width
	if attr < 0x00000010 {
		cv.Attrs[i] = (cv.Attrs[i] & 0xfffffff0) | attr
	} else {
		cv.Attrs[i] = attr
	}

	// Keep both halves of a fullwidth glyph in step.
	if x != 0 && uint32(cv.Chars[i]) == caca.MagicFullwidth {
		cv.Attrs[i-1] = cv.Attrs[i]
	} else if x+1 < cv.Width && uint32(cv.Chars[i+1]) == caca.MagicFullwidth {
		cv.Attrs[i+1] = cv.Attrs[i]
	}
}

// SetAttr sets the current attribute. Values below 0x10 replace the style bits
// only, as in caca_set_attr().
func (cv *Canvas) SetAttr(attr uint32) {
	if attr < 0x00000010 {
		attr = (cv.Attr() & 0xfffffff0) | attr
	}
	cv.Canvas.SetAttr(attr)
}

// SetHandle moves the blit handle of this canvas.
func (cv *Canvas) SetHandle(x, y int) {
	cv.HandleX, cv.HandleY = x, y
}

// Blit copies src onto cv at the given coordinates, offset by the source
// canvas' handle.
func (cv *Canvas) Blit(x, y int, src *Canvas) {
	x -= src.HandleX
	y -= src.HandleY

	starti, startj := 0, 0
	if x < 0 {
		starti = -x
	}
	if y < 0 {
		startj = -y
	}

	endi, endj := src.Width, src.Height
	if x+src.Width >= cv.Width {
		endi = cv.Width - x
	}
	if y+src.Height >= cv.Height {
		endj = cv.Height - y
	}
	stride := endi - starti

	if starti > src.Width || startj > src.Height || starti >= endi || startj >= endj {
		return
	}

	for j := startj; j < endj; j++ {
		dstix := (j+y)*cv.Width + starti + x
		srcix := j*src.Width + starti

		if starti+x != 0 && uint32(cv.Chars[dstix]) == caca.MagicFullwidth {
			cv.Chars[dstix-1] = ' '
		}
		if endi+x < cv.Width && uint32(cv.Chars[dstix+stride]) == caca.MagicFullwidth {
			cv.Chars[dstix+stride] = ' '
		}

		copy(cv.Chars[dstix:dstix+stride], src.Chars[srcix:srcix+stride])
		copy(cv.Attrs[dstix:dstix+stride], src.Attrs[srcix:srcix+stride])

		// Fix split fullwidth characters. The second test reads row zero of
		// the source rather than the current row; that is what libcaca does,
		// and it is reproduced here rather than corrected.
		if uint32(src.Chars[srcix]) == caca.MagicFullwidth {
			cv.Chars[dstix] = ' '
		}
		if endi < src.Width && uint32(src.Chars[endi]) == caca.MagicFullwidth {
			cv.Chars[dstix+stride-1] = ' '
		}
	}
}

// SetBoundaries crops or expands the canvas to the given rectangle of its own
// coordinate space. Negative x and y grow it on the left and top.
func (cv *Canvas) SetBoundaries(x, y, w, h int) {
	if w < 0 || h < 0 {
		return
	}

	dst := New(w, h)
	dst.Blit(-x, -y, cv)

	cv.Chars, cv.Attrs = dst.Chars, dst.Attrs
	cv.Width, cv.Height = dst.Width, dst.Height

	// libcaca moves the new canvas' frames into the old handle and then
	// reloads the frame shortcuts from them, so the current attribute, cursor
	// and handle are the fresh canvas' rather than whatever was in force
	// before. The border filter draws its box right after this call and picks
	// the attribute up.
	cv.Canvas.SetAttr(dst.Attr())
	cv.X, cv.Y = dst.X, dst.Y
	cv.HandleX, cv.HandleY = dst.HandleX, dst.HandleY
}

// DrawLine draws a straight line of the given character. Only the horizontal
// and vertical cases toilet needs are implemented; libcaca's Bresenham
// rasteriser handles the diagonals it never asks for.
func (cv *Canvas) DrawLine(x1, y1, x2, y2 int, ch rune) {
	dx, dy := x2-x1, y2-y1
	steps := abs(dx)
	if abs(dy) > steps {
		steps = abs(dy)
	}

	if steps == 0 {
		cv.PutChar(x1, y1, ch)
		return
	}

	for i := 0; i <= steps; i++ {
		cv.PutChar(x1+dx*i/steps, y1+dy*i/steps, ch)
	}
}

// abs returns the absolute value of an int.
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// FillBox fills a rectangle with the given character.
func (cv *Canvas) FillBox(x, y, w, h int, ch rune) {
	x2, y2 := x+w-1, y+h-1
	if x > x2 {
		x, x2 = x2, x
	}
	if y > y2 {
		y, y2 = y2, y
	}

	xmax, ymax := cv.Width-1, cv.Height-1
	if x2 < 0 || y2 < 0 || x > xmax || y > ymax {
		return
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x2 > xmax {
		x2 = xmax
	}
	if y2 > ymax {
		y2 = ymax
	}

	for j := y; j <= y2; j++ {
		for i := x; i <= x2; i++ {
			cv.PutChar(i, j, ch)
		}
	}
}

// cp437Box holds the light box-drawing set caca_draw_cp437_box() uses:
// horizontal, vertical, then the four corners.
var cp437Box = [6]rune{0x2500, 0x2502, 0x250c, 0x2514, 0x2510, 0x2518}

// DrawCP437Box draws a box with the CP437 line-drawing characters.
func (cv *Canvas) DrawCP437Box(x, y, w, h int) {
	chars := cp437Box

	x2, y2 := x+w-1, y+h-1
	if x > x2 {
		x, x2 = x2, x
	}
	if y > y2 {
		y, y2 = y2, y
	}

	xmax, ymax := cv.Width-1, cv.Height-1
	if x2 < 0 || y2 < 0 || x > xmax || y > ymax {
		return
	}

	first := x + 1
	if x < 0 {
		first = 1
	}

	if y >= 0 {
		for i := first; i < x2 && i < xmax; i++ {
			cv.PutChar(i, y, chars[0])
		}
	}
	if y2 <= ymax {
		for i := first; i < x2 && i < xmax; i++ {
			cv.PutChar(i, y2, chars[0])
		}
	}

	firstj := y + 1
	if y < 0 {
		firstj = 1
	}

	if x >= 0 {
		for j := firstj; j < y2 && j < ymax; j++ {
			cv.PutChar(x, j, chars[1])
		}
	}
	if x2 <= xmax {
		for j := firstj; j < y2 && j < ymax; j++ {
			cv.PutChar(x2, j, chars[1])
		}
	}

	cv.PutChar(x, y, chars[2])
	cv.PutChar(x, y2, chars[3])
	cv.PutChar(x2, y, chars[4])
	cv.PutChar(x2, y2, chars[5])
}
