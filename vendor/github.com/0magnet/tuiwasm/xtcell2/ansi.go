// Package xtcell lets a tcell program draw into an xterm-go terminal.
//
// # Why an adapter is needed at all
//
// tcell has two screen implementations and the build tags pick between them.
// tscreen.go — the terminfo one that speaks ANSI to a tty — is built with
// `!(js && wasm)`, so it does not exist in a browser. What does exist is
// wscreen.go, which paints by calling functions it expects to find on the
// JavaScript global object:
//
//	Go -> JS   drawCell(x, y, s, fg, bg, attrs, us, uc)
//	           clearScreen(fg, bg)   show()   showCursor(x, y)
//	           setCursorStyle(class, color)   resize(w, h)   beep()   setTitle(t)
//
//	JS -> Go   onKeyEvent(key, shift, alt, ctrl, meta)
//	           onMouseClick   onMouseMove   onFocus   onPaste
//
// tcell ships a tcell.js that implements the first list against a DOM grid of
// its own. This package implements the same list against an xterm-go terminal
// instead, which is what makes a tcell program run inside websh or desk rather
// than in a page of its own.
//
// The translation is to ANSI: xterm-go is a terminal emulator, so the natural
// way to drive it is the escape sequences a real program would write. That
// keeps this package small — the emulator already knows what to do with them.
//
// Input goes the other way and cannot use xterm-go's own channel. Its OnData
// hands over the bytes a terminal would send to a pty ("\x1b[A" for up-arrow),
// while tcell's onKeyEvent wants a DOM KeyboardEvent.key name ("ArrowUp") and
// four modifier booleans. Decoding one into the other loses information, so
// input is instead taken from a keydown listener, the same source tcell.js
// uses. See keys_js.go.
package xtcell2

import (
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Defaults for a cell whose color is ColorDefault. tcell's web screen
// substitutes these before it calls drawCell, and clearScreen is left to do
// it for itself, so the same two values are used in both places.
const (
	defaultFg = 0xe5e5e5
	defaultBg = 0x000000
)

// cup moves the cursor. tcell counts from zero and ANSI from one.
func cup(x, y int) string {
	return "\x1b[" + strconv.Itoa(y+1) + ";" + strconv.Itoa(x+1) + "H"
}

// sgr renders one cell's colors and attributes.
//
// Colors are emitted as truecolor rather than as palette indices: tcell has
// already resolved them to RGB by this point, so there is nothing to gain by
// mapping them back onto 256 colors and something to lose if the mapping is
// approximate.
func sgr(fg, bg int, attrs tcell.AttrMask, us tcell.UnderlineStyle) string {
	// Leading 0 resets, so each cell is written from a known state. Cells are
	// addressed individually anyway, so there is no run to preserve.
	p := make([]string, 0, 10)
	p = append(p, "0")

	if attrs&tcell.AttrBold != 0 {
		p = append(p, "1")
	}
	if attrs&tcell.AttrDim != 0 {
		p = append(p, "2")
	}
	if attrs&tcell.AttrItalic != 0 {
		p = append(p, "3")
	}
	if attrs&tcell.AttrBlink != 0 {
		p = append(p, "5")
	}
	if attrs&tcell.AttrReverse != 0 {
		p = append(p, "7")
	}
	if attrs&tcell.AttrStrikeThrough != 0 {
		p = append(p, "9")
	}
	// Styled underlines are the 4:n sub-parameter form. Only plain 4 is sent,
	// because a sub-parameter that the emulator does not understand can take
	// the rest of the sequence with it, and a solid underline where a curly
	// one was asked for is the better failure.
	if us != tcell.UnderlineStyleNone || attrs&tcell.AttrUnderline != 0 {
		p = append(p, "4")
	}

	p = append(p, rgb(38, fg), rgb(48, bg))
	return "\x1b[" + strings.Join(p, ";") + "m"
}

// rgb renders one truecolor parameter. base is 38 for foreground, 48 for
// background.
func rgb(base, c int) string {
	if c < 0 {
		if base == 38 {
			c = defaultFg
		} else {
			c = defaultBg
		}
	}
	return strconv.Itoa(base) + ";2;" +
		strconv.Itoa((c>>16)&0xff) + ";" +
		strconv.Itoa((c>>8)&0xff) + ";" +
		strconv.Itoa(c&0xff)
}

// drawCellSeq is the whole of one drawCell call.
//
// An empty string means the cell holds nothing; it still has to be painted, or
// what was there before stays on screen.
func drawCellSeq(x, y int, s string, fg, bg int, attrs tcell.AttrMask, us tcell.UnderlineStyle) string {
	if s == "" {
		s = " "
	}
	return cup(x, y) + sgr(fg, bg, attrs, us) + s
}

// clearScreenSeq paints the whole screen in one color and homes the cursor.
func clearScreenSeq(fg, bg int) string {
	return sgr(fg, bg, tcell.AttrNone, tcell.UnderlineStyleNone) + "\x1b[2J\x1b[H"
}

// showCursorSeq moves the cursor, or hides it: tcell asks for a hidden cursor
// by passing -1, -1.
func showCursorSeq(x, y int) string {
	if x < 0 || y < 0 {
		return "\x1b[?25l"
	}
	return "\x1b[?25h" + cup(x, y)
}

// cursorStyles maps the class names tcell's web screen uses onto DECSCUSR
// parameters. tcell names them for CSS classes because its own renderer is a
// DOM grid; a terminal wants the numbers.
var cursorStyles = map[string]int{
	"cursor-blinking-block":     1,
	"cursor-steady-block":       2,
	"cursor-blinking-underline": 3,
	"cursor-steady-underline":   4,
	"cursor-blinking-bar":       5,
	"cursor-steady-bar":         6,
}

// setCursorStyleSeq renders DECSCUSR. An unknown class falls back to the
// terminal default rather than guessing.
func setCursorStyleSeq(class string) string {
	n, ok := cursorStyles[class]
	if !ok {
		n = 0
	}
	return "\x1b[" + strconv.Itoa(n) + " q"
}

// setTitleSeq is OSC 2, terminated with BEL for the widest acceptance.
func setTitleSeq(title string) string {
	return "\x1b]2;" + title + "\x07"
}

// Everything below writes into a caller-owned buffer instead of returning a
// string, because this is the per-cell path.
//
// A full redraw of a large window is upwards of ten thousand cells, thirty
// times a second. Building the sequences as strings meant a slice, several
// strconv results and a join for every one of them — on the order of a
// hundred thousand allocations a frame. The standard Go collector absorbs
// that; TinyGo's conservative one on wasm does not, and the heap ran away to
// 3.3GB against 29MB for the same source built with Go.
//
// The string forms are kept: they are what the tests read, and they are
// clearer to check a sequence against than an append chain.

// appendCUP writes the cursor move. ANSI counts from one, tcell from zero.
func appendCUP(b []byte, x, y int) []byte {
	b = append(b, 0x1b, '[')
	b = strconv.AppendInt(b, int64(y+1), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(x+1), 10)
	return append(b, 'H')
}

// appendSGR writes one cell's colors and attributes.
func appendSGR(b []byte, fg, bg int, attrs tcell.AttrMask, us tcell.UnderlineStyle) []byte {
	b = append(b, 0x1b, '[', '0')

	for _, a := range [...]struct {
		mask tcell.AttrMask
		code byte
	}{
		{tcell.AttrBold, '1'},
		{tcell.AttrDim, '2'},
		{tcell.AttrItalic, '3'},
		{tcell.AttrBlink, '5'},
		{tcell.AttrReverse, '7'},
		{tcell.AttrStrikeThrough, '9'},
	} {
		if attrs&a.mask != 0 {
			b = append(b, ';', a.code)
		}
	}
	if us != tcell.UnderlineStyleNone || attrs&tcell.AttrUnderline != 0 {
		b = append(b, ';', '4')
	}

	b = appendRGB(b, 38, fg)
	b = appendRGB(b, 48, bg)
	return append(b, 'm')
}

// appendRGB writes one truecolor parameter. base is 38 for foreground, 48 for
// background.
func appendRGB(b []byte, base, c int) []byte {
	if c < 0 {
		if base == 38 {
			c = defaultFg
		} else {
			c = defaultBg
		}
	}
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(base), 10)
	b = append(b, ';', '2', ';')
	b = strconv.AppendInt(b, int64((c>>16)&0xff), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64((c>>8)&0xff), 10)
	b = append(b, ';')
	return strconv.AppendInt(b, int64(c&0xff), 10)
}
