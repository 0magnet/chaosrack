package lattice

import (
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// screen writes frames to a terminal, sending only the cells that changed
// since the last one: a sheet is mostly empty and mostly still, and a stack
// of them is many terminals' worth of output.
type screen struct {
	out   io.Writer
	buf   []byte // a frame's escapes, kept from frame to frame
	prev  Frame
	color bool
}

func newScreen(w io.Writer, color bool) *screen {
	return &screen{out: w, color: color}
}

// enter takes the alternate screen and hides the cursor; leave gives them
// back.
func (s *screen) enter() error {
	_, err := io.WriteString(s.out, "\x1b[?1049h\x1b[?25l\x1b[2J")
	return err
}

func (s *screen) leave() error {
	_, err := io.WriteString(s.out, "\x1b[0m\x1b[?25h\x1b[?1049l")
	return err
}

// draw writes f where it differs from the last frame.
func (s *screen) draw(f Frame) error {
	full := s.prev.Cols != f.Cols || s.prev.Rows != f.Rows
	cr, cc := -1, -1 // where the cursor is, if known
	var last Cell
	haveColor := false
	for r := range f.Rows {
		for c := range f.Cols {
			cell := f.At(c, r)
			if !full && s.prev.At(c, r) == cell {
				continue
			}
			if r != cr || c != cc {
				s.buf = append(strconv.AppendInt(append(strconv.AppendInt(append(s.buf, "\x1b["...), int64(r+1), 10), ';'), int64(c+1), 10), 'H')
			}
			if cell.Ch == 0 {
				s.buf = append(s.buf, ' ')
			} else {
				if s.color && (!haveColor || cell.R != last.R || cell.G != last.G || cell.B != last.B) {
					s.buf = appendSGR(s.buf, cell)
					last, haveColor = cell, true
				}
				s.buf = utf8.AppendRune(s.buf, cell.Ch)
			}
			cr, cc = r, c+1
		}
	}
	s.prev = f
	_, err := s.out.Write(s.buf)
	s.buf = s.buf[:0]
	return err
}

// sgr is the escape that sets a cell's color.
func sgr(c Cell) string { return string(appendSGR(nil, c)) }

// appendSGR is sgr, appended to b.
func appendSGR(b []byte, c Cell) []byte {
	b = strconv.AppendInt(append(b, "\x1b[38;2;"...), int64(c.R), 10)
	b = strconv.AppendInt(append(b, ';'), int64(c.G), 10)
	b = strconv.AppendInt(append(b, ';'), int64(c.B), 10)
	return append(b, 'm')
}

// plain writes f as lines of text, colored or not, for -once.
func plain(w io.Writer, f Frame, color bool) error {
	var bw strings.Builder
	for r := range f.Rows {
		end := f.Cols
		for end > 0 && f.At(end-1, r).Ch == 0 {
			end--
		}
		for c := range end {
			cell := f.At(c, r)
			switch {
			case cell.Ch == 0:
				bw.WriteByte(' ')
			case color:
				bw.WriteString(sgr(cell))
				bw.WriteRune(cell.Ch)
			default:
				bw.WriteRune(cell.Ch)
			}
		}
		if color {
			bw.WriteString("\x1b[0m")
		}
		bw.WriteByte('\n')
	}
	_, err := io.WriteString(w, bw.String())
	return err
}
