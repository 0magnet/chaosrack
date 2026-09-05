//go:build js && wasm

// Package play runs a demo in an xterm-go terminal.
//
// A demo is one of two shapes — it either writes styled text to an io.Writer
// or paints cells into a tcell.Screen — and getting either onto a terminal in
// a page takes more than calling it. The screen shape needs a tcell screen
// bridged to the terminal, a size that is the terminal's rather than tcell's
// hardcoded 80x24, a click handler so the window can take the keyboard, and a
// goroutine because PollEvent blocks and the page has one thread. Closing it
// again means undoing those in the right order.
//
// That was written three times inside this repository before it was written
// here. Anything embedding these demos — a desk window, a page, a texture on a
// quad in a 3D scene — needs the same thing, so it lives in a package of its
// own and takes a terminal the caller may already own.
package play

import (
	"fmt"
	"syscall/js"

	"github.com/gdamore/tcell/v3"

	tcell2 "github.com/gdamore/tcell/v2"

	xterm "github.com/0magnet/xterm-go"
	"github.com/0magnet/xterm-go/vt"

	"github.com/0magnet/tuiwasm/demos"
	"github.com/0magnet/tuiwasm/xtcell"
	"github.com/0magnet/tuiwasm/xtcell2"
	"github.com/0magnet/tuiwasm/xwrite"
)

// Session is a demo running in a terminal. Close it to stop the demo and
// release what was set up for it.
type Session struct {
	demo   demos.Demo
	term   *xterm.Terminal
	owned  bool // true when this package made the terminal and must dispose it
	bridge interface {
		Claim()
		Active() bool
	}
	screen interface{ Fini() }
	claim  js.Func
}

// NewTerminal builds a terminal in el, configured the way these demos need.
//
// vt.NewOptions rather than &vt.Options{}: the zero value has a FontSize and
// LineHeight of 0, so a cell measures 0x0 and fitting divides the window by
// that. The page does not fail, it wedges.
//
// It also moves the terminal onto the WebGL renderer, which matters more than
// it sounds. The DOM renderer rebuilds a span per cell; a demo redrawing a
// full window every frame pegs it and the page stops answering altogether.
// Anything showing an animation needs this, so a caller supplying its own
// terminal should make sure it is not on the DOM renderer.
func NewTerminal(el js.Value) *xterm.Terminal {
	t := xterm.New(vt.NewOptions())
	t.Open(el)
	t.AutoFit()
	FastRenderer(t)
	return t
}

// Mount builds a terminal in el and runs the demo in it.
func Mount(d demos.Demo, el js.Value) (*Session, error) {
	s, err := In(d, NewTerminal(el), el)
	if err != nil {
		return nil, err
	}
	s.owned = true
	return s, nil
}

// In runs the demo in a terminal the caller already owns.
//
// el is where the terminal was opened; it is needed for the mousedown handler
// that gives a screen demo the keyboard, and it is what tcell's screen is
// bridged against. The terminal is left alone by Close — a caller that made it
// is the one that should dispose it.
//
// The caller's terminal should be on the WebGL renderer. See NewTerminal.
func In(d demos.Demo, t *xterm.Terminal, el js.Value) (*Session, error) {
	s := &Session{demo: d, term: t}

	if d.Text != nil {
		// No tcell, no screen, no keyboard: it writes and stops. Any number of
		// these can run at once.
		w := xwrite.New(t)
		cols, rows := w.Size()
		if err := d.Text(w, cols, rows); err != nil {
			fmt.Fprintf(w, "\r\n%s: %v\r\n", d.Name, err) //nolint:errcheck
		}
		return s, nil
	}
	if d.Screen == nil && d.ScreenV2 == nil {
		return nil, fmt.Errorf("play: demo %q has neither a Text nor a Screen", d.Name)
	}

	// A demo whose glyphs are only correct mirrored asks for them that way. The
	// glyph cache is rasterised already, so it has to be told to redo it —
	// setting the option alone would take effect on the next resize and not
	// before, which is to say whenever the window happened to move.
	if d.Mirror != nil {
		t.Core.Options.MirrorGlyph = d.Mirror
		t.RefreshGlyphs()
	}

	// A tcell v2 demo runs against the v2 driver — the pre-migration xtcell,
	// kept as xtcell2. Everything below mirrors the v3 path one major back.
	if d.ScreenV2 != nil {
		screen := xtcell2.New(t, el)
		if err := screen.Init(); err != nil {
			return nil, err
		}
		s.bridge = screen
		s.screen = screen
		s.claim = js.FuncOf(func(js.Value, []js.Value) any {
			s.bridge.Claim()
			return nil
		})
		el.Call("addEventListener", "mousedown", s.claim, true)
		cols, rows := t.Core.Cols(), t.Core.Rows()
		view := tcell2.Screen(&gated2{Screen: screen, active: screen.Active})
		go func() {
			if err := d.ScreenV2(view, cols, rows); err != nil {
				js.Global().Get("console").Call("error", d.Name+": "+err.Error())
			}
		}()
		return s, nil
	}

	// xtcell.New rather than tcell.NewScreen: in a browser tcell's own screen
	// paints by calling a JavaScript global once per cell, which costs about
	// 79us a cell and puts a full redraw two orders of magnitude past a frame.
	// This one keeps the cells in Go and writes the frame in one go. It is an
	// ordinary tcell.Screen; the demo cannot tell.
	//
	// It takes its size from the terminal and follows it, so there is nothing
	// to bind afterwards.
	screen := xtcell.New(t, el)
	if err := screen.Init(); err != nil {
		return nil, err
	}
	s.bridge = screen
	s.screen = screen

	// Clicking anywhere takes the keyboard. Mousedown rather than a focus
	// event, because the terminal is a div and does not take focus on its own.
	s.claim = js.FuncOf(func(js.Value, []js.Value) any {
		s.bridge.Claim()
		return nil
	})
	el.Call("addEventListener", "mousedown", s.claim, true)

	cols, rows := t.Core.Cols(), t.Core.Rows()

	// Hand the demo a screen that does not paint while its window is behind
	// another. See gated: with several animations open, drawing frames nobody
	// can see is what takes the page down.
	view := tcell.Screen(&gated{Screen: screen, active: s.bridge.Active})

	// PollEvent blocks and the page has one thread to block, so the demo gets
	// its own goroutine. Blocking here would freeze the whole page, not just
	// this window.
	go func() {
		if err := d.Screen(view, cols, rows); err != nil {
			js.Global().Get("console").Call("error", d.Name+": "+err.Error())
		}
	}()
	return s, nil
}

// Close stops the demo and releases everything set up for it.
//
// Finalizing the screen is what ends the demo: canvas.Run and the other loops
// watch the event channel the screen closes, so there is no separate stop
// signal to send. Fini also gives back the keyboard and the terminal's own data
// callback, so there is nothing to unwind after it.
func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.claim.Truthy() {
		s.claim.Release()
		s.claim = js.Func{}
	}
	if s.screen != nil {
		s.screen.Fini()
		s.screen = nil
		s.bridge = nil
	}
	if s.owned && s.term != nil {
		s.term.Dispose()
		s.term = nil
	}
}

// Terminal returns the terminal the demo is running in.
func (s *Session) Terminal() *xterm.Terminal { return s.term }
