package rackhost

import (
	"os"
	"os/exec"
	"sync"
	"time"

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
	pty *ptyFile
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
	pf := &ptyFile{f: f}
	sh.pty, sh.sig = pf, sig
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
		pf.close()
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
	// Waited for once its pty has closed, which hangs it up; a shell that
	// ignores the hangup is given a moment and then killed, or it ran on as
	// the user with nothing left to reach it, and this goroutine with it.
	wait := func() {
		exited := make(chan struct{})
		go func() {
			_ = cmd.Wait() //nolint:errcheck // how it ended is on its screen
			close(exited)
		}()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill() //nolint:errcheck // it may have just gone
			<-exited
		}
	}
	return f, nil, wait, nil
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
			r.websh.run = webshShell(nil)
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

// TypeKey types ev into the shell, or the desk, on show (racktui's Typist).
func (r *Rack) TypeKey(ev *tcell.EventKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value("power-sw") == "0" {
		return false
	}
	key := r.model()
	if key == "desk" {
		r.desk.typeKey(ev)
		return true
	}
	sh := r.shellOf(key)
	if sh == nil {
		return false
	}
	if !sh.running() {
		// Enter after the shell has exited starts another.
		if ev.Key() == tcell.KeyEnter {
			sh.start()
		}
		return true
	}
	sh.typeKey(ev)
	return true
}

// typeKey types ev into the shell, if it is running.
func (sh *shellView) typeKey(ev *tcell.EventKey) {
	if !sh.running() {
		return
	}
	b := keyBytes(ev, sh.term.appCursor())
	if len(b) == 0 {
		return
	}
	if sh.sig != nil {
		sh.pty.use(func(f *os.File) { signalFor(f, b, sh.sig) })
	}
	select {
	case sh.keys <- b:
	default: // a shell that has stopped reading loses keys, not the panel
	}
}

// kill ends the shell by closing its terminal, as closing a window hangs up
// on what is in it.
func (sh *shellView) kill() {
	if sh.pty != nil {
		sh.pty.close() // the shell's reader sees the end
	}
}

// ptyFile is a shell's pty and whether it has been closed: closed by the
// goroutine that sees the shell end, while the panel may be sizing it, and
// sizing reads its descriptor, which a close takes away.
type ptyFile struct {
	mu     sync.Mutex
	f      *os.File
	closed bool
}

// use runs fn on the pty, if it is still open.
func (p *ptyFile) use(fn func(*os.File)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		fn(p.f)
	}
}

// close closes the pty, once.
func (p *ptyFile) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		_ = p.f.Close() //nolint:errcheck // the shell is over either way
	}
}
