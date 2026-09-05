package figlet

import (
	"strings"

	"github.com/0magnet/toilet-go/canvas"
)

// Mode selects how far consecutive glyphs are allowed to move into each other.
type Mode int

// The layout modes, in the order libcaca numbers them.
const (
	// ModeDefault defers to the font's own layout fields, resolving to one of
	// the modes below.
	ModeDefault Mode = iota
	// ModeKern moves glyphs together until their inked columns touch.
	ModeKern
	// ModeSmush lets touching columns merge under the font's smushing rules.
	ModeSmush
	// ModeNone leaves every glyph at its full width.
	ModeNone
	// ModeOverlap moves glyphs one column past touching, the later glyph
	// covering the earlier.
	ModeOverlap
)

// ParseMode maps toilet's mode names onto a Mode. Anything unrecognized is
// ModeDefault, as in caca_set_figfont_smush().
func ParseMode(name string) Mode {
	switch strings.ToLower(name) {
	case "kern":
		return ModeKern
	case "smush":
		return ModeSmush
	case "none":
		return ModeNone
	case "overlap":
		return ModeOverlap
	default:
		return ModeDefault
	}
}

// Renderer lays glyphs of one font out on a canvas. Feed it characters with
// PutChar and take a finished line off it with Flush.
type Renderer struct {
	font   *Font
	cv     *canvas.Canvas
	charcv *canvas.Canvas

	termWidth int
	mode      Mode
	rule      int

	x, y, w, h int
	// Lines counts the rows flushed so far. libcaca keeps this counter so that
	// a color effect can run on across a multi-line render; toilet's filters
	// read a second counter of their own, which never advances, so nothing in
	// the command uses this one.
	Lines int
}

// NewRenderer returns a renderer for the given font, in toilet's default 80
// column width and with the font's own layout mode.
func NewRenderer(f *Font) *Renderer {
	r := &Renderer{font: f, cv: canvas.New(0, 0), termWidth: 80}
	r.update()
	return r
}

// SetWidth sets the column at which the render wraps onto a new row.
func (r *Renderer) SetWidth(w int) {
	r.termWidth = w
	r.update()
}

// SetMode overrides the layout mode the font asks for.
func (r *Renderer) SetMode(m Mode) {
	r.mode = m
	r.update()
}

// Mode returns the mode in force, with ModeDefault already resolved against the
// font's layout fields.
func (r *Renderer) Mode() Mode { return r.mode }

// Rule returns the smushing rule bitfield in force.
func (r *Renderer) Rule() int { return r.rule }

// update resolves the layout mode and rebuilds the scratch glyph canvas. It is
// caca's update_figfont_settings().
func (r *Renderer) update() {
	f := r.font

	if f.FullLayout&0x3f != 0 {
		r.rule = f.FullLayout & 0x3f
	} else if f.OldLayout > 0 {
		r.rule = f.OldLayout
	}

	if r.mode == ModeDefault {
		switch {
		case f.OldLayout == -1:
			r.mode = ModeNone
		case f.OldLayout == 0 && f.FullLayout&0xc0 == 0x40:
			r.mode = ModeKern
		case f.OldLayout&0x3f != 0 && f.FullLayout&0x3f != 0 && f.FullLayout&0x80 != 0:
			r.mode = ModeSmush
			r.rule = f.FullLayout & 0x3f
		case f.OldLayout == 0 && f.FullLayout&0xbf == 0x80:
			r.mode = ModeSmush
			r.rule = 0x3f
		default:
			r.mode = ModeOverlap
		}
	}

	r.charcv = canvas.New(f.MaxLength-2, f.Height)
}

// PutChar renders one character onto the current line. Characters the font does
// not define are skipped.
func (r *Renderer) PutChar(ch rune) {
	switch ch {
	case '\r':
		return
	case '\n':
		r.x = 0
		r.y += r.font.Height
		return
	}

	f := r.font
	c := f.index(ch)
	if c < 0 {
		return
	}

	w := f.glyphs[c].width
	h := f.Height

	// Lift the glyph out of the font canvas by moving the handle onto its
	// first row, then blitting into the scratch canvas.
	f.cv.SetHandle(0, c*h)
	r.charcv.Blit(0, 0, f.cv)

	// Wrap if the glyph would run past the right margin.
	if r.x != 0 && r.x+w > r.termWidth {
		r.x = 0
		r.y += h
	}

	overlap := r.overlapFor(w, h)

	if r.x+w-overlap > r.w {
		r.w = r.x + w - overlap
		if r.w > r.termWidth {
			r.w = r.termWidth
		}
	}
	if r.y+h > r.h {
		r.h = r.y + h
	}
	r.cv.SetSize(r.w, r.h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ch2 := r.charcv.GetChar(x, y)
			if ch2 == ' ' {
				continue
			}
			ch1 := r.cv.GetChar(r.x+x-overlap, r.y+y)
			if ch1 == ' ' || r.mode != ModeSmush {
				r.cv.PutChar(r.x+x-overlap, r.y+y, ch2)
			} else {
				r.cv.PutChar(r.x+x-overlap, r.y+y, Smush(ch1, ch2, r.rule))
			}
			// libcaca writes the glyph's attribute at the unshifted column,
			// not at the one the character landed in. Kept as it is: it is
			// what colors a smushed render.
			r.cv.PutAttr(r.x+x, r.y+y, f.cv.GetAttr(x, y+c*h))
		}
	}

	r.x += w - overlap
}

// overlapFor returns how many columns the incoming glyph may move left over the
// text already on the line.
func (r *Renderer) overlapFor(w, h int) int {
	if r.mode == ModeNone {
		return 0
	}

	overlap := w
	for y := 0; y < h; y++ {
		// Blank columns at the left of the new glyph.
		xright := 0
		for ; xright < overlap; xright++ {
			if r.charcv.GetChar(xright, y) != ' ' {
				break
			}
		}

		// Blank columns at the right of what is already there.
		xleft := 0
		for ; xright+xleft < overlap && xleft < r.x; xleft++ {
			if r.cv.GetChar(r.x-1-xleft, r.y+y) != ' ' {
				break
			}
		}

		// Overlapping eats one more column, whatever is in it.
		if r.mode == ModeOverlap && xleft < r.x {
			xleft++
		}

		// Smushing eats one more column when the two characters in it merge.
		if r.mode == ModeSmush && xleft < r.x {
			if Smush(r.cv.GetChar(r.x-1-xleft, r.y+y),
				r.charcv.GetChar(xright, y), r.rule) != 0 {
				xleft++
			}
		}

		if xleft+xright < overlap {
			overlap = xleft + xright
		}
	}

	return overlap
}

// Flush finishes the current line and returns it as a canvas of its own. The
// renderer is left ready for the next line.
func (r *Renderer) Flush() *canvas.Canvas {
	r.cv.SetSize(r.w, r.h)

	// Hardblanks have done their job of holding columns apart; turn them back
	// into spaces.
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			if r.cv.GetChar(x, y) == 0xa0 {
				attr := r.cv.GetAttr(x, y)
				r.cv.PutChar(x, y, ' ')
				r.cv.PutAttr(x, y, attr)
			}
		}
	}

	// The font stays attached to the working canvas, so the line is copied out
	// rather than handed over.
	out := canvas.New(r.cv.Width, r.cv.Height)
	out.Blit(0, 0, r.cv)

	r.Lines += r.cv.Height

	r.x, r.y = 0, 0
	r.w, r.h = 0, 0
	r.cv.SetSize(0, 0)

	return out
}
