//go:build !js

// Package hostagent is the half of the desk that runs on the machine.
//
// The desk has always been a desktop environment — a window manager, a panel, a
// file manager and a shell — that could not touch anything. Its shell is websh,
// a real Bash interpreter compiled to wasm, and its filesystem is an in-memory
// afero persisted to IndexedDB. Everything works; none of it is yours.
//
// This package is what makes it yours, and it is deliberately a separate
// package in the panes module rather than part of the desk. The desk is a
// window manager and must stay one: it has a single dependency and builds only
// for js/wasm. The panes module is already the layer that "knows both the desk
// and the program it puts in a window", and a program that happens to run on
// the host rather than in the tab is still a program.
//
// # What this costs, said plainly
//
// A browser tab that can run commands as you is the most valuable target on the
// machine, and any page you visit may attempt a connection to localhost. So the
// defaults are: nothing is served unless it is asked for, the listener is
// 127.0.0.1, the Origin header must match the page the agent itself served, and
// a token generated per run must be presented.
//
// Of those, the Origin check is the load-bearing one. A browser sets Origin on
// every WebSocket handshake and a page cannot forge it, so it is what actually
// stops a hostile web page. The token is the weaker guard, and honestly so: it
// stops another *browser* page and it stops a careless local fetch, but a local
// process running as you can read it out of the served page — and a process
// running as you already has your shell without asking this one. It is defense
// in depth, not a boundary.
package hostagent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// SessionConfig is what to start and how big.
type SessionConfig struct {
	// Shell is the program to run. Empty picks $SHELL, then /bin/sh.
	Shell string

	// Args are its arguments. Nil is right for an interactive shell: given a
	// pty, a shell decides it is interactive by asking whether its stdin is
	// a terminal, so there is no flag to pass.
	Args []string

	// Dir is the working directory. Empty means the agent's own.
	Dir string

	// Cols and Rows are the initial grid. Zero picks 80x24.
	//
	// They matter at START and not only afterwards: a shell reads the window
	// size when it draws its first prompt, so a session that begins at the
	// wrong size and is corrected a frame later shows a wrapped prompt that
	// nothing redraws.
	Cols, Rows int

	// Env is the child's environment. Nil inherits the agent's.
	Env []string
}

// Session is a shell attached to a pseudo-terminal.
//
// It is an io.ReadWriter over the pty master, so the transport can be anything
// that moves bytes. Nothing here knows about WebSockets — that is handler.go's
// job, and keeping them apart is what lets this be tested by reading and
// writing an ordinary pipe.
type Session struct {
	f    *os.File
	cmd  *exec.Cmd
	done chan struct{}
}

// Start launches the shell on a new pty.
func Start(cfg SessionConfig) (*Session, error) {
	shell := cfg.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cols, rows := cfg.Cols, cfg.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	cmd := exec.Command(shell, cfg.Args...) //nolint:gosec // the shell is the point
	cmd.Dir = cfg.Dir
	env := cfg.Env
	if env == nil {
		env = os.Environ()
	}
	// TERM has to say what the far end actually emulates. xterm-go is a port
	// of xterm.js, which is an xterm with 256 colors, and a program that
	// believes it is talking to a "dumb" terminal — which is what an
	// inherited or missing TERM often amounts to — will not draw at all.
	cmd.Env = append(env, "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec // bounded below
	if err != nil {
		return nil, fmt.Errorf("hostagent: starting %s: %w", shell, err)
	}

	s := &Session{f: f, cmd: cmd, done: make(chan struct{})}
	go func() {
		// Reaping matters even though nothing reads the status: without a
		// Wait the finished shell stays a zombie for as long as the agent
		// runs, and an agent left open all day accumulates one per window.
		_ = cmd.Wait() //nolint:errcheck // nothing reads the status; this is only reaping
		close(s.done)
	}()
	return s, nil
}

// Read returns whatever the shell has written.
//
// A pty master reports EIO rather than EOF when the last slave closes, which is
// what happens the moment the shell exits. That is the normal end of a session,
// not a fault, so it is translated to io.EOF — otherwise every clean logout is
// logged as an error.
func (s *Session) Read(p []byte) (int, error) {
	n, err := s.f.Read(p)
	if err != nil && errors.Is(err, syscall.EIO) {
		return n, io.EOF
	}
	return n, err
}

// Write sends keystrokes to the shell.
func (s *Session) Write(p []byte) (int, error) { return s.f.Write(p) }

// Resize tells the pty its new grid, which is a TIOCSWINSZ on the master and
// therefore a SIGWINCH to the foreground process group. That signal is the only
// reason a full-screen program redraws when a window is dragged wider; without
// it the shell keeps wrapping at the old width no matter what the pane looks
// like.
func (s *Session) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return errors.New("hostagent: refusing a zero-sized window")
	}
	return pty.Setsize(s.f, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec // checked above
}

// Done is closed when the shell exits of its own accord, so a transport can
// notice a `exit` typed at the prompt without polling.
func (s *Session) Done() <-chan struct{} { return s.done }

// Close ends the session.
//
// Closing the master first is not enough on its own: it sends the child a
// SIGHUP, which a well-behaved shell honors and an ignoring one does not, so a
// process that trapped it would outlive the window that owned it. The kill is
// the guarantee. Waiting briefly in between gives the shell the chance to exit
// on the hangup and be reaped as itself.
func (s *Session) Close() error {
	err := s.f.Close()
	select {
	case <-s.done:
		return err
	case <-time.After(200 * time.Millisecond):
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill() //nolint:errcheck // it is already going away
	}
	<-s.done
	return err
}
