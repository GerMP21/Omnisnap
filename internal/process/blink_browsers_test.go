//go:build browsers && unix

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
)

func blinkExecutable(t *testing.T) string {
	t.Helper()
	found, err := discovery.Find(context.Background(), discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range found {
		if b.Family == "blink" {
			return b.Executable
		}
	}
	t.Skip("no Blink browser installed")
	return ""
}

// 20 launch/close cycles leave no process and no temp dir behind (Task 12).
func TestBlinkLaunchCloseCycles(t *testing.T) {
	exe := blinkExecutable(t)
	root, err := os.MkdirTemp("", "omnisnap-process-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	for i := range 20 {
		b, err := StartBlink(ctx, exe, root)
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		// The endpoint works: open a page and evaluate.
		br, err := cdp.Dial(ctx, b.WSURL)
		if err != nil {
			t.Fatalf("cycle %d: dial: %v", i, err)
		}
		s, err := br.NewSession(ctx)
		if err != nil {
			t.Fatalf("cycle %d: session: %v", i, err)
		}
		if got, err := s.Evaluate(ctx, "1 + 1"); err != nil || got != "2" {
			t.Fatalf("cycle %d: evaluate = %q, %v", i, got, err)
		}
		_ = br.Close()
		if err := b.Close(ctx); err != nil {
			t.Fatalf("cycle %d: close: %v", i, err)
		}
	}

	if dirs, _ := filepath.Glob(filepath.Join(root, "omnisnap-*")); len(dirs) != 0 {
		t.Errorf("%d temp dirs left: %v", len(dirs), dirs)
	}
	time.Sleep(200 * time.Millisecond) // let init reap the killed group
	out, err := exec.Command("ps", "-eo", "pid,stat,args").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, root) && !strings.Contains(line, " Z") {
			t.Errorf("process left: %s", line)
		}
	}
}

func TestBlinkStartFailsCleanly(t *testing.T) {
	root := t.TempDir()
	// A program that exits at once instead of serving DevTools.
	_, err := StartBlink(context.Background(), "/bin/false", root)
	if err == nil || !strings.Contains(err.Error(), "exited before DevTools") {
		t.Fatalf("err = %v", err)
	}
	if dirs, _ := filepath.Glob(filepath.Join(root, "omnisnap-*")); len(dirs) != 0 {
		t.Errorf("failed start left %v", dirs)
	}
}
