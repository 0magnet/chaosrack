package rackhost

import (
	"image/color"
	"sync"
	"unicode/utf8"

	tcell2 "github.com/gdamore/tcell/v2"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// A terminal model on a terminal: the shell or the demo drawn as the
// characters it is, the terminal sized to the scene (racktui's TextScener),
// where a picture of it would be too coarse to read.

// The bounds a terminal is sized within, whatever the scene.
const (
	textMinCols, textMinRows = 20, 5
	textMaxCols, textMaxRows = 400, 200
)

// TextScene is the terminal model on show as cells, its terminal made w by
// h. ok is false for any other model, or with the rack off.
func (r *Rack) TextScene(w, h int) (cells []racktui.TextCell, cols, rows int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := r.model()
	if r.value("power-sw") == "0" || !isTerminalModel(key) {
		return nil, 0, 0, false
	}
	w = min(max(w, textMinCols), textMaxCols)
	h = min(max(h, textMinRows), textMaxRows)
	switch key {
	case "desk":
		if r.deskStyle() != "flat" {
			return nil, 0, 0, false // a desktop in space is a picture
		}
		r.desk.sync()
		return r.desk.text(w, h), w, h, true
	case "hostterm", "terminal":
		sh := r.shellOf(key)
		if sh.term == nil {
			sh.start()
		}
		sh.resize(w, h)
		cells, cols, rows = sh.term.textCells()
		return cells, cols, rows, true
	default:
		a := r.animRunning()
		switch {
		case a.sim != nil:
			a.sim.SetSize(w, h)
			cells, cols, rows = simCells(a.sim)
		case a.term != nil:
			a.term.resize(w, h)
			cells, cols, rows = a.term.textCells()
		}
		return cells, cols, rows, true
	}
}

// resize makes the shell's terminal cols by rows, and its pty, which tells
// the shell.
func (sh *shellView) resize(cols, rows int) {
	sh.term.resize(cols, rows)
	if sh.pty != nil && sh.running() {
		// A pty gone with its shell has no size to set.
		if err := setPtySize(sh.pty, cols, rows); err != nil {
			return
		}
	}
}

// simCells is a tcell v2 simulated screen as cells.
func simCells(s tcell2.SimulationScreen) (cells []racktui.TextCell, cols, rows int) {
	sc, w, h := s.GetContents()
	// The cells handed back are the screen's own, which its Show goes on
	// writing: they are read under the screen's lock, which tcell v2 does not
	// put in its interface but its simulated screen has.
	if l, ok := s.(sync.Locker); ok {
		l.Lock()
		defer l.Unlock()
	}
	cells = make([]racktui.TextCell, w*h)
	for i, c := range sc {
		ch, _ := utf8.DecodeRune(c.Bytes)
		fg, bg := simColors(c.Style)
		cells[i] = racktui.TextCell{Ch: ch, Fg: fg, Bg: bg}
	}
	return cells, w, h
}

// simColors is a v2 style's colors, the terminal's defaults where it has
// none.
func simColors(st tcell2.Style) (fg, bg color.RGBA) {
	px := func(c tcell2.Color, def color.RGBA) color.RGBA {
		if c == tcell2.ColorDefault || !c.Valid() {
			return def
		}
		r, g, b := c.RGB()
		return color.RGBA{uint8(r), uint8(g), uint8(b), 255} //nolint:gosec // RGB is 0..255 each
	}
	fgc, bgc, _ := st.Decompose()
	return px(fgc, color.RGBA{217, 217, 217, 255}), px(bgc, color.RGBA{0, 0, 0, 255})
}

// deskStyle is the desk's style selector's, flat when there is none. Called
// with r.mu held.
func (r *Rack) deskStyle() string {
	if s := r.value("desk-style"); s != "" {
		return s
	}
	return "flat"
}
