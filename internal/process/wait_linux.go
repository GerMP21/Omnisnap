//go:build linux

package process

import (
	"syscall"
	"unsafe"
)

const pPID = 1 // idtype_t P_PID

// waitExitedNoReap blocks until pid exits, leaving it unreaped (WNOWAIT): a
// zombie keeps its pid, and with it the process group id, out of reuse.
func waitExitedNoReap(pid int) error {
	var info [128]byte // siginfo_t; only the call's success matters
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(pid),
			uintptr(unsafe.Pointer(&info[0])), syscall.WEXITED|syscall.WNOWAIT, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
