package rackhost

import (
	"os"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/0magnet/websh/console"
)

// The Terminal model, natively: websh — the shell, applets and line editor
// the page compiles into itself — run in this process by websh's console, on
// a pty of its own, as progterm runs it in a tab. On the host it works on
// the machine's own files and programs, where the page's works on a
// filesystem kept in the browser.

// webshShell starts websh on a new pty cols by rows.
func webshShell(cols, rows int) (*os.File, chan<- os.Signal, func(), error) {
	ptm, pts, err := pty.Open()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := setPtySize(ptm, cols, rows); err != nil {
		_ = ptm.Close() //nolint:errcheck // never used
		_ = pts.Close() //nolint:errcheck // never used
		return nil, nil, nil, err
	}
	sig := make(chan os.Signal, 4)
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		_, _ = console.Run(console.Options{ //nolint:errcheck // how it ended is on its screen
			In: pts, Out: pts, Signals: sig,
			Env: []string{"TERM=xterm-256color", "COLORTERM=truecolor"},
		})
		// With the shell's copy closed, and its programs', the pty's reads end.
		_ = pts.Close() //nolint:errcheck // see above
	}()
	return ptm, sig, func() { <-ended }, nil
}

// signalFor sends the signal a typed interrupt or quit character raises,
// when the terminal's settings say the kernel would raise one: ISIG on, as
// while a command runs, and not at a prompt or in a full-screen program.
func signalFor(ptm *os.File, typed []byte, sig chan<- os.Signal) {
	t, err := unix.IoctlGetTermios(int(ptm.Fd()), unix.TCGETS) //nolint:gosec // a file descriptor fits an int
	if err != nil || t.Lflag&unix.ISIG == 0 || len(typed) != 1 {
		return
	}
	var s os.Signal
	switch typed[0] {
	case t.Cc[unix.VINTR]:
		s = os.Interrupt
	case t.Cc[unix.VQUIT]:
		s = syscall.SIGQUIT
	default:
		return
	}
	select {
	case sig <- s:
	default: // one waiting is as good as several
	}
}
