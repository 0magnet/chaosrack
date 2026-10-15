//go:build unix

package rackhost

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// startPty runs cmd on a new pty cols by rows, and is the pty's master.
func startPty(cmd *exec.Cmd, cols, rows int) (*os.File, error) {
	return pty.StartWithSize(cmd, winsize(cols, rows))
}

// setPtySize makes pty f cols by rows, which tells the program on it.
func setPtySize(f *os.File, cols, rows int) error {
	return pty.Setsize(f, winsize(cols, rows))
}

func winsize(cols, rows int) *pty.Winsize {
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)} //nolint:gosec // bounded by textMaxCols and textMaxRows
}
