//go:build windows

package process

import "os/exec"

// Windows kills the tree through a Job Object (Task 30); until then, the
// browser process only.
func setProcessGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
