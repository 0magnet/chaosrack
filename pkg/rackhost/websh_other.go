//go:build !linux

package rackhost

import (
	"errors"
	"os"
)

// webshShell has websh's console to run only where it has been tried, on
// linux.
func webshShell(int, int) (*os.File, chan<- os.Signal, func(), error) {
	return nil, nil, nil, errors.New("websh in the rack runs on linux")
}

func signalFor(*os.File, []byte, chan<- os.Signal) {}
