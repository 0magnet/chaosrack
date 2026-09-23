//go:build js && wasm

package attractor

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/web"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/tty"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// `rack` — the control surface, typed at a prompt. Type rack and the panel
// takes that terminal until you quit it; the prompt comes back after.
//
// THE PANEL IS NOT A MODULE, and the distinction is the whole design. A module
// sits in a bay and controls part of the instrument. This contains the
// instrument — every bay, every control — so a bay is the one place it cannot
// go without the panel containing itself.
//
// Which is also why the terminal to type it in is the DESK's, and not only the
// Terminal model. The model is a shell drawn on a plane inside the rack, so the
// panel there is the rack containing a shell containing the rack: it works, but
// it is the same fold. On the desk the terminal is a window on the desktop the
// rack is also a window on, and the two sit beside each other.
//
// It reaches the rack by calling it. pkg/racktui is the same package
// `chaosrack tui` runs from a shell on the host; there it reaches the rack
// over a cable (CDP, window.rackctl, a round trip per read) and here it is a
// function call, because this IS the process the rack is in. One panel, two
// ways to be attached to what it drives.
//
// What made it possible: websh grew Shell.Exec, so a command that is not a
// built-in applet can be a Go function in the program the shell is embedded
// in rather than a separate wasm instance exec'd off the filesystem. And tcell
// takes its screen from a tty, so the panel draws through the shell's own
// stdout and reads the shell's own stdin — no second terminal, no globals, and
// raw mode is the switch every full-screen applet in websh already uses.

// rackShellCommand is the Exec hook websh offers every non-applet command.
func rackShellCommand(ctx context.Context, args []string) (int, bool) {
	if len(args) == 0 || args[0] != "rack" {
		return 0, false
	}
	hc := interp.HandlerCtx(ctx)
	// The session this was TYPED INTO, which is not always the one the rack
	// built for its Terminal model: the desk's terminal windows carry this same
	// hook now, and a panel that took raw mode on the wrong shell would leave
	// the window it was typed in echoing while it drew on a terminal parked off
	// screen that nobody is looking at.
	return runRackPanel(web.SessionForContext(ctx), hc.Stdin, hc.Stdout, hc.Stderr), true
}

// runRackPanel draws the panel on the shell's own stdio until it is quit.
func runRackPanel(sess *web.Session, stdin io.Reader, stdout, stderr io.Writer) int {
	if sess == nil {
		_, _ = fmt.Fprintln(stderr, "rack: no terminal") //nolint:errcheck // a closed stderr is not a reason to do anything else
		return 1
	}
	// Raw input for as long as the panel has the terminal — no echo, no
	// newline translation — which is what less and the line editor do here.
	if sh := sess.Shell; sh != nil && sh.RawMode != nil {
		sh.RawMode(true)
		defer sh.RawMode(false)
	}
	t := &shellTty{in: stdin, out: stdout, sess: sess}
	// The panel has to hear the terminal change size, and in a page it changes
	// for a reason a real one never does: the cell shrinks under it. Zooming out
	// is how the whole rack is made to fit, so a panel that did not reflow would
	// answer the gesture by drawing the same small picture in a corner of a much
	// bigger grid.
	//
	// Terminal.OnResize rather than Core.OnResize: Open owns the Core hook, and
	// taking it would stop the renderer reallocating. Saved and restored around
	// the panel, so whatever the embedder had set survives it.
	if term := sess.Term; term != nil {
		prev := term.OnResize
		term.OnResize = func(cols, rows int) {
			if prev != nil {
				prev(cols, rows)
			}
			t.tellResize()
		}
		defer func() { term.OnResize = prev }()
	}
	ctx := racktui.WithScreen(context.Background(), func() (tcell.Screen, error) {
		// OptTerm because tcell names a wasm terminal "ghostty-truecolor" and
		// xterm-go answers DA as "xterm" — left alone it emits sequences this
		// terminal does not implement and the panel draws nothing at all.
		// OptAltScreen(false) because the alternate screen is a thing a
		// terminal emulator switches to, and the shell's scrollback is what
		// the user wants back when the panel quits.
		return tcell.NewTerminfoScreenFromTty(t,
			tcell.OptTerm("xterm-256color"), tcell.OptAltScreen(false))
	})
	if err := racktui.Run(ctx, inPageRack{}); err != nil {
		_, _ = fmt.Fprintln(stderr, "rack:", err) //nolint:errcheck // as above
		return 1
	}
	return 0
}

// shellTty is tcell's terminal, which here is the shell's stdio.
//
// Nothing has to be simulated: the pipes are real, raw mode is the shell's,
// and the size comes from the terminal the shell is mounted on. Start, Stop
// and Drain are no-ops because none of what they exist for — saving termios,
// waking a blocked read on a device — is true of a pipe in a page.
type shellTty struct {
	in   io.Reader
	out  io.Writer
	sess *web.Session
	// resize is tcell's own channel, handed over at Init and taken back at
	// Fini. Guarded because the terminal's resize arrives on the JS event
	// loop and tcell swaps the channel from its own goroutine.
	mu     sync.Mutex
	resize chan<- bool
}

func (t *shellTty) Start() error { return nil }
func (t *shellTty) Stop() error  { return nil }
func (t *shellTty) Drain() error { return nil }
func (t *shellTty) Close() error { return nil }

func (t *shellTty) NotifyResize(ch chan<- bool) {
	t.mu.Lock()
	t.resize = ch
	t.mu.Unlock()
}

// tellResize wakes tcell, and drops the news if it is not listening.
//
// Non-blocking on purpose: this runs on the JS event loop, which in a
// single-threaded wasm runtime is the only thread there is. Blocking on a
// channel tcell is not reading would stop the page, not just the panel — and a
// resize that arrives while one is already queued says nothing the queued one
// does not, since tcell re-reads the size rather than taking it from here.
func (t *shellTty) tellResize() {
	t.mu.Lock()
	ch := t.resize
	t.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- true:
	default:
	}
}

func (t *shellTty) Read(p []byte) (int, error)  { return t.in.Read(p) }
func (t *shellTty) Write(p []byte) (int, error) { return t.out.Write(p) }

func (t *shellTty) WindowSize() (tty.WindowSize, error) {
	ws := tty.WindowSize{Width: 80, Height: 24}
	if t.sess != nil && t.sess.Term != nil {
		if c := t.sess.Term.Core.Cols(); c > 0 {
			ws.Width = c
		}
		if r := t.sess.Term.Core.Rows(); r > 0 {
			ws.Height = r
		}
	}
	return ws, nil
}
