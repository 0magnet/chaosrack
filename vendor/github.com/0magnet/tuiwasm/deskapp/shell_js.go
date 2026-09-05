//go:build js && wasm

package deskapp

import (
	"github.com/0magnet/desk"
	"github.com/0magnet/desk/panes/term"
	xterm "github.com/0magnet/xterm-go"
)

// RegisterShell adds a websh shell to the desk.
//
// The demos are output: a text one writes and stops, and a tcell one paints
// and reads its own keys. Neither is a terminal you can type into, which is
// what a window that looks like a terminal implies — so there is one here
// that is.
//
// It is also the honest exhibit. Everything else in the launcher demonstrates
// a library drawing; this demonstrates the thing the terminal was built for,
// and it is the same websh that desk ships.
const shellGreeting = "" +
	"\x1b[1;36mwebsh\x1b[0m — a shell, in the same terminal the demos draw into\r\n" +
	"\x1b[2mthe demos in the launcher are output; this one takes input\x1b[0m\r\n\r\n" +
	"try: \x1b[1mls /\x1b[0m · \x1b[1mecho hi > note.txt && cat note.txt\x1b[0m · \x1b[1mhelp\x1b[0m\r\n" +
	"     \x1b[1mtoilet -f smblock hello | lolcat\x1b[0m\r\n\r\n"

// shellPane is the shell's pane, kept so a full-screen applet can borrow the
// terminal it is running in. An applet is handed pipes, which is right for
// anything that reads and writes text and useless for a tcell demo that has to
// paint cells and read keys.
var shellPane *term.Pane

// shellTerminal returns the terminal the shell is drawing on, if there is one.
// There is not, before the window has been opened.
func shellTerminal() (t struct {
	term *xterm.Terminal
	ok   bool
}) {
	if shellPane == nil {
		return t
	}
	sess := shellPane.Session()
	if sess == nil || sess.Term == nil {
		return t
	}
	t.term, t.ok = sess.Term, true
	return t
}

func RegisterShell() {
	// toilet and lolcat are ports that live in their own repositories; a
	// pipeline joining them belongs in a shell rather than in either of them.
	RegisterApplets()
	// The demos are commands too, so Ctrl+C has a prompt to come back to.
	RegisterDemoApplets()

	desk.Register(desk.App{
		Name:   "shell",
		Title:  "shell",
		Help:   "websh — bash in the browser, the terminal these demos draw into",
		Width:  760,
		Height: 460,
		Open: func([]string) (desk.Pane, error) {
			// Kept so a full-screen applet can borrow this terminal; see
			// shellTerminal. A second shell window replaces the reference,
			// which is right — the newest one is the one being typed into.
			shellPane = term.New(shellGreeting, "tuiwasm")
			return shellPane, nil
		},
	})
}
