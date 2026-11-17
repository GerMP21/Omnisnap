//go:build unix

package process

import (
	"os/exec"
	"syscall"
)

// setProcessGroup makes the browser lead its own process group, so the whole
// tree (renderers, zygote, utilities) can be signalled at once.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
