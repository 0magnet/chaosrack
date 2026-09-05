//go:build js && wasm

package deskapp

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"

	"github.com/0magnet/tuiwasm/demos"
	"github.com/0magnet/tuiwasm/play"
)

// RegisterDemoApplets turns every demo into a shell command.
//
// They used to be desk apps, each opening its own window with its own
// terminal. That made them things you launch rather than things you run, and
// it meant Ctrl+C had nowhere to return to — there was no shell underneath,
// only a window with a close button.
//
// As commands they behave the way the programs they demonstrate behave: they
// are typed at a prompt, they take the terminal for as long as they run, and
// they give it back. `help` lists them and `ls /bin` finds them, which is more
// discoverable than a launcher menu for anyone who already knows a shell.
func RegisterDemoApplets() {
	for _, d := range demos.All() {
		d := d
		shell.RegisterApplet(d.Name, d.Desc, func(ctx context.Context, s *shell.Shell, hc *interp.HandlerContext, _ []string) int {
			return runDemo(ctx, s, hc, d)
		})
	}
}

func runDemo(ctx context.Context, s *shell.Shell, hc *interp.HandlerContext, d demos.Demo) int {
	// A text demo writes and stops. It needs no screen, no keys and no
	// teardown — it is an ordinary command that happens to print in color.
	if d.Text != nil {
		cols, rows := 80, 24
		if s.Size != nil {
			cols, rows = s.Size()
		}
		if err := d.Text(hc.Stdout, cols, rows); err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %v\n", d.Name, err) //nolint:errcheck
			return 1
		}
		return 0
	}

	// A tcell demo paints the whole screen and reads its own keys, so it needs
	// the terminal itself rather than a pipe. The shell has one; borrow it.
	t := shellTerminal()
	if !t.ok {
		fmt.Fprintf(hc.Stderr, "%s: no terminal to draw on\n", d.Name) //nolint:errcheck
		return 1
	}

	// Raw mode stops the shell interpreting the keys the demo is about to
	// read. The demo attaches its own handlers to the terminal underneath.
	if s.RawMode != nil {
		s.RawMode(true)
		defer s.RawMode(false)
	}

	sess, err := play.In(d, t.term, js.Undefined())
	if err != nil {
		fmt.Fprintf(hc.Stderr, "%s: %v\n", d.Name, err) //nolint:errcheck
		return 1
	}

	// Ctrl+C reaches here as a canceled context: websh's session watches the
	// raw input for it and cancels whatever is running. That is the whole of
	// the "back to the shell" story — the demo stops, the screen is handed
	// back, and the prompt returns.
	<-ctx.Done()

	sess.Close()
	// Fini leaves the cursor and styling reset but not the scrollback, so
	// clear what the demo painted rather than leaving the prompt in the
	// middle of it.
	fmt.Fprint(hc.Stdout, "\x1b[H\x1b[2J") //nolint:errcheck
	return 0
}
