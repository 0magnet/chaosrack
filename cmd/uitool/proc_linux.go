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

// yieldGroup drops the child's group to the lowest CPU priority. A headless
// browser composites the whole panel in software, which is two cores for as
// long as a sweep runs; niced, it only takes what the desktop is not using.
// Processes the browser starts later inherit it.
func yieldGroup(cmd *exec.Cmd) {
	_ = syscall.Setpriority(syscall.PRIO_PGRP, cmd.Process.Pid, 19) //nolint:errcheck // a harness at normal priority still works
}

func killGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // gone already is fine
}
