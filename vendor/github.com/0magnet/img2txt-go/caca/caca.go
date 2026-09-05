// Package caca is a Go port of the parts of libcaca that img2txt uses: the
// dithering engine, the text canvas and the export codecs.
//
// It reproduces libcaca 0.99.beta20 output byte for byte.
//
// Original C implementation copyright © 2002—2021 Sam Hocevar <sam@hocevar.net>
// and Jean-Yves Lamoureux <jylam@lnxscene.org>, released under the WTFPL.
package caca

// Color indices, matching enum caca_color.
const (
	Black        = 0x00
	Blue         = 0x01
	Green        = 0x02
	Cyan         = 0x03
	Red          = 0x04
	Magenta      = 0x05
	Brown        = 0x06
	LightGray    = 0x07
	DarkGray     = 0x08
	LightBlue    = 0x09
	LightGreen   = 0x0a
	LightCyan    = 0x0b
	LightRed     = 0x0c
	LightMagenta = 0x0d
	Yellow       = 0x0e
	White        = 0x0f
	Default      = 0x10
	Transparent  = 0x20
)

// Style flags, matching enum caca_style.
const (
	StyleBold      = 0x01
	StyleItalics   = 0x02
	StyleUnderline = 0x04
	StyleBlink     = 0x08
)

// MagicFullwidth marks the cell following a fullwidth glyph.
const MagicFullwidth = 0x000ffffe

// ansitab16 holds the RGB values of the ANSI palette on 16 bits (4-4-4-4).
// These match gnome-terminal; entry 6 (brown) is deliberately 0xfa50.
var ansitab16 = [16]uint16{
	0xf000, 0xf00a, 0xf0a0, 0xf0aa, 0xfa00, 0xfa0a, 0xfa50, 0xfaaa,
	0xf555, 0xf55f, 0xf5f5, 0xf5ff, 0xff55, 0xff5f, 0xfff5, 0xffff,
}

// ansitab14 is the same palette on 14 bits (3-4-4-3).
var ansitab14 = [16]uint16{
	0x3800, 0x3805, 0x3850, 0x3855, 0x3d00, 0x3d05, 0x3d28, 0x3d55,
	0x3aaa, 0x3aaf, 0x3afa, 0x3aff, 0x3faa, 0x3faf, 0x3ffa, 0x3fff,
}

// Canvas is a grid of characters with per-cell attributes.
type Canvas struct {
	Width, Height int
	Chars         []rune
	Attrs         []uint32
	curattr       uint32
}

// NewCanvas returns a canvas of the given size.
func NewCanvas(w, h int) *Canvas {
	cv := &Canvas{}
	cv.SetSize(w, h)
	return cv
}

// SetSize resizes the canvas, clearing its contents.
func (cv *Canvas) SetSize(w, h int) {
	cv.Width, cv.Height = w, h
	cv.Chars = make([]rune, w*h)
	cv.Attrs = make([]uint32, w*h)
	for i := range cv.Chars {
		cv.Chars[i] = ' '
	}
}

// Clear fills the canvas with spaces carrying the current attribute.
func (cv *Canvas) Clear() {
	for i := range cv.Chars {
		cv.Chars[i] = ' '
		cv.Attrs[i] = cv.curattr
	}
}

// SetColorANSI sets the current foreground and background color indices.
func (cv *Canvas) SetColorANSI(fg, bg uint8) {
	if fg > 0x20 || bg > 0x20 {
		return
	}
	attr := (uint32(bg|0x40) << 18) | (uint32(fg|0x40) << 4)
	cv.curattr = (cv.curattr & 0x0000000f) | attr
}

// Attr returns the current attribute.
func (cv *Canvas) Attr() uint32 { return cv.curattr }

// SetAttr sets the current attribute.
func (cv *Canvas) SetAttr(a uint32) { cv.curattr = a }

// PutChar writes a glyph with the current attribute at the given cell.
func (cv *Canvas) PutChar(x, y int, ch rune) {
	if x < 0 || y < 0 || x >= cv.Width || y >= cv.Height {
		return
	}
	cv.Chars[x+y*cv.Width] = ch
	cv.Attrs[x+y*cv.Width] = cv.curattr
}

// nearestANSI maps a 14-bit ARGB value onto the closest ANSI color index.
func nearestANSI(argb14 uint16) uint8 {
	if argb14 < (0x10 | 0x40) {
		return uint8(argb14 ^ 0x40)
	}
	if argb14 == (Default|0x40) || argb14 == (Transparent|0x40) {
		return uint8(argb14 ^ 0x40)
	}
	if argb14 < 0x0fff { // too transparent
		return Transparent
	}

	best := uint8(Default)
	dist := 0x3fff
	for i := 0; i < 16; i++ {
		d := 0
		a := int((ansitab14[i] >> 7) & 0xf)
		b := int((argb14 >> 7) & 0xf)
		d += (a - b) * (a - b)

		a = int((ansitab14[i] >> 3) & 0xf)
		b = int((argb14 >> 3) & 0xf)
		d += (a - b) * (a - b)

		a = int((ansitab14[i] << 1) & 0xf)
		b = int((argb14 << 1) & 0xf)
		d += (a - b) * (a - b)

		if d < dist {
			dist = d
			best = uint8(i)
		}
	}
	return best
}

// AttrToANSIFg returns the ANSI foreground index of an attribute.
func AttrToANSIFg(attr uint32) uint8 { return nearestANSI(uint16((attr >> 4) & 0x3fff)) }

// AttrToANSIBg returns the ANSI background index of an attribute.
func AttrToANSIBg(attr uint32) uint8 { return nearestANSI(uint16(attr >> 18)) }

// AttrToRGB12Fg returns the 12-bit RGB foreground color of an attribute.
func AttrToRGB12Fg(attr uint32) uint16 {
	fg := uint16((attr >> 4) & 0x3fff)
	if fg < (0x10 | 0x40) {
		return ansitab16[fg^0x40] & 0x0fff
	}
	if fg == (Default | 0x40) {
		return ansitab16[LightGray] & 0x0fff
	}
	if fg == (Transparent | 0x40) {
		return ansitab16[LightGray] & 0x0fff
	}
	return (fg << 1) & 0x0fff
}

// AttrToRGB12Bg returns the 12-bit RGB background color of an attribute.
func AttrToRGB12Bg(attr uint32) uint16 {
	bg := uint16(attr >> 18)
	if bg < (0x10 | 0x40) {
		return ansitab16[bg^0x40] & 0x0fff
	}
	if bg == (Default | 0x40) {
		return ansitab16[Black] & 0x0fff
	}
	if bg == (Transparent | 0x40) {
		return ansitab16[Black] & 0x0fff
	}
	return (bg << 1) & 0x0fff
}
