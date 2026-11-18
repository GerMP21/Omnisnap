//go:build windows

package groundtruth

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procDiskFreeEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// FreeBytes is the space available to unprivileged users under dir.
func FreeBytes(dir string) (uint64, error) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, avail uint64
	ok, _, e := procDiskFreeEx.Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if ok == 0 {
		return 0, fmt.Errorf("groundtruth: disk space: %w", e)
	}
	return avail, nil
}
