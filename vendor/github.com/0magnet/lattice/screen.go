package lattice

import (
	"io"
	"strconv"
	"strings"
)

// screen writes frames to a terminal, sending only the cells that changed
// since the last one: a sheet is mostly empty and mostly still, and a stack
// of them is many terminals' worth of output.
type screen struct {
	out   io.Writer
	w     strings.Builder
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
				s.w.WriteString("\x1b[" + strconv.Itoa(r+1) + ";" + strconv.Itoa(c+1) + "H")
			}
			if cell.Ch == 0 {
				s.w.WriteByte(' ')
			} else {
				if s.color && (!haveColor || cell.R != last.R || cell.G != last.G || cell.B != last.B) {
					s.w.WriteString(sgr(cell))
					last, haveColor = cell, true
				}
				s.w.WriteRune(cell.Ch)
			}
			cr, cc = r, c+1
		}
	}
	s.prev = f
	_, err := io.WriteString(s.out, s.w.String())
	s.w.Reset()
	return err
}

// sgr is the escape that sets a cell's color.
func sgr(c Cell) string {
	return "\x1b[38;2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B)) + "m"
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
