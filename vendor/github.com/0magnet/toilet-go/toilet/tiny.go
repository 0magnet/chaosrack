package toilet

import "github.com/0magnet/toilet-go/canvas"

// tinyDriver is toilet's "term" font: one canvas cell per input character, with
// the input's own colors carried through. It is a port of src/term.c.
type tinyDriver struct {
	cv        *canvas.Canvas
	termWidth int

	x, y   int // cursor
	w, h   int // extent of the text written so far
	ew, eh int // allocated canvas size, grown by half when outgrown
}

// newTinyDriver returns the "term" renderer at the given width.
func newTinyDriver(termWidth int) *tinyDriver {
	return &tinyDriver{cv: canvas.New(0, 0), termWidth: termWidth, ew: 16, eh: 2}
}

// feed writes one character at the cursor.
func (d *tinyDriver) feed(ch rune, attr uint32) {
	switch ch {
	case '\r':
		return
	case '\n':
		d.x = 0
		d.y++
		return
	case '\t':
		d.x = (d.x &^ 7) + 8
		return
	}

	// Wrap at the right margin.
	if d.x != 0 && d.x+1 > d.termWidth {
		d.x = 0
		d.y++
	}

	if d.x+1 > d.w {
		d.w = d.x + 1
		if d.w > d.termWidth {
			d.w = d.termWidth
		}
		if d.w > d.ew {
			d.ew += d.ew / 2
		}
	}

	if d.y+1 > d.h {
		d.h = d.y + 1
		if d.h > d.eh {
			d.eh += d.eh / 2
		}
	}

	d.cv.SetAttr(attr)
	d.cv.SetSize(d.ew, d.eh)
	d.cv.PutChar(d.x, d.y, ch)
	d.x++
}

// flush finishes the line and starts a new canvas.
func (d *tinyDriver) flush() *canvas.Canvas {
	out := d.cv
	out.SetSize(d.w, d.h)

	d.ew, d.eh = 16, 2
	d.x, d.y = 0, 0
	d.w, d.h = 0, 0
	d.cv = canvas.New(d.ew, d.eh)

	return out
}
