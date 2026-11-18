package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ownerLock holds the pid of the omnisnap process that owns a temp dir, so the
// sweeper (Task 29) can delete dirs whose owner is gone.
const (
	ownerLock = "owner.lock"
	groupLock = "group.lock" // the launched browser's pid, which is its process group id
)

// tempDir is one launch's temporary directory: <root>/omnisnap-<pid>-<n>/
// with owner.lock and the browser profile. Never the user's profile: Chrome
// 136+ also refuses remote debugging on its default data dir.
type tempDir struct{ dir string }

func newTempDir(root string) (tempDir, error) {
	if root == "" {
		root = os.TempDir()
	}
	dir, err := os.MkdirTemp(root, fmt.Sprintf("omnisnap-%d-", os.Getpid()))
	if err != nil {
		return tempDir{}, fmt.Errorf("process: temp dir: %w", err)
	}
	d := tempDir{dir: dir}
	if err := os.WriteFile(filepath.Join(dir, ownerLock), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		_ = d.remove()
		return tempDir{}, fmt.Errorf("process: owner lock: %w", err)
	}
	if err := os.Mkdir(d.profile(), 0o700); err != nil {
		_ = d.remove()
		return tempDir{}, fmt.Errorf("process: profile dir: %w", err)
	}
	return d, nil
}

func (d tempDir) profile() string { return filepath.Join(d.dir, "profile") }

// remove deletes the dir. Files a dying browser still holds can make the first
// attempts fail, so it retries briefly.
func (d tempDir) remove() error {
	var err error
	for range 20 {
		if err = os.RemoveAll(d.dir); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("process: remove %s: %w", d.dir, err)
}
