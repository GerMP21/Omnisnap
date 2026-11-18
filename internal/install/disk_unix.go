//go:build unix

package install

import (
	"fmt"
	"syscall"
)

// freeBytes is the space available to unprivileged users under dir.
func freeBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("install: disk space: %w", err)
	}
	return st.Bavail * uint64(st.Bsize), nil
}
