//go:build !unix && !js

package subcmds

import "os/exec"

// ownGroup does nothing where there are no process groups.
func ownGroup(*exec.Cmd) {}
