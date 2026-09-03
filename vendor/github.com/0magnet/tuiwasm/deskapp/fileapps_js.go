//go:build js && wasm

package deskapp

import (
	"github.com/0magnet/desk"
	"github.com/0magnet/desk/panes/files"
	"github.com/0magnet/desk/panes/term"
	"github.com/0magnet/desk/panes/viewer"
)

// RegisterFileApps adds the file manager and the image viewer.
//
// They are the other half of having a shell. The terminal is better at doing
// things to files and a list is better at finding out what is there, and both
// work on the same filesystem — so a file written by a command in the shell
// window appears in the file manager without either knowing about the other.
//
// That sharing is the whole point, and it is why they take term.FS() rather
// than each making a filesystem of its own. term.FS() is the same handle websh
// writes through, so all three windows see one tree.
func RegisterFileApps() {
	desk.Register(desk.App{
		Name:   "files",
		Title:  "files",
		Help:   "a file manager over the same filesystem the shell writes to",
		Width:  620,
		Height: 420,
		Open: func(args []string) (desk.Pane, error) {
			dir := "/"
			if len(args) > 0 && args[0] != "" {
				dir = args[0]
			}
			return files.New(term.FS(), dir), nil
		},
	})

	// The viewer registers itself, because it also installs a `view` command
	// in the shell — opening a window from a command line is the thing it is
	// for, and only the viewer knows how to spell that.
	viewer.Register(term.FS())
}
