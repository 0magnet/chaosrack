//go:build js && wasm

package deskapp

import (
	"github.com/0magnet/desk"
	"github.com/0magnet/desk/panes/term"
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
	"try: \x1b[1mls /\x1b[0m · \x1b[1mecho hi > note.txt && cat note.txt\x1b[0m · \x1b[1mhelp\x1b[0m\r\n\r\n"

func RegisterShell() {
	desk.Register(desk.App{
		Name:   "shell",
		Title:  "shell",
		Help:   "websh — bash in the browser, the terminal these demos draw into",
		Width:  760,
		Height: 460,
		Open: func([]string) (desk.Pane, error) {
			return term.New(shellGreeting, "tuiwasm"), nil
		},
	})
}
