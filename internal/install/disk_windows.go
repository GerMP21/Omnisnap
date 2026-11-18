//go:build windows

package install

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procDiskFreeEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// freeBytes is the space available to unprivileged users under dir.
func freeBytes(dir string) (uint64, error) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, avail uint64
	ok, _, e := procDiskFreeEx.Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if ok == 0 {
		return 0, fmt.Errorf("install: disk space: %w", e)
	}
	return avail, nil
}
