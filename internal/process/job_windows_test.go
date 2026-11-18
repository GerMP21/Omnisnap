//go:build windows

package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TerminateProcess on the browser must take its children with it: the Job
// Object closes and kills the tree (SPEC-process.md, Task 31). PowerShell
// spawns a long-lived cmd child and records its pid, so the test can watch the
// child after the parent is killed, without needing a browser.
func TestJobObjectKillsTheProcessTree(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	script := fmt.Sprintf(
		`$c = Start-Process -FilePath cmd.exe -ArgumentList '/c','ping','-n','120','127.0.0.1' -PassThru -NoNewWindow; `+
			`Set-Content -Path '%s' -Value $c.Id -Encoding ascii; Start-Sleep -Seconds 120`, pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	h, err := spawn(ctx, "", "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", script}, nil)
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	pid := childPID(t, pidFile)
	if !pidAlive(pid) {
		t.Fatalf("child %d is not running", pid)
	}
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelClose()
	if err := h.Close(closeCtx); err != nil {
		t.Fatalf("close: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for pidAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d survived TerminateProcess; the Job Object did not kill the tree", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// childPID waits for the pid PowerShell wrote, and fails if the child never
// started.
func childPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no child pid in %s: PowerShell did not start the child", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
