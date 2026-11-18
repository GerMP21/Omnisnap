//go:build unix

package process

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// setProcessGroup makes the browser lead its own process group, so the whole
// tree (renderers, zygote, utilities) can be signalled at once.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(cmd *exec.Cmd) { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// killGroupPID kills a process group left behind by a dead owner. The group
// can outlive its leader, which is why the sweeper records the browser's pid.
func killGroupPID(pid int) { _ = syscall.Kill(-pid, syscall.SIGKILL) }

// pidAlive reports whether pid still runs. A zombie does not count: it holds
// no resources and its parent (init, for a killed owner) reaps it.
func pidAlive(pid int) bool {
	// Linux exposes the state; elsewhere kill(0) has to do.
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		s := string(b)
		if i := strings.LastIndexByte(s, ')'); i >= 0 && i+1 < len(s) {
			fields := strings.Fields(s[i+1:])
			return len(fields) > 0 && fields[0] != "Z"
		}
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
