//go:build js && wasm

package xtcell

import (
	"sync"
	"syscall/js"
	"unicode/utf8"

	"github.com/gdamore/tcell/v3"

	xterm "github.com/0magnet/xterm-go"
	"github.com/0magnet/xterm-go/vt"
)

// term is everything a Screen needs of a terminal: somewhere to put a frame, a
// size to tell tcell about, and the two callbacks it takes over while it runs.
//
// It is an interface rather than *xterm.Terminal because most of what is worth
// testing here — the diff, the sequences it produces, the event plumbing —
// involves no terminal at all, and building a real one needs a DOM that
// `go test` does not have.
type term interface {
	Write(data []byte)
	Cols() int
	Rows() int
	OnResize() func(cols, rows int)
	SetOnResize(func(cols, rows int))
	OnData() func(string)
	SetOnData(func(string))
}

// xtermTerm adapts the real terminal, whose size and callbacks live on Core.
//
// A pointer, and carrying an AttributeData, because it is also the cellSink:
// the direct path reuses that one struct for every cell rather than allocating
// ten thousand of them a frame.
type xtermTerm struct {
	t    *xterm.Terminal
	attr *vt.AttributeData
}

func (x *xtermTerm) Write(data []byte)            { x.t.Write(data) }
func (x *xtermTerm) Cols() int                    { return x.t.Core.Cols() }
func (x *xtermTerm) Rows() int                    { return x.t.Core.Rows() }
func (x *xtermTerm) OnResize() func(int, int)     { return x.t.Core.OnResize }
func (x *xtermTerm) SetOnResize(f func(int, int)) { x.t.Core.OnResize = f }
func (x *xtermTerm) OnData() func(string)         { return x.t.Core.OnData }
func (x *xtermTerm) SetOnData(f func(string))     { x.t.Core.OnData = f }

// current is the screen holding the keyboard focus.
//
// It no longer decides who may draw. That used to be the whole of this
// package's difficulty: tcell's own wasm screen paints by calling functions it
// looks up on the JavaScript global object, so a page had exactly one of them
// and a second window opening left the first painting into the wrong terminal.
// This screen writes to its terminal directly and shares nothing, so any number
// of them can run side by side.
//
// What is still exclusive is the keyboard, because a keystroke has one
// destination. Active reports it, and a host that runs animations can use it to
// skip frames nobody is looking at.
var current *Screen

// Screen is a tcell.Screen that draws into an xterm-go terminal.
//
// # Why this exists rather than tcell's own
//
// tcell has two screen implementations and the build tags choose between them.
// tscreen.go — the terminfo one, which speaks ANSI to a tty — is built with
// `!(js && wasm)`, so in a browser it does not exist. What is left is
// wscreen.go, which paints by calling a JavaScript global once per cell:
//
//	drawCell(x, y, str, fg, bg, attrs, us, uc)
//
// That is the problem. Backing that global with a Go callback, which is the
// only way to get the cells into an emulator written in Go, makes every cell a
// Go -> JS -> Go round trip carrying eight arguments. Measured in this browser
// it costs 79us per cell: a full redraw of a 200x50 window is 794ms against a
// 16.7ms frame budget, and the marshalling of the arguments is fifty times the
// cost of the call itself. One animation was enough to saturate a core and stop
// the page answering; the same frame handed over as a single string costs
// 0.40ms.
//
// So the cells never leave Go. tcell's CellBuffer — which is exported, and is
// the whole of the storage and dirty-tracking — holds the screen, this file
// turns the dirty ones into escape sequences, and the frame goes to the
// terminal in one Write. tcell is otherwise untouched: this is an ordinary
// tcell.Screen and every program that draws through the interface is unchanged.
type Screen struct {
	term term
	el   js.Value

	mu    sync.Mutex
	cells tcell.CellBuffer
	cols  int
	rows  int

	// style is the default set by SetStyle, used by Clear.
	style tcell.Style

	// Cursor position, or -1,-1 for hidden. cursorSet says whether anything
	// has been asked for yet, so a screen that never mentions the cursor does
	// not keep re-hiding it.
	curX, curY int
	curStyle   string
	curDirty   bool

	// fallback substitutions registered through RegisterRuneFallback.
	fallback map[rune]string

	// One frame's worth of escape sequences, written in a single call.
	// Truncated rather than released between frames: the capacity is the point.
	buf []byte

	// The last styling written, so a run of cells that share it says it once.
	// It is most of the bytes — a cell is about ten of position and thirty-odd
	// of colour — and neighbouring cells nearly always share a style.
	//
	// The inputs are compared rather than the rendered sequence, so an
	// unchanged style costs nothing at all.
	haveStyle bool
	lastFg    int
	lastBg    int
	lastAttrs tcell.AttrMask
	lastUS    tcell.UnderlineStyle

	// Where the terminal's cursor is left standing, so a cell that follows the
	// previous one does not have to say where it is. Invalidated by anything
	// that moves the cursor for its own reasons.
	penX, penY int
	penValid   bool

	evch  chan tcell.Event
	stopq chan struct{}
	once  sync.Once

	keydown   js.Func
	input     js.Value
	savedData func(string)

	// Mouse reporting; see mouse_js.go.
	mouseOn            bool
	mdown, mup, mwheel js.Func

	// sink is set when the terminal will take cells directly. See direct_js.go.
	sink cellSink

	started bool
}

// New returns a screen that draws into t. Call Init before using it.
//
// el is the element t was opened into: the key handler is scoped to the
// terminal's own input rather than to the document, so one window does not eat
// another's keystrokes.
func New(t *xterm.Terminal, el js.Value) *Screen {
	return newScreen(&xtermTerm{t: t, attr: vt.NewAttributeData()}, el)
}

func newScreen(t term, el js.Value) *Screen {
	return &Screen{
		term:  t,
		el:    el,
		style: tcell.StyleDefault,
		// Hidden, which is where tcell starts, and stated rather than assumed.
		// The terminal starts with its cursor showing, so a screen that merely
		// believes the cursor is hidden and never says so leaves a block
		// blinking in the corner of every animation — which is what it did.
		curX:     -1,
		curY:     -1,
		curDirty: true,
		fallback: map[rune]string{},
		evch:     make(chan tcell.Event, 64),
		stopq:    make(chan struct{}),
	}
}

// Init sizes the screen from the terminal, takes the keyboard and starts
// delivering events.
func (s *Screen) Init() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	// Take the short way to the cells when the terminal offers one. A terminal
	// that does not — the fake one in the tests — still gets escape sequences,
	// so both paths stay exercised and neither is load-bearing on its own.
	s.sink, _ = s.term.(cellSink)
	s.cols, s.rows = s.term.Cols(), s.term.Rows()
	if s.cols <= 0 {
		s.cols = 80
	}
	if s.rows <= 0 {
		s.rows = 24
	}
	s.cells.Resize(s.cols, s.rows)
	// Start from a terminal that is actually blank. A fresh CellBuffer reports
	// nothing dirty — every cell's last and current contents are both the empty
	// string — so without this the first frame would draw over whatever the
	// terminal happened to be showing.
	s.buf = append(s.buf, clearScreenSeq(-1, -1)...)
	s.haveStyle, s.penValid = false, false
	s.flushLocked()
	s.mu.Unlock()

	// Follow the terminal rather than telling it. Sizing is the emulator's
	// business — it is the thing that knows the font metrics and the element —
	// and tcell's web screen offered no way to hear about it, which is why the
	// old adapter had to push a size in from outside.
	prev := s.term.OnResize()
	s.term.SetOnResize(func(cols, rows int) {
		if prev != nil {
			prev(cols, rows)
		}
		s.resized(cols, rows)
	})

	s.bindInput(s.el)
	s.bindMouse(s.el)
	current = s
	return nil
}

func (s *Screen) resized(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s.mu.Lock()
	if cols == s.cols && rows == s.rows {
		s.mu.Unlock()
		return
	}
	s.cols, s.rows = cols, rows
	s.cells.Resize(cols, rows)
	s.cells.Invalidate()
	s.haveStyle = false
	s.mu.Unlock()
	s.post(tcell.NewEventResize(cols, rows))
}

// Fini stops the screen and gives the terminal back.
func (s *Screen) Fini() {
	s.once.Do(func() {
		// stopq first, so post stops offering events before the queue closes
		// under it. Then the key handler goes, so nothing can produce one at
		// all, and only then is the queue closed.
		close(s.stopq)
		s.detachKeys()
		s.detachMouse()

		s.mu.Lock()
		// Leave the terminal in a state someone else can use: cursor back,
		// styling reset.
		s.buf = append(s.buf, "\x1b[0m\x1b[?25h"...)
		s.haveStyle, s.penValid = false, false
		s.flushLocked()
		s.mu.Unlock()

		// v3 says the event queue stays open until Fini, and callers read it
		// closing as the screen going away — an animation loop ends on it.
		close(s.evch)

		if current == s {
			current = nil
		}
	})
}

// Claim gives this screen the keyboard. Call it when its window comes to the
// front.
func (s *Screen) Claim() {
	if current == s {
		return
	}
	current = s
	s.focusInput()
	s.Sync()
}

// Active reports whether this screen holds the keyboard — that is, whether it
// is the window in front.
//
// It is no longer required for correctness; screens no longer share anything.
// It survives because drawing a frame nobody is looking at is still wasted
// work, and in a page every window computes its frames on the one thread.
func (s *Screen) Active() bool { return current == nil || current == s }

// Terminal size. This screen's size is always the terminal's.
func (s *Screen) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

// SetSize resizes the buffer. The terminal is the authority on size, so this
// exists for the interface and for callers that resize before the emulator has
// measured itself.
func (s *Screen) SetSize(w, h int) { s.resized(w, h) }

// ---------------------------------------------------------------- cell access

// SetContent sets a cell from a rune and its combining marks.
//
// v3's CellBuffer has no SetContent of its own — it keeps only Put, which takes
// the string — so the runes are joined here. That allocates, which is why the
// animations in termanim call Put with a constant string instead; this is the
// path for callers written against the older shape.
func (s *Screen) SetContent(x, y int, primary rune, combining []rune, style tcell.Style) {
	s.mu.Lock()
	s.cells.Put(x, y, string(append([]rune{primary}, combining...)), style)
	s.mu.Unlock()
}

func (s *Screen) Get(x, y int) (string, tcell.Style, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cells.Get(x, y)
}

func (s *Screen) Put(x, y int, str string, style tcell.Style) (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cells.Put(x, y, str, style)
}

func (s *Screen) PutStr(x, y int, str string) { s.PutStrStyled(x, y, str, s.style) }

func (s *Screen) PutStrStyled(x, y int, str string, style tcell.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for str != "" && x < s.cols && y < s.rows {
		rest, width := s.cells.Put(x, y, str, style)
		if width <= 0 {
			// A rune the buffer would not place. Skipping the cell rather than
			// the string keeps one bad rune from truncating a line.
			width = 1
		}
		if rest == str {
			break
		}
		str = rest
		x += width
	}
}

func (s *Screen) SetStyle(style tcell.Style) {
	s.mu.Lock()
	s.style = style
	s.mu.Unlock()
}

func (s *Screen) Fill(r rune, style tcell.Style) {
	s.mu.Lock()
	s.cells.Fill(r, style)
	s.mu.Unlock()
}

func (s *Screen) Clear() { s.Fill(' ', s.style) }

// ------------------------------------------------------------------ the frame

// Show writes the cells that changed since the last frame.
func (s *Screen) Show() {
	s.mu.Lock()
	s.draw()
	s.flushLocked()
	s.mu.Unlock()
}

// Sync repaints everything, for when what is on the terminal is not what this
// screen last put there — after a resize, or after another program wrote to it.
func (s *Screen) Sync() {
	s.mu.Lock()
	// Cleared first, and not only for tidiness. Invalidate works by forgetting
	// what was last drawn, but a cell that is empty was last drawn as the empty
	// string and still is one, so it does not come back dirty and would never be
	// repainted. Anything the screen believes is blank has to be blanked here or
	// a Sync leaves the previous occupant's pixels behind.
	s.buf = append(s.buf, clearScreenSeq(-1, -1)...)
	s.haveStyle, s.penValid = false, false
	s.cells.Invalidate()
	s.haveStyle = false
	s.curDirty = true
	s.draw()
	s.flushLocked()
	s.mu.Unlock()
}

// draw appends every dirty cell to the frame buffer. Called with the lock held.
//
// Cells are addressed individually rather than assumed to be consecutive: only
// the dirty ones are written, so the next cell is rarely the next column.
func (s *Screen) draw() {
	if s.sink != nil {
		// Anything already queued as sequences has to land before the cells do.
		// A Sync queues a clear-screen, and a clear arriving after the cells
		// were written would erase the frame it was supposed to precede.
		s.flushLocked()
		s.drawDirect()
		if s.curDirty {
			s.buf = append(s.buf, showCursorSeq(s.curX, s.curY)...)
			s.curDirty = false
		}
		return
	}

	drew := false
	for y := 0; y < s.rows; y++ {
		for x := 0; x < s.cols; x++ {
			if !s.cells.Dirty(x, y) {
				continue
			}
			str, style, width := s.cells.Get(x, y)
			if width < 1 {
				width = 1
			}
			s.cell(x, y, str, style)
			// The glyph advanced the terminal's cursor. At the right-hand edge
			// it did not — the terminal holds a pending wrap instead — so the
			// position is given up rather than guessed at.
			if x+width < s.cols {
				s.penX, s.penY, s.penValid = x+width, y, true
			} else {
				s.penValid = false
			}
			s.cells.SetDirty(x, y, false)
			drew = true
			// A wide rune occupies the cell to its right as well; that cell has
			// nothing of its own to draw.
			for i := 1; i < width && x+i < s.cols; i++ {
				s.cells.SetDirty(x+i, y, false)
			}
			x += width - 1
		}
	}
	// The cursor is emitted after the cells because drawing moved it.
	if drew || s.curDirty {
		s.buf = append(s.buf, showCursorSeq(s.curX, s.curY)...)
		s.curDirty = false
		// Placing the cursor moved it somewhere the next frame cannot assume.
		s.penValid = false
	}
}

// cell writes one cell, saying only what changed since the last one.
func (s *Screen) cell(x, y int, str string, style tcell.Style) {
	if str == "" {
		str = " "
	}
	// Guarded, and decoded rather than converted. []rune(str) allocates, and
	// this runs once per cell of every frame — for a full-screen animation that
	// is ten thousand allocations sixty times a second, for a lookup that in
	// almost every program finds nothing because no fallback was ever
	// registered.
	if len(s.fallback) > 0 {
		if r, _ := utf8.DecodeRuneInString(str); r != utf8.RuneError {
			if sub, ok := s.fallback[r]; ok {
				str = sub
			}
		}
	}

	// v3 reads a style through accessors rather than taking it apart with
	// Decompose, and carries underline as a style rather than an attribute bit,
	// so the underline is asked for by name instead of assumed absent.
	fgc, bgc := style.GetForeground(), style.GetBackground()
	attrs := style.GetAttributes()
	fg, bg := colorHex(fgc), colorHex(bgc)
	// tcell renders reverse video by swapping, and so does every terminal, so
	// it is passed through as SGR 7 rather than swapped here.
	us := style.GetUnderlineStyle()

	// The cursor is only moved when it is not already there. Cells are written
	// in order, so after drawing one the terminal's cursor sits at the start of
	// the next — and in a full-screen animation, where every cell is dirty, that
	// next cell is the one about to be drawn. A position is about nine bytes
	// against a cell's forty, so eliding it takes roughly a quarter off the
	// frame. Not attempted at the right-hand edge, where the terminal is in the
	// pending-wrap state and where the cursor sits is not worth reasoning about.
	if !s.penValid || s.penX != x || s.penY != y {
		s.buf = appendCUP(s.buf, x, y)
	}
	if !s.haveStyle || fg != s.lastFg || bg != s.lastBg || attrs != s.lastAttrs || us != s.lastUS {
		s.buf = appendSGR(s.buf, fg, bg, attrs, us)
		s.haveStyle = true
		s.lastFg, s.lastBg, s.lastAttrs, s.lastUS = fg, bg, attrs, us
	}
	s.buf = append(s.buf, str...)
}

// flushLocked hands the frame to the terminal as a single write.
func (s *Screen) flushLocked() {
	if len(s.buf) == 0 {
		return
	}
	// Write, not WriteString: converting would copy the whole frame, which is
	// the allocation this buffer exists to avoid.
	s.term.Write(s.buf)
	s.buf = s.buf[:0]
}

// colorHex renders a tcell colour as 0xRRGGBB, or -1 for "whatever the
// terminal's default is", which the sequence builders substitute for.
func colorHex(c tcell.Color) int {
	if !c.Valid() {
		return -1
	}
	return int(c.Hex())
}

// ----------------------------------------------------------------- the cursor

func (s *Screen) ShowCursor(x, y int) {
	s.mu.Lock()
	if x != s.curX || y != s.curY {
		s.curX, s.curY, s.curDirty = x, y, true
	}
	s.mu.Unlock()
}

func (s *Screen) HideCursor() { s.ShowCursor(-1, -1) }

func (s *Screen) SetCursorStyle(cs tcell.CursorStyle, _ ...tcell.Color) {
	class, ok := cursorClasses[cs]
	if !ok {
		return
	}
	s.mu.Lock()
	if class != s.curStyle {
		s.curStyle = class
		s.buf = append(s.buf, setCursorStyleSeq(class)...)
	}
	s.mu.Unlock()
}

// ------------------------------------------------------------------- events

// EventQ is where events are delivered.
//
// v3 hands the channel out rather than wrapping it in PollEvent and
// ChannelEvents, and a caller may write to it as well as read from it. That
// replaces five methods with one, and it is why this screen no longer needs a
// forwarding goroutine of its own.
//
// The channel is closed by Fini, which is the contract callers rely on to
// notice a screen going away: an animation loop reads it and returns when it
// closes, so a closed window ends its goroutine rather than spinning.
func (s *Screen) EventQ() chan tcell.Event { return s.evch }

// post delivers an event, dropping it if the queue is full.
//
// Dropping is deliberate. These arrive from DOM handlers, which run on the one
// thread the program itself runs on, so blocking here would deadlock the page
// rather than apply back-pressure to anything.
func (s *Screen) post(ev tcell.Event) {
	select {
	case <-s.stopq:
		// Finalised. The queue is closed and sending would panic.
		return
	default:
	}
	select {
	case s.evch <- ev:
	default:
	}
}

// ------------------------------------------------------- terminal capabilities

// Colors reports truecolor: the sequences this writes are 24-bit, and tcell has
// already resolved its palette to RGB by the time a cell is drawn.
func (s *Screen) Colors() int { return 1 << 24 }

func (s *Screen) CharacterSet() string { return "UTF-8" }

// Terminal names what this is pretending to be, the way $TERM and the terminal
// version would. Callers use it to decide what a terminal is likely to support;
// xterm-256color is what xterm-go implements.
func (s *Screen) Terminal() (string, string) { return "xterm-256color", "xterm-go" }

// KeyboardProtocol reports how keys arrive.
//
// None of these really applies: they name escape-sequence protocols, and keys
// here come from DOM events instead. Legacy is the honest one to claim, because
// callers read this to decide what to expect — and what this delivers is a
// press with modifiers and no release, which is what legacy means. Claiming one
// of the richer protocols would promise release events that never come.
func (s *Screen) KeyboardProtocol() tcell.KeyProtocol { return tcell.LegacyKeyboard }

func (s *Screen) RegisterRuneFallback(r rune, subst string) {
	s.mu.Lock()
	s.fallback[r] = subst
	s.mu.Unlock()
}

func (s *Screen) UnregisterRuneFallback(r rune) {
	s.mu.Lock()
	delete(s.fallback, r)
	s.mu.Unlock()
}

func (s *Screen) Beep() error {
	s.mu.Lock()
	s.buf = append(s.buf, '\a')
	s.flushLocked()
	s.mu.Unlock()
	return nil
}

func (s *Screen) SetTitle(title string) {
	s.mu.Lock()
	s.buf = append(s.buf, setTitleSeq(title)...)
	s.mu.Unlock()
}

// Suspend stops drawing and restores the terminal.
//
// tcell v2.13.10's own wasm screen cannot do this: wScreen.Suspend takes the
// screen's lock and returns without releasing it on the path where the screen
// was running, and Resume has the same defect when it is already running
// (wscreen.go:466). Suspending a live screen there leaks its mutex for good and
// every later Show or key event on it blocks forever — which, in a page where
// Go has one thread, stops the whole tab rather than just that window.
func (s *Screen) Suspend() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, "\x1b[0m\x1b[?25h"...)
	s.haveStyle, s.penValid = false, false
	s.flushLocked()
	return nil
}

// Resume repaints. Whatever was on the terminal while this screen was suspended
// is not what it last put there.
func (s *Screen) Resume() error {
	s.Sync()
	return nil
}

// ------------------------------------------------------------------- no-ops

// Paste and focus reporting are not wired up. They are not errors — a
// program may ask for them and carry on without — so they are accepted and
// ignored rather than refused.
func (s *Screen) EnablePaste()  {}
func (s *Screen) DisablePaste() {}
func (s *Screen) EnableFocus()  {}
func (s *Screen) DisableFocus() {}

// ShowNotification would be OSC 9 or OSC 777, which this emulator does not
// implement. A page that wants to notify has the browser's own API for it.
func (s *Screen) ShowNotification(title, body string) {}

// Resize is tcell's way of placing the screen inside a larger area. Its own
// terminal screen ignores it too.
func (s *Screen) Resize(int, int, int, int) {}

// LockRegion marks cells as not to be redrawn. The frame is a single write
// here, so there is nothing a partial redraw would save.
func (s *Screen) LockRegion(x, y, width, height int, lock bool) {}

// Tty reports that there is no tty behind this. There is a terminal, but not
// one tcell could drive itself: the whole point of this screen is that the
// emulator is in the same process and takes bytes through a Go call.
func (s *Screen) Tty() (tcell.Tty, bool) { return nil, false }

// Clipboard access would be OSC 52, which this emulator does not implement.
// HasClipboard says so, which v3 added so a caller can ask rather than write
// and wonder; the two calls are accepted and ignored for anyone who does not.
func (s *Screen) HasClipboard() bool  { return false }
func (s *Screen) SetClipboard([]byte) {}
func (s *Screen) GetClipboard()       {}

// cursorClasses maps tcell's cursor styles onto the class names the sequence
// builder knows. It is the same set tcell's web screen uses.
var cursorClasses = map[tcell.CursorStyle]string{
	tcell.CursorStyleDefault:           "cursor-blinking-block",
	tcell.CursorStyleBlinkingBlock:     "cursor-blinking-block",
	tcell.CursorStyleSteadyBlock:       "cursor-steady-block",
	tcell.CursorStyleBlinkingUnderline: "cursor-blinking-underline",
	tcell.CursorStyleSteadyUnderline:   "cursor-steady-underline",
	tcell.CursorStyleBlinkingBar:       "cursor-blinking-bar",
	tcell.CursorStyleSteadyBar:         "cursor-steady-bar",
}

// Compile-time proof that this is a tcell.Screen. Every method above exists
// because the interface asks for it, and this is what says so.
var _ tcell.Screen = (*Screen)(nil)
