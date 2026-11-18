package process

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The third cleanup layer (SPEC-process.md): a start-up sweeper deletes
// omnisnap-* temp dirs whose owner.lock names a process that is gone, and kills
// any process group the lock recorded. This is what recovers from SIGKILL,
// which runs no handler at all.

// Sweep removes leftovers under root. alive decides whether an owner pid is
// still running (injected in tests). Foreign directories are never touched,
// and neither are the current process's own dirs.
func Sweep(root string, alive func(pid int) bool) (removed []string, err error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("process: sweep %s: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "omnisnap-") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		pid, ok := readOwner(dir)
		if ok && (pid == os.Getpid() || alive(pid)) {
			continue
		}
		if ok && pid > 0 {
			// The owner is gone; its browser may still be running in the
			// group it leads. A group id can outlive its leader.
			if group, ok := readGroup(dir); ok {
				killGroupPID(group)
			}
		}
		if err := removeDir(dir); err != nil {
			return removed, err
		}
		removed = append(removed, dir)
	}
	return removed, nil
}

// readOwner reads <dir>/owner.lock.
func readOwner(dir string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(dir, ownerLock))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// readGroup reads <dir>/group.lock, the browser's process group id.
func readGroup(dir string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(dir, groupLock))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// removeDir retries briefly: a dying browser may still hold files.
func removeDir(dir string) error {
	var err error
	for range 20 {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("process: sweep remove %s: %w", dir, err)
}

// SweepLeftovers sweeps the default roots with the real process check.
func SweepLeftovers() []string { return SweepDefault(pidAlive) }

// SweepDefault sweeps the places omnisnap creates temp dirs: the OS temp dir,
// and the snap Firefox profile root when it exists. Called at start-up of a
// capture command; silent unless something is removed.
func SweepDefault(alive func(pid int) bool) []string {
	var removed []string
	for _, root := range []string{os.TempDir(), snapCommonRoot()} {
		if root == "" {
			continue
		}
		if got, err := Sweep(root, alive); err == nil {
			removed = append(removed, got...)
		}
	}
	return removed
}

// snapCommonRoot is ~/snap/firefox/common when it exists.
func snapCommonRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, "snap", "firefox", "common")
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}
