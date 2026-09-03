//go:build js && wasm

package xwrite

import (
	xterm "github.com/0magnet/xterm-go"
)

// Writer writes to an xterm-go terminal.
type Writer struct{ term *xterm.Terminal }

// New returns a Writer for term.
func New(term *xterm.Terminal) *Writer { return &Writer{term: term} }

// Write sends p to the terminal, fixing bare newlines on the way.
//
// It never fails: the terminal is a buffer in the same address space, so
// there is no short write and nothing to report.
func (w *Writer) Write(p []byte) (int, error) {
	w.term.Write(crlf(p))
	return len(p), nil
}

// Size is the terminal's current size in cells, which the layout libraries
// want so they can fit their output to it.
func (w *Writer) Size() (cols, rows int) {
	return w.term.Core.Cols(), w.term.Core.Rows()
}
