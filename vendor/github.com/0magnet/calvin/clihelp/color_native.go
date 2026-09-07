//go:build !js

package clihelp

import (
	"os"

	"github.com/mattn/go-isatty"
)

// Color decides whether the help is styled.
//
// It is a variable rather than a guess made at each call because only the host
// knows: `pisano --help > notes.txt` wants no escapes, and a browser's terminal
// wants them even though the help is written down a pipe that cannot be asked
// whether anything is on the other end.
var Color = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
