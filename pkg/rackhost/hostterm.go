package rackhost

import (
	"os"
	"os/exec"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The Host Terminal model, natively: the user's own shell in a pty, on a
// terminal in memory, its screen on a quad as the Terminal Animation's is.
// The page reaches a host shell through its server's agent, and only when
// the server was started with --shell; a rack on the host is already where
// the shell runs, and its keyboard is already the user's.
//
// The shell starts when the model is first shown and runs while the rack
// does, as the page's pane stays mounted: leaving the model and coming back
// finds it as it was left. With the panel's window put away, the keys are
// the shell's (racktui's Typist); Ctrl+] brings the window back.

// shellView is the shell running, or the screen of the one that last ran.
type shellView struct {
	term *vterm
	// pty is the shell's terminal device, sized with the terminal.
	pty *os.File
	// keys is what is typed, written to the shell by a goroutine of its own
	// so that a shell not reading never holds up the panel.
	keys chan []byte
	// done is closed when the shell has exited.
	done chan struct{}
}

// shellExitedMsg is written under a shell's last screen when it exits.
const shellExitedMsg = "\r\n\x1b[7m the shell has exited: Enter starts another \x1b[0m"

// running reports whether a shell is running.
func (sh *shellView) running() bool {
	if sh.done == nil {
		return false
	}
	select {
	case <-sh.done:
		return false
	default:
		return true
	}
}

// start runs the user's shell, $SHELL, on a fresh terminal.
func (sh *shellView) start() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	t := newVterm(animCols, animRows, false)
	t.cursor = true
	sh.term = t
	done := make(chan struct{})
	sh.done = done
	cmd := exec.Command(shell) //nolint:gosec // the user's own shell, on the user's own keyboard
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := startPty(cmd, animCols, animRows)
	if err != nil {
		t.put([]byte("the shell would not start: " + err.Error() + shellExitedMsg))
		close(done)
		return
	}
	sh.pty = f
	keys := make(chan []byte, 256)
	sh.keys = keys
	// The emulator's answers to the shell's questions go back to it, as a
	// terminal's do.
	t.term.OnData = func(s string) {
		select {
		case keys <- []byte(s):
		default:
		}
	}
	go func() {
		for {
			select {
			case b := <-keys:
				if _, err := f.Write(b); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	go func() {
		defer close(done)
		buf := make([]byte, 32<<10)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				t.put(buf[:n])
			}
			if err != nil {
				break
			}
		}
		// How it ended is on its screen; the pty is done with.
		_ = cmd.Wait() //nolint:errcheck // see above
		_ = f.Close()  //nolint:errcheck // see above
		t.put([]byte(shellExitedMsg))
	}()
}

// shellFigure is the host shell's screen now, on a quad, the shell started
// the first time it is shown. Called with r.mu held.
func (r *Rack) shellFigure() *rasterview.Quad {
	sh := &r.shell
	if sh.term == nil {
		sh.start()
	}
	return termQuad(sh.term.picture())
}

// TypeKey types ev into the host shell, while it is the model on show
// (racktui's Typist). Enter after the shell has exited starts another.
func (r *Rack) TypeKey(ev *tcell.EventKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.model() != "hostterm" || r.value("power-sw") == "0" {
		return false
	}
	sh := &r.shell
	if !sh.running() {
		if ev.Key() == tcell.KeyEnter {
			sh.start()
		}
		return true
	}
	if b := keyBytes(ev, sh.term.appCursor()); len(b) > 0 {
		select {
		case sh.keys <- b:
		default: // a shell that has stopped reading loses keys, not the panel
		}
	}
	return true
}
