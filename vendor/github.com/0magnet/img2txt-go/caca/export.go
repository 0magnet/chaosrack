package caca

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Version is the libcaca version reported in exported documents.
const Version = "0.99.beta20"

// ExportList returns the supported export formats and their descriptions, in
// the order libcaca reports them.
func ExportList() [][2]string {
	return [][2]string{
		{"caca", "native libcaca format"},
		{"ansi", "ANSI"},
		{"utf8", "UTF-8 with ANSI escape codes"},
		{"utf8cr", "UTF-8 with ANSI escape codes and MS-DOS \\r"},
		{"html", "HTML"},
		{"html3", "backwards-compatible HTML"},
		{"bbfr", "BBCode (French)"},
		{"irc", "IRC with mIRC colours"},
		{"ps", "PostScript document"},
		{"svg", "SVG vector image"},
		{"tga", "TGA image"},
		{"troff", "troff source"},
	}
}

// Export renders the canvas in the named format.
func (cv *Canvas) Export(format string) ([]byte, bool) {
	switch strings.ToLower(format) {
	case "caca":
		return cv.exportCaca(), true
	case "ansi":
		return cv.exportANSI(), true
	case "utf8":
		return cv.exportUTF8(false), true
	case "utf8cr":
		return cv.exportUTF8(true), true
	case "html":
		return cv.exportHTML(), true
	case "html3":
		return cv.exportHTML3(), true
	case "bbfr":
		return cv.exportBBFr(), true
	case "irc":
		return cv.exportIRC(), true
	case "ps":
		return cv.exportPS(), true
	case "svg":
		return cv.exportSVG(), true
	case "tga":
		return cv.exportTGA()
	case "troff":
		return cv.exportTroff(), true
	}
	return nil, false
}

// ansiPalette reorders libcaca color indices into ANSI order.
var ansiPalette = [16]uint8{
	0, 4, 2, 6, 1, 5, 3, 7,
	8, 12, 10, 14, 9, 13, 11, 15,
}

// attrToRGB24Fg returns the 24-bit foreground color of an attribute.
func attrToRGB24Fg(attr uint32) uint32 { return rgb12to24(AttrToRGB12Fg(attr)) }

// attrToRGB24Bg returns the 24-bit background color of an attribute.
func attrToRGB24Bg(attr uint32) uint32 { return rgb12to24(AttrToRGB12Bg(attr)) }

// rgb12to24 expands a 12-bit RGB value to 24 bits by duplicating each nibble.
func rgb12to24(c uint16) uint32 {
	return (uint32(c&0xf00) << 12) | (uint32(c&0xf00) << 8) |
		(uint32(c&0x0f0) << 8) | (uint32(c&0x0f0) << 4) |
		(uint32(c&0x00f) << 4) | uint32(c&0x00f)
}

// attrToARGB64 splits an attribute into background and foreground ARGB
// nibbles, matching caca_attr_to_argb64.
//
// This is not AttrToRGB12Bg/Fg with an alpha nibble bolted on: the two
// functions disagree about CACA_TRANSPARENT, which becomes opaque black there
// and transparent white here, and the alpha nibble comes from the palette entry
// rather than being assumed opaque.
func attrToARGB64(attr uint32) [8]uint8 {
	fg16 := uint16((attr >> 4) & 0x3fff)
	bg16 := uint16(attr >> 18)

	switch {
	case bg16 < (0x10 | 0x40):
		bg16 = ansitab16[bg16^0x40]
	case bg16 == (Default | 0x40):
		bg16 = ansitab16[Black]
	case bg16 == (Transparent | 0x40):
		bg16 = 0x0fff
	default:
		bg16 = ((bg16 << 2) & 0xf000) | ((bg16 << 1) & 0x0fff)
	}

	switch {
	case fg16 < (0x10 | 0x40):
		fg16 = ansitab16[fg16^0x40]
	case fg16 == (Default | 0x40):
		fg16 = ansitab16[LightGray]
	case fg16 == (Transparent | 0x40):
		fg16 = 0x0fff
	default:
		fg16 = ((fg16 << 2) & 0xf000) | ((fg16 << 1) & 0x0fff)
	}

	var argb [8]uint8
	argb[0] = uint8(bg16 >> 12)
	argb[1] = uint8((bg16 >> 8) & 0xf)
	argb[2] = uint8((bg16 >> 4) & 0xf)
	argb[3] = uint8(bg16 & 0xf)
	argb[4] = uint8(fg16 >> 12)
	argb[5] = uint8((fg16 >> 8) & 0xf)
	argb[6] = uint8((fg16 >> 4) & 0xf)
	argb[7] = uint8(fg16 & 0xf)

	return argb
}

// exportCaca writes the native libcaca canvas format.
func (cv *Canvas) exportCaca() []byte {
	var b []byte
	b = append(b, "\xCA\xCA"+"CV"...)

	u32 := func(v uint32) {
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], v)
		b = append(b, tmp[:]...)
	}
	u16 := func(v uint16) {
		var tmp [2]byte
		binary.BigEndian.PutUint16(tmp[:], v)
		b = append(b, tmp[:]...)
	}

	framecount := uint32(1)
	u32(16 + 32*framecount)
	u32(uint32(cv.Width) * uint32(cv.Height) * 8 * framecount)
	u16(0x0001)
	u32(framecount)
	u16(0x0000)

	// frame_info
	u32(uint32(cv.Width))
	u32(uint32(cv.Height))
	u32(0)
	u32(cv.curattr)
	u32(0) // x
	u32(0) // y
	u32(0) // handlex
	u32(0) // handley

	for n := 0; n < cv.Width*cv.Height; n++ {
		u32(uint32(cv.Chars[n]))
		u32(cv.Attrs[n])
	}
	return b
}

// exportANSI writes CP437 text with ANSI color codes.
func (cv *Canvas) exportANSI() []byte {
	var b []byte
	prevfg, prevbg := -1, -1

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		for x := 0; x < cv.Width; x++ {
			ansifg := AttrToANSIFg(lineattr[x])
			ansibg := AttrToANSIBg(lineattr[x])
			fg := int(LightGray)
			if ansifg < 0x10 {
				fg = int(ansiPalette[ansifg])
			}
			bg := int(Black)
			if ansibg < 0x10 {
				bg = int(ansiPalette[ansibg])
			}
			ch := linechar[x]
			if ch == MagicFullwidth {
				ch = '?'
			}

			if fg != prevfg || bg != prevbg {
				b = append(b, "\033[0;"...)
				switch {
				case fg < 8 && bg < 8:
					b = append(b, fmt.Sprintf("3%d;4%dm", fg, bg)...)
				case fg < 8:
					b = append(b, fmt.Sprintf("5;3%d;4%dm", fg, bg-8)...)
				case bg < 8:
					b = append(b, fmt.Sprintf("1;3%d;4%dm", fg-8, bg)...)
				default:
					b = append(b, fmt.Sprintf("5;1;3%d;4%dm", fg-8, bg-8)...)
				}
			}

			b = append(b, UTF32ToCP437(ch))
			prevfg, prevbg = fg, bg
		}

		if cv.Width == 80 {
			b = append(b, "\033[s\n\033[u"...)
		} else {
			b = append(b, "\033[0m\r\n"...)
			prevfg, prevbg = -1, -1
		}
	}
	return b
}

// exportUTF8 writes UTF-8 text with ANSI color codes.
func (cv *Canvas) exportUTF8(cr bool) []byte {
	var b []byte

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		prevfg, prevbg := 0x10, 0x10

		for x := 0; x < cv.Width; x++ {
			attr := lineattr[x]
			ch := linechar[x]
			if ch == MagicFullwidth {
				continue
			}

			ansifg := AttrToANSIFg(attr)
			ansibg := AttrToANSIBg(attr)
			fg := 0x10
			if ansifg < 0x10 {
				fg = int(ansiPalette[ansifg])
			}
			bg := 0x10
			if ansibg < 0x10 {
				bg = int(ansiPalette[ansibg])
			}

			if fg != prevfg || bg != prevbg {
				b = append(b, "\033[0"...)
				if fg < 8 {
					b = append(b, fmt.Sprintf(";3%d", fg)...)
				} else if fg < 16 {
					b = append(b, fmt.Sprintf(";1;3%d;9%d", fg-8, fg-8)...)
				}
				if bg < 8 {
					b = append(b, fmt.Sprintf(";4%d", bg)...)
				} else if bg < 16 {
					b = append(b, fmt.Sprintf(";5;4%d;10%d", bg-8, bg-8)...)
				}
				b = append(b, 'm')
			}

			b = UTF32ToUTF8(b, ch)
			prevfg, prevbg = fg, bg
		}

		if prevfg != 0x10 || prevbg != 0x10 {
			b = append(b, "\033[0m"...)
		}
		if cr {
			b = append(b, "\r\n"...)
		} else {
			b = append(b, '\n')
		}
	}
	return b
}

// htmlChar appends one glyph in HTML-escaped form.
func htmlChar(b []byte, ch rune) []byte {
	switch {
	case ch == MagicFullwidth:
		return b
	case uch(ch) <= 0x00000020 || (uch(ch) >= 0x0000007f && uch(ch) <= 0x000000a0):
		// Control characters and space become U+00A0 NO-BREAK SPACE.
		return append(b, "&#160;"...)
	case ch == '&':
		return append(b, "&amp;"...)
	case ch == '<':
		return append(b, "&lt;"...)
	case ch == '>':
		return append(b, "&gt;"...)
	case ch == '"':
		return append(b, "&quot;"...)
	case ch == '\'':
		return append(b, "&#39;"...)
	case uch(ch) < 0x00000080:
		return append(b, byte(ch))
	case uch(ch) <= 0x0010fffd && (uch(ch)&0x0000fffe) != 0x0000fffe &&
		(uch(ch) < 0x0000d800 || uch(ch) > 0x0000dfff):
		return append(b, fmt.Sprintf("&#%d;", ch)...)
	default:
		return append(b, fmt.Sprintf("&#%d;", 0x0000fffd)...)
	}
}

// exportHTML writes an XHTML document.
func (cv *Canvas) exportHTML() []byte {
	var b []byte

	b = append(b, "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Strict//EN\"\n"...)
	b = append(b, "   \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd\">\n"...)
	b = append(b, "<html xmlns=\"http://www.w3.org/1999/xhtml\" lang=\"en\" xml:lang=\"en\">"...)
	b = append(b, "<head>\n"...)
	b = append(b, fmt.Sprintf("<title>Generated by libcaca %s</title>\n", Version)...)
	b = append(b, "</head><body>\n"...)
	b = append(b, fmt.Sprintf("<div style=\"%s\">\n",
		"font-family: monospace, fixed; font-weight: bold;")...)

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		for x := 0; x < cv.Width; {
			b = append(b, "<span style=\""...)
			if AttrToANSIFg(lineattr[x]) != Default {
				b = append(b, fmt.Sprintf(";color:#%.03x", AttrToRGB12Fg(lineattr[x]))...)
			}
			if AttrToANSIBg(lineattr[x]) < 0x10 {
				b = append(b, fmt.Sprintf(";background-color:#%.03x", AttrToRGB12Bg(lineattr[x]))...)
			}
			if lineattr[x]&StyleBold != 0 {
				b = append(b, ";font-weight:bold"...)
			}
			if lineattr[x]&StyleItalics != 0 {
				b = append(b, ";font-style:italic"...)
			}
			if lineattr[x]&StyleUnderline != 0 {
				b = append(b, ";text-decoration:underline"...)
			}
			if lineattr[x]&StyleBlink != 0 {
				b = append(b, ";text-decoration:blink"...)
			}
			b = append(b, "\">"...)

			l := 0
			for ; x+l < cv.Width && lineattr[x+l] == lineattr[x]; l++ {
				b = htmlChar(b, linechar[x+l])
			}
			b = append(b, "</span>"...)
			x += l
		}
		b = append(b, "<br />\n"...)
	}

	b = append(b, "</div></body></html>\n"...)
	return b
}

// exportHTML3 writes a table-based HTML document for old browsers.
func (cv *Canvas) exportHTML3() []byte {
	hasMultiCellRow := false
	bitmapLen := (cv.Width + 7) / 8
	cellBoundary := make([]byte, bitmapLen)

	isBoundary := func(x int) bool { return cellBoundary[x/8]&(1<<(x%8)) != 0 }

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]
		for x := 1; x < cv.Width; x++ {
			if isBoundary(x) {
				continue
			}
			if (linechar[x-1] == MagicFullwidth && !IsFullwidth(linechar[x])) ||
				AttrToANSIBg(lineattr[x-1]) != AttrToANSIBg(lineattr[x]) ||
				(AttrToANSIBg(lineattr[x]) < 0x10 &&
					attrToRGB24Bg(lineattr[x-1]) != attrToRGB24Bg(lineattr[x])) {
				hasMultiCellRow = true
				cellBoundary[x/8] |= 1 << (x % 8)
			}
		}
	}

	var b []byte
	b = append(b, "<table border=\"0\" cellpadding=\"0\" cellspacing=\"0\" summary=\"[libcaca canvas export]\">\n"...)

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		b = append(b, "<tr>"...)

		for x := 0; x < cv.Width; {
			// Factor adjacent cells that share attributes into one colspan.
			l := 1
			for x+l < cv.Width &&
				((y != 0 && uch(linechar[x+l]) > 0x00000020 &&
					(uch(linechar[x+l]) < 0x0000007f || uch(linechar[x+l]) > 0x000000a0)) ||
					!isBoundary(x+l) ||
					linechar[x+l] == MagicFullwidth ||
					cv.Height == 1) &&
				(linechar[x+l-1] != MagicFullwidth || IsFullwidth(linechar[x+l])) &&
				AttrToANSIBg(lineattr[x+l]) == AttrToANSIBg(lineattr[x]) &&
				(AttrToANSIBg(lineattr[x]) >= 0x10 ||
					attrToRGB24Bg(lineattr[x+l]) == attrToRGB24Bg(lineattr[x])) {
				l++
			}

			nonblank := false
			for i := 0; i < l; i++ {
				c := linechar[x+i]
				if !(uch(c) <= 0x00000020 || (uch(c) >= 0x0000007f && uch(c) <= 0x000000a0)) {
					nonblank = true
				}
			}

			b = append(b, "<td"...)
			if AttrToANSIBg(lineattr[x]) < 0x10 {
				b = append(b, fmt.Sprintf(" bgcolor=\"#%.06x\"", attrToRGB24Bg(lineattr[x]))...)
			}
			if hasMultiCellRow && l > 1 {
				colspan := l
				for i := 0; i < l; i++ {
					if i != 0 && !isBoundary(x+i) {
						colspan--
					}
				}
				if colspan > 1 {
					b = append(b, fmt.Sprintf(" colspan=\"%d\"", colspan)...)
				}
			}
			b = append(b, '>')
			b = append(b, "<tt>"...)

			needfont := false
			for i := 0; i < l; i++ {
				if nonblank && (i == 0 || lineattr[x+i] != lineattr[x+i-1]) {
					needfont = AttrToANSIFg(lineattr[x+i]) != Default
					if needfont {
						b = append(b, fmt.Sprintf("<font color=\"#%.06x\">",
							attrToRGB24Fg(lineattr[x+i]))...)
					}
					if lineattr[x+i]&StyleBold != 0 {
						b = append(b, "<b>"...)
					}
					if lineattr[x+i]&StyleItalics != 0 {
						b = append(b, "<i>"...)
					}
					if lineattr[x+i]&StyleUnderline != 0 {
						b = append(b, "<u>"...)
					}
					if lineattr[x+i]&StyleBlink != 0 {
						b = append(b, "<blink>"...)
					}
				}

				b = htmlChar(b, linechar[x+i])

				if nonblank && (i+1 == l || lineattr[x+i+1] != lineattr[x+i]) {
					if lineattr[x+i]&StyleBlink != 0 {
						b = append(b, "</blink>"...)
					}
					if lineattr[x+i]&StyleUnderline != 0 {
						b = append(b, "</u>"...)
					}
					if lineattr[x+i]&StyleItalics != 0 {
						b = append(b, "</i>"...)
					}
					if lineattr[x+i]&StyleBold != 0 {
						b = append(b, "</b>"...)
					}
					if needfont {
						b = append(b, "</font>"...)
					}
				}
			}

			b = append(b, "</tt>"...)
			b = append(b, "</td>"...)
			x += l
		}
		b = append(b, "</tr>\n"...)
	}

	b = append(b, "</table>\n"...)
	return b
}

// exportBBFr writes French BBCode.
func (cv *Canvas) exportBBFr() []byte {
	var b []byte
	b = append(b, "[font=Courier New]"...)

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		for x := 0; x < cv.Width; {
			l := 1
			if linechar[x] == ' ' {
				for x+l < cv.Width && lineattr[x+l] == lineattr[x] && linechar[x] == ' ' {
					l++
				}
			} else {
				for x+l < cv.Width && lineattr[x+l] == lineattr[x] && linechar[x] != ' ' {
					l++
				}
			}

			needback := AttrToANSIBg(lineattr[x]) < 0x10
			needfront := AttrToANSIFg(lineattr[x]) < 0x10

			if needback {
				b = append(b, fmt.Sprintf("[f=#%.06x]", attrToRGB24Bg(lineattr[x]))...)
			}
			if linechar[x] == ' ' {
				b = append(b, fmt.Sprintf("[c=#%.06x]", attrToRGB24Bg(lineattr[x]))...)
			} else if needfront {
				b = append(b, fmt.Sprintf("[c=#%.06x]", attrToRGB24Fg(lineattr[x]))...)
			}

			if lineattr[x]&StyleBold != 0 {
				b = append(b, "[g]"...)
			}
			if lineattr[x]&StyleItalics != 0 {
				b = append(b, "[i]"...)
			}
			if lineattr[x]&StyleUnderline != 0 {
				b = append(b, "[s]"...)
			}

			for i := 0; i < l; i++ {
				switch {
				case linechar[x+i] == MagicFullwidth:
				case linechar[x+i] == ' ':
					b = append(b, '_')
				default:
					b = UTF32ToUTF8(b, linechar[x+i])
				}
			}

			if lineattr[x]&StyleUnderline != 0 {
				b = append(b, "[/s]"...)
			}
			if lineattr[x]&StyleItalics != 0 {
				b = append(b, "[/i]"...)
			}
			if lineattr[x]&StyleBold != 0 {
				b = append(b, "[/g]"...)
			}
			if linechar[x] == ' ' || needfront {
				b = append(b, "[/c]"...)
			}
			if needback {
				b = append(b, "[/f]"...)
			}
			x += l
		}
		b = append(b, '\n')
	}

	b = append(b, "[/font]\n"...)
	return b
}

// ircPalette maps libcaca color indices to mIRC color numbers.
var ircPalette = [16]uint8{
	1, 2, 3, 10, 5, 6, 7, 15, // dark
	14, 12, 9, 11, 4, 13, 8, 0, // light
}

// exportIRC writes text with mIRC color codes.
func (cv *Canvas) exportIRC() []byte {
	var b []byte

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		prevfg, prevbg := 0x10, 0x10

		for x := 0; x < cv.Width; x++ {
			attr := lineattr[x]
			ch := linechar[x]
			if ch == MagicFullwidth {
				continue
			}

			ansifg := AttrToANSIFg(attr)
			ansibg := AttrToANSIBg(attr)
			fg := 0x10
			if ansifg < 0x10 {
				fg = int(ircPalette[ansifg])
			}
			bg := 0x10
			if ansibg < 0x10 {
				bg = int(ircPalette[ansibg])
			}

			if bg != prevbg || fg != prevfg {
				needEscape := false
				if bg == 0x10 {
					if fg == 0x10 {
						b = append(b, 0x0f)
					} else {
						if prevbg == 0x10 {
							b = append(b, fmt.Sprintf("\x03%d", fg)...)
						} else {
							b = append(b, fmt.Sprintf("\x0f\x03%d", fg)...)
						}
						if ch == ',' {
							needEscape = true
						}
					}
				} else {
					if fg == 0x10 {
						b = append(b, fmt.Sprintf("\x0f\x03,%d", bg)...)
					} else {
						b = append(b, fmt.Sprintf("\x03%d,%d", fg, bg)...)
					}
				}
				if uch(ch) >= 0x30 && uch(ch) <= 0x39 {
					needEscape = true
				}
				if needEscape {
					b = append(b, 0x02, 0x02)
				}
			}

			b = UTF32ToUTF8(b, ch)
			prevfg, prevbg = fg, bg
		}

		if cv.Width == 0 {
			b = append(b, ' ')
		}
		b = append(b, '\r', '\n')
	}
	return b
}

// psHeader is the PostScript prologue.
const psHeader = "%!\n" +
	"%% libcaca PDF export\n" +
	"%%LanguageLevel: 2\n" +
	"%%Pages: 1\n" +
	"%%DocumentData: Clean7Bit\n" +
	"/csquare {\n" +
	"  newpath\n" +
	"  0 0 moveto\n" +
	"  0 1 rlineto\n" +
	"  1 0 rlineto\n" +
	"  0 -1 rlineto\n" +
	"  closepath\n" +
	"  setrgbcolor\n" +
	"  fill\n" +
	"} def\n" +
	"/S {\n" +
	"  Show\n" +
	"} bind def\n" +
	"/Courier-Bold findfont\n" +
	"8 scalefont\n" +
	"setfont\n" +
	"gsave\n" +
	"6 10 scale\n"

// exportPS writes a PostScript document.
func (cv *Canvas) exportPS() []byte {
	var b []byte
	b = append(b, psHeader...)
	b = append(b, fmt.Sprintf("0 %d translate\n", cv.Height)...)

	// Background squares, drawn bottom-up.
	for y := cv.Height - 1; y >= 0; y-- {
		lineattr := cv.Attrs[y*cv.Width:]
		for x := 0; x < cv.Width; x++ {
			argb := attrToARGB64(lineattr[x])
			b = append(b, fmt.Sprintf("1 0 translate\n %f %f %f csquare\n",
				float32(argb[1])*(1.0/0xf),
				float32(argb[2])*(1.0/0xf),
				float32(argb[3])*(1.0/0xf))...)
		}
		b = append(b, fmt.Sprintf("-%d 1 translate\n", cv.Width)...)
	}

	b = append(b, "grestore\n"...)
	b = append(b, fmt.Sprintf("0 %d translate\n", cv.Height*10)...)

	for y := cv.Height - 1; y >= 0; y-- {
		row := cv.Height - y - 1
		lineattr := cv.Attrs[row*cv.Width:]
		linechar := cv.Chars[row*cv.Width:]

		for x := 0; x < cv.Width; x++ {
			ch := linechar[x]
			argb := attrToARGB64(lineattr[x])

			b = append(b, "newpath\n"...)
			b = append(b, fmt.Sprintf("%d %d moveto\n", (x+1)*6, y*10+2)...)
			b = append(b, fmt.Sprintf("%f %f %f setrgbcolor\n",
				float32(argb[5])*(1.0/0xf),
				float32(argb[6])*(1.0/0xf),
				float32(argb[7])*(1.0/0xf))...)

			switch {
			case uch(ch) < 0x00000020 || uch(ch) >= 0x00000080:
				b = append(b, "(?) show\n"...)
			default:
				switch byte(ch & 0x7f) {
				case '\\', '(', ')':
					b = append(b, fmt.Sprintf("(\\%c) show\n", byte(ch))...)
				default:
					b = append(b, fmt.Sprintf("(%c) show\n", byte(ch))...)
				}
			}
		}
	}

	b = append(b, "showpage\n"...)
	return b
}

// exportSVG writes an SVG vector image.
func (cv *Canvas) exportSVG() []byte {
	var b []byte
	b = append(b, fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
		"<svg width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\""+
		" xmlns=\"http://www.w3.org/2000/svg\""+
		" xmlns:xlink=\"http://www.w3.org/1999/xlink\""+
		" xml:space=\"preserve\" version=\"1.1\"  baseProfile=\"full\">\n",
		cv.Width*6, cv.Height*10, cv.Width*6, cv.Height*10)...)

	b = append(b, " <g id=\"mainlayer\" font-size=\"10\""+
		" style=\"font-family: monospace\">\n"...)

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		for x := 0; x < cv.Width; x++ {
			b = append(b, fmt.Sprintf("<rect style=\"fill:#%.03x\" x=\"%d\" y=\"%d\""+
				" width=\"6\" height=\"10\"/>\n",
				AttrToRGB12Bg(lineattr[x]), x*6, y*10)...)
		}
	}

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]
		for x := 0; x < cv.Width; x++ {
			ch := linechar[x]
			if ch == ' ' || ch == MagicFullwidth {
				continue
			}

			bold, italic := "", ""
			if lineattr[x]&StyleBold != 0 {
				bold = " font-weight=\"bold\""
			}
			if lineattr[x]&StyleItalics != 0 {
				italic = " font-style=\"italic\""
			}
			b = append(b, fmt.Sprintf("<text style=\"fill:#%.03x\"%s%s x=\"%d\" y=\"%d\">",
				AttrToRGB12Fg(lineattr[x]), bold, italic, x*6, y*10+8)...)

			switch {
			case uch(ch) < 0x00000020:
				b = append(b, '?')
			case uch(ch) > 0x0000007f:
				b = UTF32ToUTF8(b, ch)
			default:
				switch ch {
				case '>':
					b = append(b, "&gt;"...)
				case '<':
					b = append(b, "&lt;"...)
				case '&':
					b = append(b, "&amp;"...)
				default:
					b = append(b, byte(ch))
				}
			}
			b = append(b, "</text>\n"...)
		}
	}

	b = append(b, " </g>\n"...)
	b = append(b, "</svg>\n"...)
	return b
}

// ansi2troff maps color indices to troff color names.
var ansi2troff = [16]string{
	"black", "blue", "green", "cyan",
	"red", "magenta", "yellow", "white",
	"black", "blue", "green", "cyan",
	"red", "magenta", "yellow", "white",
}

// exportTroff writes troff source.
func (cv *Canvas) exportTroff() []byte {
	var b []byte
	b = append(b, ".nf\n"...)

	prevfg, prevbg := uint8(0), uint8(0)
	started := false

	for y := 0; y < cv.Height; y++ {
		lineattr := cv.Attrs[y*cv.Width:]
		linechar := cv.Chars[y*cv.Width:]

		for x := 0; x < cv.Width; x++ {
			fg := AttrToANSIFg(lineattr[x])
			bg := AttrToANSIBg(lineattr[x])
			ch := linechar[x]

			if fg != prevfg || !started {
				b = append(b, fmt.Sprintf("\\m[%s]", ansi2troff[fg&0x0f])...)
			}
			if bg != prevbg || !started {
				b = append(b, fmt.Sprintf("\\M[%s]", ansi2troff[bg&0x0f])...)
			}
			if lineattr[x]&StyleBold != 0 {
				b = append(b, "\\fB"...)
			}
			if lineattr[x]&StyleItalics != 0 {
				b = append(b, "\\fI"...)
			}

			switch {
			case ch == '\\':
				b = append(b, "\\\\"...)
			case ch == ' ':
				// Unbreakable space at line ends, else spaces get dropped.
				if x == 0 || x == cv.Width-1 {
					b = append(b, 0xc2, 0xa0)
				} else {
					b = UTF32ToUTF8(b, ch)
				}
			default:
				b = UTF32ToUTF8(b, ch)
			}

			if lineattr[x]&(StyleBold|StyleItalics) != 0 {
				b = append(b, "\\fR"...)
			}

			prevfg, prevbg = fg, bg
			started = true
		}
		b = append(b, '\n')
	}
	return b
}
