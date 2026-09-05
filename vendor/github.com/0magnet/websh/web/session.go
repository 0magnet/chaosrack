//go:build js && wasm

// Package web mounts a websh shell onto a DOM element.
//
// websh's parts — the interpreter, the applets, the filesystem — are libraries,
// and assembling a working terminal out of them takes a couple of hundred lines
// of line editor, stdin plumbing and raw-mode switching. That assembly is the
// same every time, so it lives here rather than being copied into every program
// that wants a shell on a page.
//
// A Session is a widget: hand it an element and it fills it. Nothing here knows
// whether that element is the whole page or the body of a draggable window, and
// nothing needs to — the terminal observes its own container.
package web

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"syscall/js"

	"github.com/0magnet/afero"
	xterm "github.com/0magnet/xterm-go"
	"github.com/0magnet/xterm-go/vt"

	"github.com/0magnet/websh/shell"
)

// Options configure a Session. The zero value is usable.
type Options struct {
	// FS is the filesystem the shell runs over. Nil creates a fresh
	// in-memory one and seeds it. Passing an existing one is how several
	// sessions come to share files — and how a caller supplies a filesystem
	// restored from somewhere persistent.
	FS afero.Fs

	// Host is the name in the prompt. Empty means "websh".
	Host string

	// Greeting is written once, before the first prompt.
	Greeting string

	// Env holds extra "NAME=value" variables for the shell's environment,
	// overriding the defaults on a name clash. An embedder uses it to put a
	// toolchain on the PATH, say.
	Env []string

	// Scrollback is the number of lines kept. Zero means 2000.
	Scrollback int

	// FontFamily is the CSS font stack the terminal draws with. Empty keeps
	// xterm-go's default. A page that already has a face of its own wants
	// this: the terminal is measured from it at Open, so it cannot be
	// changed afterwards without re-measuring the cell.
	FontFamily string

	// FontSize is the cell size in CSS pixels. Zero keeps the default.
	FontSize float64

	// NoWebGL forces the DOM renderer.
	NoWebGL bool

	// AfterCommand runs after each command line finishes, on the shell's
	// goroutine. It is where a caller flushes the filesystem somewhere
	// durable, which has to happen after a command rather than during one.
	AfterCommand func()
}

// Session is a terminal with a shell attached, mounted on an element.
type Session struct {
	Term   *xterm.Terminal
	Shell  *shell.Shell
	Editor *shell.LineEditor

	host   string
	stdinW *io.PipeWriter
	lines  chan string

	running   bool
	rawInput  bool
	cancelRun context.CancelFunc
	closed    bool

	afterCommand func()
}

// NewSession builds a terminal on el and starts a shell on it.
func NewSession(el js.Value, opt Options) (*Session, error) {
	if el.IsNull() || el.IsUndefined() {
		return nil, fmt.Errorf("websh: cannot mount on a missing element")
	}
	fsys := opt.FS
	if fsys == nil {
		fsys = afero.NewMemMapFs()
		if err := shell.Seed(fsys); err != nil {
			return nil, fmt.Errorf("websh: seeding the filesystem: %w", err)
		}
	}
	host := opt.Host
	if host == "" {
		host = "websh"
	}
	scrollback := opt.Scrollback
	if scrollback == 0 {
		scrollback = 2000
	}

	s := &Session{
		host:         host,
		lines:        make(chan string, 8),
		afterCommand: opt.AfterCommand,
	}

	o := vt.NewOptions()
	o.Scrollback = scrollback
	if opt.FontFamily != "" {
		o.FontFamily = opt.FontFamily
	}
	if opt.FontSize > 0 {
		o.FontSize = opt.FontSize
	}
	s.Term = xterm.New(o)
	s.Term.Open(el)
	// Watch the container, not the window: mounted in anything smaller than
	// the page, the window never changes when the terminal's box does.
	s.Term.AutoFit()
	if !opt.NoWebGL {
		if err := s.Term.EnableWebGL(); err != nil {
			js.Global().Get("console").Call("log", "websh: webgl unavailable: "+err.Error())
		}
	}

	stdinR, stdinW := io.Pipe()
	s.stdinW = stdinW

	sh, err := shell.New(fsys, stdinR, termWriter{s.Term}, termWriter{s.Term}, opt.Env...)
	if err != nil {
		s.Term.Dispose()
		return nil, fmt.Errorf("websh: %w", err)
	}
	s.Shell = sh
	if err := sh.PopulateBin(); err != nil {
		s.Term.WriteString("failed to populate /bin: " + err.Error() + "\r\n")
	}

	s.Editor = &shell.LineEditor{
		Echo: func(str string) { s.Term.WriteString(str) },
		Redraw: func(content string, back int) {
			line := "\r\x1b[2K" + s.Prompt() + content
			if back > 0 {
				line += fmt.Sprintf("\x1b[%dD", back)
			}
			s.Term.WriteString(line)
		},
		Submit:    func(l string) { s.lines <- l },
		Interrupt: func() { s.Shell.CancelPending(); s.WritePrompt() },
		EOF:       func() { s.Term.WriteString("\r\n"); s.WritePrompt() },
		ClearScreen: func() {
			s.Term.WriteString("\x1b[2J\x1b[H")
			s.WritePrompt()
			s.Term.WriteString(s.Editor.Line())
		},
		Complete: s.complete,
	}
	if err := sh.UseHistory(s.Editor.History, s.Editor.ClearHistory); err != nil {
		s.Term.WriteString("history unavailable: " + err.Error() + "\r\n")
	}

	// Full-screen applets take raw bytes and need the size.
	sh.RawMode = func(on bool) { s.rawInput = on }
	sh.Size = func() (int, int) { return s.Term.Core.Cols(), s.Term.Core.Rows() }

	s.Term.Core.OnData = s.onData

	go s.run()

	if opt.Greeting != "" {
		s.Term.WriteString(opt.Greeting)
	}
	s.WritePrompt()
	return s, nil
}

// Submit runs a line as though it had been typed at the prompt: it is echoed
// where the typing would have appeared, remembered in the history, and run.
//
// It is what a link into a page needs — "open this and run that" — and the
// echo is the point rather than a side effect. A command that arrives from a
// URL should be visible in the scrollback, so that what ran is on the screen
// and not only in the address bar.
//
// The send is on its own goroutine because the line channel is small and the
// run loop may be busy; blocking here would block whatever called it, which on
// this platform is usually the browser's event loop.
func (s *Session) Submit(line string) {
	if s.closed || line == "" {
		return
	}
	s.Term.WriteString(line + "\r\n")
	go func() {
		// A send on a closed session panics; there is nobody left to tell.
		defer func() { _ = recover() }() //nolint:errcheck
		s.lines <- line
	}()
}

// Prompt is the current prompt string, colors and all.
func (s *Session) Prompt() string {
	if s.Shell.Pending() {
		return "\x1b[1;33m>\x1b[0m "
	}
	dir := s.Shell.Dir()
	if strings.HasPrefix(dir, "/home/user") {
		dir = "~" + dir[len("/home/user"):]
	}
	return "\x1b[1;32m" + s.host + "\x1b[0m:\x1b[1;34m" + dir + "\x1b[0m$ "
}

// WritePrompt draws the prompt.
func (s *Session) WritePrompt() { s.Term.WriteString(s.Prompt()) }

func (s *Session) onData(data string) {
	if s.running {
		if s.rawInput {
			// A full-screen applet owns the terminal: raw bytes, no echo,
			// and Ctrl+C is passed through for it to handle itself.
			go shell.Write(s.stdinW, []byte(data))
			return
		}
		if strings.Contains(data, "\x03") {
			if s.cancelRun != nil {
				s.cancelRun()
			}
			return
		}
		s.Term.WriteString(strings.ReplaceAll(data, "\r", "\r\n"))
		go shell.Write(s.stdinW, []byte(strings.ReplaceAll(data, "\r", "\n")))
		return
	}
	s.Editor.Input(data)
}

func (s *Session) complete(word string, isFirstWord bool) []string {
	if isFirstWord && !strings.Contains(word, "/") {
		var names []string
		for _, n := range append(shell.AppletNames(), Builtins...) {
			if strings.HasPrefix(n, word) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		return names
	}
	dir, base := filepath.Split(word)
	search := dir
	if !filepath.IsAbs(search) {
		search = filepath.Join(s.Shell.Dir(), dir)
	}
	infos, err := afero.ReadDir(s.Shell.FS, filepath.Clean(search))
	if err != nil {
		return nil
	}
	var names []string
	for _, info := range infos {
		if !strings.HasPrefix(info.Name(), base) {
			continue
		}
		cand := dir + info.Name()
		if info.IsDir() {
			cand += "/"
		}
		names = append(names, cand)
	}
	sort.Strings(names)
	return names
}

// run is the command loop. It is a goroutine so the JS event loop — and so the
// terminal — stays responsive while a command runs.
func (s *Session) run() {
	for line := range s.lines {
		if s.closed {
			return
		}
		if !s.Shell.Pending() {
			s.Editor.AddHistory(line)
		}

		// Canceling at the end of the line is safe: the interpreter detaches
		// background jobs from this context, so `sleep 30 &` survives to the
		// next prompt as it would in bash.
		ctx, cancel := context.WithCancel(context.Background())
		s.cancelRun, s.running = cancel, true

		_, err := s.Shell.Run(ctx, line)

		s.running, s.cancelRun = false, nil
		cancel()

		if err != nil {
			if msg := err.Error(); !strings.HasPrefix(msg, "exit status") {
				s.Term.WriteString(s.host + ": " + strings.ReplaceAll(msg, "\n", "\r\n") + "\r\n")
			}
		}
		if s.afterCommand != nil {
			s.afterCommand()
		}
		s.WritePrompt()
	}
}

// Close tears the session down. The filesystem outlives it, so files written
// here are still there for whatever opens next.
func (s *Session) Close() {
	if s.closed {
		return
	}
	s.closed = true
	if s.stdinW != nil {
		// The session is going away; a failed close has no reader to report to.
		s.stdinW.Close() //nolint:errcheck,gosec
	}
	close(s.lines)
	if s.Term != nil {
		s.Term.Dispose()
	}
}

type termWriter struct{ term *xterm.Terminal }

func (w termWriter) Write(p []byte) (int, error) {
	w.term.WriteString(strings.ReplaceAll(string(p), "\n", "\r\n"))
	return len(p), nil
}

// Builtins are the interpreter's own commands, which completion offers
// alongside the applets.
var Builtins = []string{
	"cd", "pwd", "echo", "printf", "read", "exit", "export", "unset",
	"source", "test", "true", "false", "set", "shift", "local",
	"declare", "eval", "alias", "unalias", "type", "return", "break",
	"continue", "pushd", "popd", "dirs", "let", "getopts", "wait",
	"jobs", "kill", "disown", "fg", "bg", "enable", "compgen", "history",
	"builtin", "umask", "times", "trap", "shopt", "mapfile", "readarray",
}
