// Package figlet reads FIGlet and TOIlet fonts and renders text with them.
//
// It is a Go port of the FIGfont half of libcaca 0.99.beta20 (caca/figfont.c),
// which is itself where TOIlet's original figlet.c ended up. The parser handles
// both the `.flf` fonts figlet ships and the `.tlf` fonts toilet adds, in plain,
// gzipped or zipped form, and the renderer implements the four layout modes and
// the six horizontal smushing rules of the FIGfont specification.
//
// Original C implementation copyright © 2002—2018 Sam Hocevar <sam@hocevar.net>,
// released under the WTFPL.
package figlet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0magnet/toilet-go/canvas"
)

// stdGlyphs is the number of printable ASCII glyphs every FIGfont must define,
// U+0020 to U+007E.
const stdGlyphs = 127 - 32

// extGlyphs adds the seven German glyphs a FIGfont must define after those.
const extGlyphs = stdGlyphs + 7

// deutsch lists the code points of those seven mandatory extra glyphs:
// Ä Ö Ü ä ö ü ß.
var deutsch = [7]rune{196, 214, 220, 228, 246, 252, 223}

// ErrBadFont reports a file that is not a usable FIGfont.
var ErrBadFont = errors.New("figlet: invalid font")

// glyph is one entry of the font's lookup table: the code point it renders and
// the number of columns it occupies.
type glyph struct {
	code  rune
	width int
}

// Font is a parsed FIGfont. Glyphs are held as rows of a single tall canvas,
// glyph n occupying rows n*Height to n*Height+Height-1, which is how libcaca
// stores them and what lets a glyph be blitted out with a canvas handle.
type Font struct {
	// Hardblank is the character the font uses to hold a column open. It is
	// rewritten to U+00A0 on load and back to a space on output.
	Hardblank rune

	// Height is the number of rows in every glyph, Baseline the row the
	// characters sit on, and MaxLength the longest line the file may contain.
	Height, Baseline, MaxLength int

	// OldLayout is the signed layout number of the original FIGfont format and
	// FullLayout the bitfield that superseded it. PrintDirection and
	// CodetagCount are parsed and carried but not used.
	OldLayout                    int
	FullLayout                   int
	PrintDirection, CodetagCount int

	glyphs []glyph
	cv     *canvas.Canvas
}

// Glyphs returns the number of glyphs the font defines.
func (f *Font) Glyphs() int { return len(f.glyphs) }

// GlyphWidth returns the column count of the glyph for ch, and whether the font
// defines it at all.
func (f *Font) GlyphWidth(ch rune) (int, bool) {
	for _, g := range f.glyphs {
		if g.code == ch {
			return g.width, true
		}
	}
	return 0, false
}

// index returns the position of ch in the lookup table, or -1. libcaca scans
// the table linearly and takes the first match, so a font that defines a code
// point twice renders the earlier one.
func (f *Font) index(ch rune) int {
	for i, g := range f.glyphs {
		if g.code == ch {
			return i
		}
	}
	return -1
}

// LoadFont reads a font from disk. As in libcaca, the path is tried as given
// first, then with a `.tlf` suffix and then with a `.flf` suffix, so callers
// pass a bare font name.
func LoadFont(path string) (*Font, error) {
	for _, p := range []string{path, path + ".tlf", path + ".flf"} {
		data, err := os.ReadFile(filepath.Clean(p))
		if err != nil {
			continue
		}
		f, err := ParseFont(data)
		if err != nil {
			continue
		}
		return f, nil
	}
	return nil, fmt.Errorf("%w: could not load font %s", ErrBadFont, path)
}

// ParseFont parses the contents of a FIGfont file, unpacking it first if it is
// a gzip or zip archive.
func ParseFont(raw []byte) (*Font, error) {
	data, err := decompress(raw)
	if err != nil {
		return nil, err
	}

	r := &lineReader{data: data}
	f := &Font{}

	line, ok := r.gets(2048)
	if !ok {
		return nil, fmt.Errorf("%w: empty file", ErrBadFont)
	}

	hardblank, commentLines, err := f.parseHeader(line)
	if err != nil {
		return nil, err
	}
	f.Hardblank, _ = canvas.DecodeUTF8(append(append([]byte{}, hardblank...), 0))

	for i := 0; i < commentLines; i++ {
		r.gets(2048)
	}

	// The mandatory glyphs come first and are implicitly numbered; anything
	// after them is introduced by a line giving its code point.
	var body []byte
	for !r.eof() {
		n := len(f.glyphs)

		switch {
		case n < stdGlyphs:
			f.glyphs = append(f.glyphs, glyph{code: rune(32 + n)})
		case n < extGlyphs:
			f.glyphs = append(f.glyphs, glyph{code: deutsch[n-stdGlyphs]})
		default:
			code, kind := r.readCodeTag()
			switch kind {
			case codeEOF:
				return f.finish(body)
			case codeBlank:
				// Blank lines are ignored, as in jacky.flf. libcaca still
				// counts the glyph, leaving an entry it never fills in; the
				// zero value here plays that role.
				f.glyphs = append(f.glyphs, glyph{})
				continue
			case codeNegative:
				// Negative code points are not supported. libcaca skips the
				// glyph's rows without storing them, which shifts every later
				// glyph up by one; that is reproduced rather than corrected.
				for j := 0; j < f.Height; j++ {
					r.gets(2048)
				}
				f.glyphs = append(f.glyphs, glyph{})
				continue
			case codeBad:
				return nil, fmt.Errorf("%w: bad glyph tag for glyph #%d",
					ErrBadFont, n)
			}
			f.glyphs = append(f.glyphs, glyph{code: code})
		}

		for j := 0; j < f.Height; j++ {
			if l, gok := r.gets(2048); gok {
				body = append(body, l...)
			}
		}
	}

	return f.finish(body)
}

// parseHeader reads the `flf2a`/`tlf2a` signature line. It follows the sscanf
// format libcaca uses, which needs six of its nine conversions to succeed and
// silently accepts anything after them.
func (f *Font) parseHeader(line []byte) (hardblank []byte, commentLines int, err error) {
	s := &scanner{b: line}

	// "%*[ft]lf2a": one or more of 'f' and 't', then the literal "lf2a".
	if !s.runOf("ft") || !s.literal("lf2a") {
		return nil, 0, fmt.Errorf("%w: bad header %q", ErrBadFont, line)
	}

	// "%6s": the hardblank, up to six bytes so a UTF-8 character fits.
	hardblank, ok := s.word(6)
	if !ok {
		return nil, 0, fmt.Errorf("%w: bad header %q", ErrBadFont, line)
	}

	fields := []*int{
		&f.Height, &f.Baseline, &f.MaxLength, &f.OldLayout, &commentLines,
		&f.PrintDirection, &f.FullLayout, &f.CodetagCount,
	}
	n := 1
	for i, p := range fields {
		// Only old_layout is scanned with "%i", which honors a 0x or 0
		// base prefix; the rest are plain decimal.
		v, vok := s.number(i == 3)
		if !vok {
			break
		}
		*p = v
		n++
	}
	if n < 6 {
		return nil, 0, fmt.Errorf("%w: bad header %q", ErrBadFont, line)
	}

	if f.OldLayout < -1 || f.OldLayout > 63 || f.FullLayout > 32767 ||
		(f.FullLayout&0x80 != 0 && f.FullLayout&0x3f == 0 && f.OldLayout != 0) {
		return nil, 0, fmt.Errorf("%w: invalid layout %d/%d",
			ErrBadFont, f.OldLayout, f.FullLayout)
	}
	if f.Height <= 0 {
		return nil, 0, fmt.Errorf("%w: invalid height %d", ErrBadFont, f.Height)
	}

	return hardblank, commentLines, nil
}

// finish imports the collected glyph rows into a canvas, replaces hardblanks
// and strips the end-of-line markers, measuring each glyph as it goes.
func (f *Font) finish(body []byte) (*Font, error) {
	if len(f.glyphs) < extGlyphs {
		return nil, fmt.Errorf("%w: only %d glyphs, expected at least %d",
			ErrBadFont, len(f.glyphs), extGlyphs)
	}

	f.cv = canvas.New(0, 0)
	f.cv.ImportUTF8(body)

	// Every glyph line ends in one or two copies of a marker character. Scan
	// each row from the right: the first non-space is the marker, repeats of it
	// are blanked too, and the first cell that differs fixes the glyph width.
	for j := 0; j < f.Height*len(f.glyphs); j++ {
		var oldch rune

		for i := f.MaxLength; i > 0; {
			i--
			ch := f.cv.GetChar(i, j)

			// Hardblanks become U+00A0, which smushing rule 6 recognizes and
			// which the flush turns back into a space.
			if ch == f.Hardblank {
				ch = 0xa0
				f.cv.PutChar(i, j, ch)
			}

			switch {
			case oldch != 0 && ch != oldch:
				if f.glyphs[j/f.Height].width == 0 {
					f.glyphs[j/f.Height].width = i + 1
				}
			case oldch != 0 && ch == oldch:
				f.cv.PutChar(i, j, ' ')
			case ch != ' ':
				oldch = ch
				f.cv.PutChar(i, j, ' ')
			}
		}
	}

	return f, nil
}

// codeKind classifies a glyph tag line.
type codeKind int

const (
	codeOK codeKind = iota
	codeEOF
	codeBlank
	codeNegative
	codeBad
)

// readCodeTag reads the line introducing an extra glyph and returns its code
// point. Hexadecimal tags are written `0x…`; everything else is decimal.
func (r *lineReader) readCodeTag() (rune, codeKind) {
	line, ok := r.gets(2048)
	if !ok {
		return 0, codeEOF
	}
	if len(line) == 0 || line[0] == '\n' || line[0] == '\r' {
		return 0, codeBlank
	}
	if line[0] == '-' {
		return 0, codeNegative
	}
	if line[0] < '0' || line[0] > '9' {
		return 0, codeBad
	}

	s := &scanner{b: line}
	if len(line) > 1 && line[1] == 'x' {
		s.i = 2
		v, vok := s.digits(16)
		if !vok {
			return 0, codeBad
		}
		return rune(v), codeOK
	}
	v, vok := s.digits(10)
	if !vok {
		return 0, codeBad
	}
	return rune(v), codeOK
}
