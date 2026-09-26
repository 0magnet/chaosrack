//go:build !linux

package main

import "os/exec"

// Elsewhere the child is only stopped on a normal exit: no process group of
// its own and no death signal, which are Linux's.
func ownGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	_ = cmd.Process.Kill() //nolint:errcheck // gone already is fine
}
