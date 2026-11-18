//go:build unix

package groundtruth

import (
	"fmt"
	"syscall"
)

// FreeBytes is the space available to unprivileged users under dir.
func FreeBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("groundtruth: disk space: %w", err)
	}
	return st.Bavail * uint64(st.Bsize), nil
}
