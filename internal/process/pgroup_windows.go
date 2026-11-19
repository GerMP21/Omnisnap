//go:build windows

package process

import (
	"os/exec"
	"syscall"
)

// setProcessGroup gives the browser its own console process group (Ctrl-C in a
// terminal then reaches omnisnap only); the tree is killed through the Job
// Object (job_windows.go).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killGroupPID: process groups are Job Objects on Windows; when the owner dies
// the Job closes and kills the tree, so there is nothing left to kill here.
func killGroupPID(int) {}

// pidAlive asks for the process's exit code: STILL_ACTIVE means it runs.
func pidAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	const stillActive = 259
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
