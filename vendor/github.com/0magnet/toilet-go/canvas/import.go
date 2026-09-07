package canvas

import "github.com/0magnet/img2txt-go/caca"

// This is libcaca's codec/text.c importer, in the "utf8" mode toilet uses. It
// is what turns a line of input — or a block of FIGfont glyph data — into a
// canvas, and it is where escape sequences in the input become cell
// attributes.

// utf8Trailing gives the number of continuation bytes implied by a lead byte.
var utf8Trailing = [256]uint8{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5,
}

// utf8Offsets removes the lead and continuation bit patterns accumulated by the
// decoder below.
var utf8Offsets = [6]uint32{
	0x00000000, 0x00003080, 0x000E2080,
	0x03C82080, 0xFA082080, 0x82082080,
}

// DecodeUTF8 decodes one code point and returns it with the number of bytes it
// occupied. It is caca_utf8_to_utf32(): the encoding is not validated beyond
// the length implied by the lead byte, and a zero byte anywhere in the sequence
// terminates it with a length of zero.
func DecodeUTF8(s []byte) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}

	todo := int(utf8Trailing[s[0]])
	var ret uint32

	for i := 0; ; i++ {
		if i >= len(s) || s[i] == 0 {
			return 0, 0
		}
		ret += uint32(s[i]) << (6 * (todo - i))
		if todo == i {
			return rune(ret - utf8Offsets[todo]), i + 1
		}
	}
}

// ansi2caca maps the eight ECMA-48 colors onto libcaca's palette order.
var ansi2caca = [8]uint8{
	caca.Black, caca.Red, caca.Green, caca.Brown,
	caca.Blue, caca.Magenta, caca.Cyan, caca.LightGray,
}

// importState is the ANSI graphic rendition state the importer carries between
// escape sequences.
type importState struct {
	clearattr uint32

	fg, bg   uint8 // current colors
	dfg, dbg uint8 // colors a reset returns to

	bold, blink, italics, negative, concealed, underline bool
	faint, strike, proportional                          bool // parsed, unused
}

// parseGRCM applies an SGR sequence and recomputes the canvas color.
func (cv *Canvas) parseGRCM(im *importState, argv []uint32) {
	for _, a := range argv {
		switch {
		case a >= 30 && a <= 37:
			im.fg = ansi2caca[a-30]
		case a >= 40 && a <= 47:
			im.bg = ansi2caca[a-40]
		case a >= 90 && a <= 97:
			im.fg = ansi2caca[a-90] + 8
		case a >= 100 && a <= 107:
			im.bg = ansi2caca[a-100] + 8
		default:
			switch a {
			case 0: // default rendition
				im.fg, im.bg = im.dfg, im.dbg
				im.bold, im.blink, im.italics = false, false, false
				im.negative, im.concealed, im.underline = false, false, false
				im.faint, im.strike, im.proportional = false, false, false
			case 1:
				im.bold = true
			case 2:
				im.faint = true
			case 3:
				im.italics = true
			case 4, 21: // singly and doubly underlined
				im.underline = true
			case 5, 6: // slow and rapid blink
				im.blink = true
			case 7:
				im.negative = true
			case 8:
				im.concealed = true
			case 9:
				im.strike = true
			case 22:
				im.bold, im.faint = false, false
			case 23:
				im.italics = false
			case 24:
				im.underline = false
			case 25:
				im.blink = false
			case 26:
				im.proportional = true
			case 27:
				im.negative = false
			case 28:
				im.concealed = false
			case 29:
				im.strike = false
			case 39:
				im.fg = im.dfg
			case 49:
				im.bg = im.dbg
			case 50:
				im.proportional = false
			}
		}
	}

	var efg, ebg uint8
	if im.concealed {
		efg, ebg = caca.Transparent, caca.Transparent
	} else {
		efg, ebg = im.fg, im.bg
		if im.negative {
			efg, ebg = im.bg, im.fg
		}
		if im.bold {
			if efg < 8 {
				efg += 8
			} else if efg == caca.Default {
				efg = caca.White
			}
		}
	}

	cv.SetColorANSI(efg, ebg)
}

// ImportUTF8 parses UTF-8 text, with ECMA-48 escape sequences, into the canvas.
// It is caca_import_canvas_from_memory(cv, ..., "utf8"): a zero-sized canvas
// grows to fit the text rather than wrapping it, and the frame cursor is left
// where the text ended.
func (cv *Canvas) ImportUTF8(data []byte) {
	var im importState

	width, height := cv.Width, cv.Height
	growx, growy := width == 0, height == 0
	x, y := cv.X, cv.Y
	saveX, saveY := 0, 0

	im.dfg, im.dbg = caca.Default, caca.Transparent
	cv.SetColorANSI(im.dfg, im.dbg)
	im.clearattr = cv.Attr()

	cv.parseGRCM(&im, []uint32{0})

	size := len(data)
	for i := 0; i < size; {
		var ch rune
		wch := 0
		skip := 1

		switch {
		case data[i] == '\r':
			x = 0

		case data[i] == '\n':
			x, y = 0, y+1

		case data[i] == '\t':
			x = (x + 8) &^ 7

		case data[i] == '\x08':
			if x > 0 {
				x--
			}

		// An escape sequence needs at least three bytes; wait for more.
		case data[i] == '\033' && i+2 >= size:
			i = size
			continue

		// Safe: the case above already sent every i with i+2 >= size to the end
		// of the input, so i+1 and i+2 are in range by the time this is reached.
		// gosec cannot see an invariant established by an earlier case.
		case data[i] == '\033' && data[i+1] == '(' && data[i+2] == 'B': //nolint:gosec
			skip += 2

		case data[i] == '\033' && data[i+1] == '[': //nolint:gosec
			n, stop := cv.parseCSI(data[i:], &im, &x, &y, &saveX, &saveY,
				width, height)
			if stop {
				i = size
				continue
			}
			skip += n

		case data[i] == '\033' && data[i+1] == ']': //nolint:gosec
			n, stop := parseOSC(data[i:])
			if stop {
				i = size
				continue
			}
			skip += n

		// A form feed starts a new frame. caca_create_frame() copies the
		// current one, so with a single frame kept here the effect is just to
		// send the cursor home; the cells carry over either way.
		case i+1 < size && data[i] == '\f' && data[i+1] == '\n': //nolint:gosec
			x, y = 0, 0
			skip++

		default:
			var bytes int
			ch, bytes = DecodeUTF8(data[i:])
			if bytes == 0 {
				// Not valid UTF-8, so assume the byte was latin1.
				ch, bytes = rune(data[i]), 1
			}
			wch = 1
			if caca.IsFullwidth(ch) {
				wch = 2
			}
			skip += bytes - 1
		}

		// Wrap long lines, or grow the canvas sideways.
		for x+wch > width {
			if growx {
				saved := cv.Attr()
				cv.SetAttr(im.clearattr)
				width = x + wch
				cv.SetSize(width, height)
				cv.SetAttr(saved)
			} else {
				x -= width
				y++
			}
		}

		// Grow downwards, or scroll.
		if y >= height {
			saved := cv.Attr()
			cv.SetAttr(im.clearattr)
			if growy {
				height = y + 1
				cv.SetSize(width, height)
			} else {
				lines := (y - height) + 1
				for j := 0; j+lines < height; j++ {
					copy(cv.Attrs[j*cv.Width:(j+1)*cv.Width],
						cv.Attrs[(j+lines)*cv.Width:(j+lines+1)*cv.Width])
					copy(cv.Chars[j*cv.Width:(j+1)*cv.Width],
						cv.Chars[(j+lines)*cv.Width:(j+lines+1)*cv.Width])
				}
				cv.FillBox(0, height-lines, cv.Width-1, height-1, ' ')
				y -= lines
			}
			cv.SetAttr(saved)
		}

		if wch != 0 {
			cv.PutChar(x, y, ch)
			x += wch
		}

		i += skip
	}

	if growy && y > height {
		saved := cv.Attr()
		cv.SetAttr(im.clearattr)
		height = y
		cv.SetSize(width, height)
		cv.SetAttr(saved)
	}

	cv.X, cv.Y = x, y
}

// parseCSI interprets one control sequence starting at data[0] == ESC. It
// returns the offset of the final byte and whether the import should stop
// because the sequence is truncated.
func (cv *Canvas) parseCSI(data []byte, im *importState, x, y, saveX, saveY *int,
	width, height int) (int, bool) {
	size := len(data)

	// Offsets to the parameter bytes (0x30-0x3f), the intermediate bytes
	// (0x20-0x2f) and the mandatory final byte (0x40-0x7e).
	param := 2
	inter := param
	for inter < size && data[inter] >= 0x30 && data[inter] <= 0x3f {
		inter++
	}
	final := inter
	for final < size && data[final] >= 0x20 && data[final] <= 0x2f { //nolint:gosec
		final++
	}

	if final >= size || data[final] < 0x40 || data[final] > 0x7e { //nolint:gosec
		return 0, true // invalid final byte
	}

	if param < inter && data[param] >= 0x3c {
		return final, false // private sequence, skipped whole
	}
	if final-param > 100 {
		return final, false // suspiciously long, skipped whole
	}

	var argv []uint32
	if param < inter {
		argv = append(argv, 0)
		for j := param; j < inter; j++ {
			switch {
			case data[j] == ';':
				argv = append(argv, 0)
			case data[j] >= '0' && data[j] <= '9':
				argv[len(argv)-1] = 10*argv[len(argv)-1] + uint32(data[j]-'0')
			}
		}
	}
	argc := len(argv)

	switch data[final] { //nolint:gosec
	case 'H', 'f': // CUP, HVP - cursor position
		*x, *y = 0, 0
		if argc > 1 && argv[1] > 0 {
			*x = int(argv[1]) - 1
		}
		if argc > 0 && argv[0] > 0 {
			*y = int(argv[0]) - 1
		}
	case 'A': // CUU - cursor up
		*y -= delta(argv)
		if *y < 0 {
			*y = 0
		}
	case 'B': // CUD - cursor down
		*y += delta(argv)
	case 'C': // CUF - cursor right
		*x += delta(argv)
	case 'D': // CUB - cursor left
		*x -= delta(argv)
		if *x < 0 {
			*x = 0
		}
	case 'G': // CHA - cursor character absolute
		*x = 0
		if argc != 0 && argv[0] > 0 {
			*x = int(argv[0]) - 1
		}
	case 'J': // ED - erase in page
		saved := cv.Attr()
		cv.SetAttr(im.clearattr)
		// The width and height arguments below are libcaca's, passed
		// unchanged: it hands caca_fill_box() the far corner where the
		// function expects a size, so the cleared area is one cell short.
		switch {
		case argc == 0 || argv[0] == 0:
			cv.DrawLine(*x, *y, width, *y, ' ')
			cv.FillBox(0, *y+1, width-1, height-1, ' ')
		case argv[0] == 1:
			cv.FillBox(0, 0, width-1, *y-1, ' ')
			cv.DrawLine(0, *y, *x, *y, ' ')
		case argv[0] == 2:
			cv.FillBox(0, 0, width-1, height-1, ' ')
		}
		cv.SetAttr(saved)
	case 'K': // EL - erase in line
		switch {
		case argc == 0 || argv[0] == 0:
			cv.DrawLine(*x, *y, width, *y, ' ')
		case argv[0] == 1:
			cv.DrawLine(0, *y, *x, *y, ' ')
		case argv[0] == 2:
			if *x < width {
				cv.DrawLine(*x, *y, width-1, *y, ' ')
			}
		}
	case 'P': // DCH - delete character
		n := 1
		if argc != 0 && argv[0] != 0 {
			n = int(argv[0])
		}
		for j := 0; j+n < width; j++ {
			cv.PutChar(j, *y, cv.GetChar(j+n, *y))
			cv.PutAttr(j, *y, cv.GetAttr(j+n, *y))
		}
	case 'X': // ECH - erase character
		if argc != 0 && argv[0] != 0 {
			saved := cv.Attr()
			cv.SetAttr(im.clearattr)
			cv.DrawLine(*x, *y, *x+int(argv[0])-1, *y, ' ')
			cv.SetAttr(saved)
		}
	case 'd': // VPA - line position absolute
		*y = 0
		if argc != 0 && argv[0] > 0 {
			*y = int(argv[0]) - 1
		}
	case 'm': // SGR - select graphic rendition
		if argc != 0 {
			cv.parseGRCM(im, argv)
		} else {
			cv.parseGRCM(im, []uint32{0})
		}
	case 's': // private: save cursor position
		*saveX, *saveY = *x, *y
	case 'u': // private: restore cursor position
		*x, *y = *saveX, *saveY
	}

	return final, false
}

// delta returns an ECMA-48 movement count, which defaults to one when the
// sequence carried no parameter.
func delta(argv []uint32) int {
	if len(argv) != 0 {
		return int(argv[0])
	}
	return 1
}

// parseOSC skips an operating system command. libcaca parses the command number
// and string only to discard them, so this does the same.
func parseOSC(data []byte) (int, bool) {
	size := len(data)

	semicolon := 2
	for semicolon < size && data[semicolon] >= '0' && data[semicolon] <= '9' {
		semicolon++
	}
	if semicolon >= size || data[semicolon] != ';' {
		return 0, true // invalid mode
	}

	final := semicolon + 1
	for final < size && data[final] >= 0x20 {
		final++
	}
	if final >= size || data[final] != '\a' {
		return 0, true // no terminating bell
	}

	return final, false
}
