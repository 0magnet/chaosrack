//go:build !unix

package rackhost

import (
	"errors"
	"os"
	"os/exec"
)

// errNoPty is why there is no shell where there are no ptys to run one on.
var errNoPty = errors.New("no pty on this system")

func startPty(*exec.Cmd, int, int) (*os.File, error) { return nil, errNoPty }

func setPtySize(*os.File, int, int) error { return errNoPty }
