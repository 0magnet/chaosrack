package main

import (
	"os/exec"
	"syscall"
)

// ownGroup puts the child in a process group of its own, so the browser's
// helper processes go with it, and has the kernel kill it if this process
// dies without running atExit — killed by a timeout, say, or by ^C.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL, Setpgid: true}
}

func killGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // gone already is fine
}
