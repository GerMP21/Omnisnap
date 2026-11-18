//go:build unix

package process

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// dead reports no process alive: every lock looks like a killed run.
func dead(int) bool { return false }

// Sweep ignores foreign dirs and live owners, and removes dirs whose owner is
// gone (the SIGKILL case: no handler ran).
func TestSweepDecisions(t *testing.T) {
	root := t.TempDir()
	mk := func(name, lock string) string {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if lock != "" {
			if err := os.WriteFile(filepath.Join(dir, ownerLock), []byte(lock), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	stale := mk("omnisnap-999999-1", "999999\n")
	live := mk("omnisnap-424242-1", "424242\n")
	foreign := mk("something-else", "999999\n")
	unlocked := mk("omnisnap-999999-2", "")

	removed, err := Sweep(root, func(pid int) bool { return pid == 424242 })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(removed, ","); got != stale+","+unlocked {
		t.Errorf("removed %v; want the stale and unlocked dirs", removed)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("live owner's dir removed: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("foreign dir removed: %v", err)
	}
}

// Kill and resume (Task 29): a browser left running by a SIGKILLed owner is
// killed by the next start's sweep, and its dir deleted within 2 s.
func TestSweepKillsTheLeftoverGroup(t *testing.T) {
	root := t.TempDir()
	h, err := spawn(context.Background(), root, "/bin/sh", []string{"-c", "sleep 60"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := h.Dir()
	browser := h.PID()
	// Simulate the owner dying: its pid is gone, the browser is not.
	if err := os.WriteFile(filepath.Join(dir, ownerLock), []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	removed, err := Sweep(root, pidAlive)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != dir {
		t.Errorf("removed %v; want %s", removed, dir)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("sweep took %v; want < 2 s", d)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("dir %s survived the sweep", dir)
	}
	deadline := time.Now().Add(2 * time.Second)
	for pidAlive(browser) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if pidAlive(browser) {
		t.Errorf("leftover browser %d survived the sweep", browser)
	}
	// Reap the shell so the test leaves no zombie.
	_ = h.cmd.Wait()
}

// Sweeping a root that does not exist is not an error (a fresh machine).
func TestSweepMissingRoot(t *testing.T) {
	if removed, err := Sweep(filepath.Join(t.TempDir(), "nope"), dead); err != nil || len(removed) != 0 {
		t.Errorf("Sweep(missing) = %v, %v", removed, err)
	}
}

// The group lock records the browser's pid: owner.lock alone cannot name the
// group the browser leads.
func TestSpawnWritesGroupLock(t *testing.T) {
	h, err := spawn(context.Background(), t.TempDir(), "/bin/sh", []string{"-c", "sleep 60"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close(context.Background()) }()
	b, err := os.ReadFile(filepath.Join(h.Dir(), groupLock))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != strconv.Itoa(h.PID()) {
		t.Errorf("group.lock = %q; want %d", b, h.PID())
	}
}

// Twenty runs killed with SIGKILL: the next start leaves 0 processes and 0
// dirs within 2 s (Task 29, success criterion 2).
func TestTwentyKilledRunsAreSwept(t *testing.T) {
	root := t.TempDir()
	var dirs []string
	var pids []int
	for range 20 {
		h, err := spawn(context.Background(), root, "/bin/sh", []string{"-c", "sleep 60"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, h.Dir())
		pids = append(pids, h.PID())
		if err := os.WriteFile(filepath.Join(h.Dir(), ownerLock), []byte("999999\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	removed, err := Sweep(root, pidAlive)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 20 {
		t.Errorf("swept %d dirs; want 20", len(removed))
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, pid := range pids {
		for pidAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if pidAlive(pid) {
			t.Errorf("browser %d survived", pid)
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("sweep took %v; want < 2 s", d)
	}
	left, _ := os.ReadDir(root)
	if len(left) != 0 {
		t.Errorf("%d entries left in the temp root", len(left))
	}
}
