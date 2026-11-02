//go:build unix

package subcmds

import (
	"os/exec"
	"syscall"
)

// ownGroup puts cmd in a process group of its own, so the terminal's Ctrl+C,
// which goes to the whole foreground group, reaches render and not ffmpeg:
// render ends a live recording on it and still needs ffmpeg to finish the
// video.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
