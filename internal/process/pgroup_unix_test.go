//go:build unix

package process

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Close kills the whole process group, not just the parent: Chrome's
// renderers, zygote and utilities are children of the launched process.
func TestCloseKillsTheProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidFile := root + "/child.pid"
	h, err := spawn(context.Background(), root, "/bin/sh", []string{"-c", "sleep 60 & echo $! > " + pidFile + "; wait"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var child int
	for range 100 {
		if b, err := os.ReadFile(pidFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if child == 0 {
		t.Fatal("child did not start")
	}
	dir := h.Dir()
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(context.Background()); err != nil {
		t.Errorf("second Close: %v (Close must be idempotent)", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for alive(child) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if alive(child) {
		t.Errorf("grandchild %d survived Close", child)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir %s survived Close", dir)
	}
}

// alive: the process exists and is not a zombie waiting to be reaped.
func alive(pid int) bool {
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		fields := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+1:]))
		return len(fields) > 0 && fields[0] != "Z"
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
