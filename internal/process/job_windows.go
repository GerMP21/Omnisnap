//go:build windows

package process

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Windows Job Objects (SPEC-process.md, Task 31): every browser goes into a
// job with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so TerminateProcess on omnisnap
// — which runs no handler — still takes the whole browser tree down.
//
// The API is called through kernel32 directly: Go's syscall package does not
// expose Job Objects, and one struct plus one flag does not justify a new
// module dependency (CONVENTIONS: dependencies need approval).

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObject       = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJob     = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob    = kernel32.NewProc("AssignProcessToJobObject")
	jobObjectExtendedInfo     = 9
	jobObjectLimitKillOnClose = 0x2000
)

type jobBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type jobExtendedLimitInformation struct {
	BasicLimitInformation jobBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// createJob makes a job that kills its processes when the last handle closes:
// the handle lives in the Handle, and Windows closes it when omnisnap dies.
func createJob(pid int) (uintptr, error) {
	job, _, err := procCreateJobObject.Call(0, 0)
	if job == 0 {
		return 0, fmt.Errorf("process: CreateJobObject: %w", err)
	}
	info := jobExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = uint32(jobObjectLimitKillOnClose)
	ok, _, serr := procSetInformationJob.Call(job, uintptr(jobObjectExtendedInfo),
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if ok == 0 {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return 0, fmt.Errorf("process: SetInformationJobObject: %w", serr)
	}
	const (
		processSetQuota  = 0x0100
		processTerminate = 0x0001
	)
	proc, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(pid))
	if err != nil {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return 0, fmt.Errorf("process: OpenProcess %d: %w", pid, err)
	}
	defer func() { _ = syscall.CloseHandle(proc) }()
	if ok, _, serr := procAssignProcessToJob.Call(job, uintptr(proc)); ok == 0 {
		_ = syscall.CloseHandle(syscall.Handle(job))
		return 0, fmt.Errorf("process: AssignProcessToJobObject: %w", serr)
	}
	return job, nil
}

// closeJob kills every process in the job when the last handle closes.
func closeJob(job uintptr) {
	if job != 0 {
		_ = syscall.CloseHandle(syscall.Handle(job))
	}
}
