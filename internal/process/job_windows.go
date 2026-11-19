//go:build windows

package process

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows Job Objects (SPEC-process.md, Task 31): every browser goes into a
// job with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so TerminateProcess on omnisnap
// — which runs no handler — still takes the whole browser tree down.
//
// The API comes from golang.org/x/sys/windows, the one dependency the
// conventions allow without asking for this exact job (CONVENTIONS,
// "Dependencies"). Go's syscall package does not expose Job Objects.

const (
	processSetQuota  = windows.PROCESS_SET_QUOTA
	processTerminate = windows.PROCESS_TERMINATE
)

// createJob makes a job that kills its processes when the last handle closes:
// the handle lives in the Handle, and Windows closes it when omnisnap dies.
func createJob(pid int) (uintptr, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("process: CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("process: SetInformationJobObject: %w", err)
	}
	proc, err := windows.OpenProcess(processSetQuota|processTerminate, false, uint32(pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("process: OpenProcess %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("process: AssignProcessToJobObject: %w", err)
	}
	return uintptr(job), nil
}

// closeJob kills every process in the job when the last handle closes.
func closeJob(job uintptr) {
	if job != 0 {
		_ = windows.CloseHandle(windows.Handle(job))
	}
}
