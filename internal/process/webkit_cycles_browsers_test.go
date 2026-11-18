//go:build linux && browsers

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Task 21: 20 launch/close cycles leave no Weston, driver or MiniBrowser
// behind, and no window appears (the display is private).
func TestWebKitCyclesLeaveNoProcesses(t *testing.T) {
	driver, browser := webkitBinaries(t)
	if _, err := exec.LookPath("weston"); err != nil {
		t.Skip("weston not installed: the display would be the user's desktop")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	base := webkitProcs(t)
	for i := range 20 {
		w, err := StartWebKit(ctx, WebKitOptions{Driver: driver, Browser: browser})
		if err != nil {
			t.Fatalf("cycle %d: start: %v", i, err)
		}
		if w.Visible() {
			t.Errorf("cycle %d: the browser is visible (not on a private display)", i)
		}
		if err := w.Close(context.WithoutCancel(ctx)); err != nil {
			t.Errorf("cycle %d: close: %v", i, err)
		}
	}
	// Close returns after the handle is reaped, but Weston's children can take
	// a moment to disappear from /proc.
	deadline := time.Now().Add(5 * time.Second)
	var left []string
	for time.Now().Before(deadline) {
		if left = webkitProcs(t); len(left) <= len(base) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("%d WebKit process(es) survived 20 cycles: %v", len(left)-len(base), left)
}

// webkitProcs lists the WebKit-related command lines (driver, browser, weston).
func webkitProcs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(b), "\x00", " ")
		for _, needle := range []string{"WebKitWebDriver", "MiniBrowser", "weston"} {
			if strings.Contains(cmd, needle) {
				out = append(out, strings.TrimSpace(cmd))
				break
			}
		}
	}
	return out
}
