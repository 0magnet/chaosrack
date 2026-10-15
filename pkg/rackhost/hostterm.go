package rackhost

import (
	"os"
	"os/exec"

	"github.com/gdamore/tcell/v3"

	"github.com/0magnet/chaosrack/pkg/rasterview"
)

// The two shell models, natively, each a shell on a pty on a terminal in
// memory, its screen on a quad as the Terminal Animation's is.
//
// The Host Terminal is the user's own shell. The page reaches one through
// its server's agent, and only when the server was started with --shell; a
// rack on the host is already where the shell runs, and its keyboard is
// already the user's. The Terminal is websh, which the page compiles into
// itself (websh.go).
//
// A shell starts when its model is first shown and runs while the rack
// does, as the page's panes stay mounted: leaving the model and coming back
// finds it as it was left. With the panel's window put away, the keys are
// the shell's (racktui's Typist); Ctrl+] brings the window back.

// shellStart starts a shell on a terminal cols by rows. It is the pty's
// master; where Ctrl+C and Ctrl+\ go as signals, for a shell in this process,
// whose terminal the kernel has no process group to signal (nil for one in a
// process of its own); and how to wait for the shell once its pty has closed.
type shellStart func(cols, rows int) (ptm *os.File, sig chan<- os.Signal, wait func(), err error)

// shellView is a shell running, or the screen of the one that last ran.
type shellView struct {
	run  shellStart
	term *vterm
	// pty is the shell's terminal device, sized with the terminal.
	pty *os.File
	// sig is where a typed interrupt goes as a signal; see shellStart.
	sig chan<- os.Signal
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

// start runs the shell on a fresh terminal.
func (sh *shellView) start() {
	t := newVterm(animCols, animRows, false)
	t.cursor = true
	sh.term = t
	done := make(chan struct{})
	sh.done = done
	f, sig, wait, err := sh.run(animCols, animRows)
	if err != nil {
		t.put([]byte("the shell would not start: " + err.Error() + shellExitedMsg))
		close(done)
		return
	}
	sh.pty, sh.sig = f, sig
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
		wait()
		_ = f.Close() //nolint:errcheck // see above
		t.put([]byte(shellExitedMsg))
	}()
}

// hostShell starts the user's shell, $SHELL, in a process of its own.
func hostShell(cols, rows int) (*os.File, chan<- os.Signal, func(), error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell) //nolint:gosec // the user's own shell, on the user's own keyboard
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := startPty(cmd, cols, rows)
	if err != nil {
		return nil, nil, nil, err
	}
	return f, nil, func() { _ = cmd.Wait() }, nil //nolint:errcheck // how it ended is on its screen
}

// shellOf is model key's shell, or nil for a model that is not one. Called
// with r.mu held.
func (r *Rack) shellOf(key string) *shellView {
	switch key {
	case "hostterm":
		if r.shell.run == nil {
			r.shell.run = hostShell
		}
		return &r.shell
	case "terminal":
		if r.websh.run == nil {
			r.websh.run = webshShell
		}
		return &r.websh
	}
	return nil
}

// figure is shell sh's screen now, on a quad, the shell started the
// first time it is shown. Called with r.mu held.
func (sh *shellView) figure() *rasterview.Quad {
	if sh.term == nil {
		sh.start()
	}
	return termQuad(sh.term.picture())
}

// TypeKey types ev into the shell on show (racktui's Typist). Enter after
// the shell has exited starts another.
func (r *Rack) TypeKey(ev *tcell.EventKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	sh := r.shellOf(r.model())
	if sh == nil || r.value("power-sw") == "0" {
		return false
	}
	if !sh.running() {
		if ev.Key() == tcell.KeyEnter {
			sh.start()
		}
		return true
	}
	b := keyBytes(ev, sh.term.appCursor())
	if len(b) == 0 {
		return true
	}
	if sh.sig != nil {
		signalFor(sh.pty, b, sh.sig)
	}
	select {
	case sh.keys <- b:
	default: // a shell that has stopped reading loses keys, not the panel
	}
	return true
}
