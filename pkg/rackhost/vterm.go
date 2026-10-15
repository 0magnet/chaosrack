package rackhost

import (
	"image"
	"image/color"
	"io"
	"sync"

	"github.com/gdamore/tcell/v3/tty"

	"github.com/0magnet/chaosrack/pkg/attractor"
	"github.com/0magnet/xterm-go/vt"
)

// A terminal in memory, for the models that put one on the scene: what a
// program writes is parsed by xterm-go's emulator, as a terminal on the page
// parses it, and what the emulator answers — and the keys sent in — is what
// the program reads. Its screen is read as a picture, two pixels a cell.

// vterm is a terminal in memory. It is the program's tty (tcell's Tty).
type vterm struct {
	mu         sync.Mutex
	term       *vt.Terminal
	cols, rows int
	in         chan []byte
	pending    []byte
	done       chan struct{}
	closeOnce  sync.Once
}

var _ tty.Tty = (*vterm)(nil)

// newVterm is a cols by rows terminal, taking a line feed as a new line when
// convertEOL says.
func newVterm(cols, rows int, convertEOL bool) *vterm {
	o := vt.NewOptions()
	o.Cols, o.Rows, o.Scrollback = cols, rows, 0
	o.ConvertEol = convertEOL // for a program that writes lines, not a screen
	v := &vterm{term: vt.NewTerminal(o), cols: cols, rows: rows, in: make(chan []byte, 64), done: make(chan struct{})}
	// The emulator's answers to the program's questions go back to it, as a
	// terminal's do: tcell asks what the terminal is before it draws.
	v.term.OnData = func(s string) { v.send(s) }
	return v
}

// send is input to the program: keys, or the terminal's answers.
func (v *vterm) send(s string) {
	select {
	case v.in <- []byte(s):
	case <-v.done:
	default: // a program not reading: its input is dropped, not queued forever
	}
}

func (v *vterm) Start() error             { return nil }
func (v *vterm) Stop() error              { return nil }
func (v *vterm) Drain() error             { return nil }
func (v *vterm) NotifyResize(chan<- bool) {}
func (v *vterm) WindowSize() (tty.WindowSize, error) {
	return tty.WindowSize{Width: v.cols, Height: v.rows}, nil
}

// Read is the program's input: blocks until there is some, or the terminal
// is closed.
func (v *vterm) Read(p []byte) (int, error) {
	if len(v.pending) == 0 {
		select {
		case b := <-v.in:
			v.pending = b
		case <-v.done:
			return 0, io.EOF
		}
	}
	n := copy(p, v.pending)
	v.pending = v.pending[n:]
	return n, nil
}

// Write is the program's output, into the emulator.
func (v *vterm) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.term.Write(p)
	return len(p), nil
}

// Close ends the terminal, for tcell: see end.
func (v *vterm) Close() error {
	v.end()
	return nil
}

// end ends the terminal: a program reading it reads the end.
func (v *vterm) end() { v.closeOnce.Do(func() { close(v.done) }) }

// picture is the screen as it stands (cellpaint.go).
func (v *vterm) picture() *image.RGBA {
	v.mu.Lock()
	defer v.mu.Unlock()
	m := cellPicture(v.cols, v.rows)
	buf := v.term.Buffer()
	px := func(c [3]float32) color.RGBA {
		return color.RGBA{uint8(c[0] * 255), uint8(c[1] * 255), uint8(c[2] * 255), 255}
	}
	for r := range v.rows {
		if buf.YDisp+r >= buf.Lines.Length() {
			break
		}
		line := buf.Lines.Get(buf.YDisp + r)
		if line == nil {
			continue
		}
		for c := range v.cols {
			fg := px(attractor.VTColor(line.GetFg(c), [3]float32{0.85, 0.85, 0.85}))
			bg := px(attractor.VTColor(line.GetBg(c), [3]float32{0, 0, 0}))
			paintCell(m, c, r, rune(line.GetCodePoint(c)), fg, bg) //nolint:gosec // a code point fits a rune
		}
	}
	return m
}
