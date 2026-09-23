//go:build js && wasm

package attractor

import (
	"context"
	"fmt"
	"io"

	"github.com/0magnet/sh/v3/interp"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/tty"

	"github.com/0magnet/chaosrack/pkg/racktui"
)

// `rack` — the control surface, typed at the rack's own prompt.
//
// The Terminal model is a websh shell on a plane you can rotate, and this is
// a command in it. Type rack and the panel takes the terminal until you quit
// it; the prompt comes back after.
//
// THE PANEL IS NOT A MODULE, and the distinction is the whole design. A module
// sits in a bay and controls part of the instrument. This contains the
// instrument — every bay, every control — so a bay is the one place it cannot
// go without the panel containing itself.
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
	return runRackPanel(hc.Stdin, hc.Stdout, hc.Stderr), true
}

// runRackPanel draws the panel on the shell's own stdio until it is quit.
func runRackPanel(stdin io.Reader, stdout, stderr io.Writer) int {
	if termSession == nil {
		_, _ = fmt.Fprintln(stderr, "rack: no terminal") //nolint:errcheck // a closed stderr is not a reason to do anything else
		return 1
	}
	// Raw input for as long as the panel has the terminal — no echo, no
	// newline translation — which is what less and the line editor do here.
	if sh := termSession.Shell; sh != nil && sh.RawMode != nil {
		sh.RawMode(true)
		defer sh.RawMode(false)
	}
	t := &shellTty{in: stdin, out: stdout}
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
	in  io.Reader
	out io.Writer
}

func (t *shellTty) Start() error                { return nil }
func (t *shellTty) Stop() error                 { return nil }
func (t *shellTty) Drain() error                { return nil }
func (t *shellTty) Close() error                { return nil }
func (t *shellTty) NotifyResize(chan<- bool)    {}
func (t *shellTty) Read(p []byte) (int, error)  { return t.in.Read(p) }
func (t *shellTty) Write(p []byte) (int, error) { return t.out.Write(p) }

func (t *shellTty) WindowSize() (tty.WindowSize, error) {
	ws := tty.WindowSize{Width: 80, Height: 24}
	if termSession != nil && termSession.Term != nil {
		if c := termSession.Term.Core.Cols(); c > 0 {
			ws.Width = c
		}
		if r := termSession.Term.Core.Rows(); r > 0 {
			ws.Height = r
		}
	}
	return ws, nil
}
