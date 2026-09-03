//go:build js && wasm

package xtcell

import (
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/xterm-go/vt"
)

// The escape sequences are a detour.
//
// Removing the per-cell JavaScript call left three costs of roughly equal size
// in a full-screen frame, measured on a 210x49 terminal: 20ms setting the cells
// in tcell, 40ms turning the dirty ones into escape sequences, and 33ms in the
// emulator parsing those sequences back into cells. The middle and last of
// those are the same information being encoded and immediately decoded, and
// they exist only because that is how a terminal is normally reached.
//
// It is not normally reached like this. The emulator is written in Go and lives
// in this process, and its cells are three words in a slice. So when the
// terminal is one this package can see into, the dirty cells are written where
// they were going to end up anyway and the renderer is told which rows changed.
// A terminal that is not — the fake one the tests use — still gets sequences,
// which is also what keeps this optional rather than load-bearing.

// cellSink is a terminal whose cells can be written directly.
type cellSink interface {
	// putCell writes one cell. fg and bg are already in the emulator's packed
	// form; see packStyle.
	putCell(x, y int, str string, fg, bg uint32, width int)
	// refresh tells the renderer which viewport rows changed.
	refresh(y1, y2 int)
}

func (x *xtermTerm) putCell(col, row int, str string, fg, bg uint32, width int) {
	b := x.t.Core.Buffer()
	line := b.Lines.Get(b.YBase + row)
	if line == nil {
		return
	}

	// One AttributeData, reused. SetCellFromCodepoint copies the two words out
	// of it rather than keeping the pointer, so there is nothing to allocate
	// per cell — which is the whole point of coming this way.
	x.attr.Fg, x.attr.Bg = fg, bg

	r, size := utf8.DecodeRuneInString(str)
	if r == utf8.RuneError && size <= 1 {
		r = ' '
	}
	if width < 1 {
		width = 1
	}
	line.SetCellFromCodepoint(col, uint32(r), width, x.attr)

	// A wide rune owns the cell to its right as well. The emulator marks that
	// one as a zero-width continuation; without it the cell keeps whatever it
	// held and the glyph is drawn over stale content.
	if width == 2 && col+1 < line.Length {
		line.SetCellFromCodepoint(col+1, 0, 0, x.attr)
	}

	// Combining marks attach to the cell just written.
	for _, c := range str[size:] {
		line.AddCodepointToCell(col, uint32(c), 0)
	}
}

func (x *xtermTerm) refresh(y1, y2 int) {
	if f := x.t.Core.OnRefreshRows; f != nil {
		f(y1, y2)
	}
}

// packStyle renders a tcell style into the emulator's two attribute words.
//
// The layout is xterm.js's: the top two bits of each word are the colour mode
// and the low 24 are the colour, with the remaining bits carrying the
// attributes, split across the two words for no reason other than that there
// were spare bits in each. A colour mode of zero means "the terminal's
// default", which is what an unset tcell colour should become.
func packStyle(style tcell.Style) (fg, bg uint32) {
	// v3 reads a style through accessors rather than taking it apart with
	// Decompose, and carries underline as a style of its own rather than as a
	// bit in the attribute mask.
	fgc, bgc := style.GetForeground(), style.GetBackground()
	attrs := style.GetAttributes()
	if fgc.Valid() {
		fg = vt.AttrCMRGB | (uint32(fgc.Hex()) & vt.AttrRGBMask)
	}
	if bgc.Valid() {
		bg = vt.AttrCMRGB | (uint32(bgc.Hex()) & vt.AttrRGBMask)
	}

	if attrs&tcell.AttrBold != 0 {
		fg |= vt.FgBold
	}
	if style.HasUnderline() {
		fg |= vt.FgUnderline
	}
	if attrs&tcell.AttrBlink != 0 {
		fg |= vt.FgBlink
	}
	if attrs&tcell.AttrReverse != 0 {
		fg |= vt.FgInverse
	}
	if attrs&tcell.AttrStrikeThrough != 0 {
		fg |= vt.FgStrikethrough
	}
	if attrs&tcell.AttrItalic != 0 {
		bg |= vt.BgItalic
	}
	if attrs&tcell.AttrDim != 0 {
		bg |= vt.BgDim
	}
	return fg, bg
}

// drawDirect is draw's other half: same walk over the dirty cells, but each one
// is written into the emulator instead of described to it.
//
// Called with the lock held, and only after anything already queued as
// sequences has gone out — a queued clear-screen arriving after these cells
// would erase them.
func (s *Screen) drawDirect() {
	first, last := -1, -1
	for y := 0; y < s.rows; y++ {
		for x := 0; x < s.cols; x++ {
			if !s.cells.Dirty(x, y) {
				continue
			}
			str, style, width := s.cells.Get(x, y)
			if width < 1 {
				width = 1
			}
			if str == "" {
				str = " "
			}
			if len(s.fallback) > 0 {
				if r, _ := utf8.DecodeRuneInString(str); r != utf8.RuneError {
					if sub, ok := s.fallback[r]; ok {
						str = sub
					}
				}
			}
			fg, bg := packStyle(style)
			s.sink.putCell(x, y, str, fg, bg, width)

			s.cells.SetDirty(x, y, false)
			for i := 1; i < width && x+i < s.cols; i++ {
				s.cells.SetDirty(x+i, y, false)
			}
			x += width - 1

			if first < 0 {
				first = y
			}
			last = y
		}
	}
	if first < 0 {
		// Nothing changed. The cursor may still have, and that goes out as a
		// sequence either way.
		return
	}
	s.sink.refresh(first, last)
}
