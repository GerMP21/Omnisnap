//go:build linux

package process

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Change cap-live-browsers: every process Omnisnap launches carries the
// highest out-of-memory score; the test process and processes Omnisnap did
// not start keep theirs.

func oomScore(t *testing.T, pid int) int {
	t.Helper()
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/oom_score_adj")
	if err != nil {
		t.Fatal(err)
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLaunchSetsOOMScore(t *testing.T) {
	before := oomScore(t, os.Getpid())
	h, err := spawnEnv(context.Background(), t.TempDir(), "sleep", []string{"30"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close(context.Background()) }()
	if got := oomScore(t, h.PID()); got != oomScoreMax {
		t.Errorf("launched process: oom_score_adj %d; want %d", got, oomScoreMax)
	}
	if got := oomScore(t, os.Getpid()); got != before {
		t.Errorf("the test process: oom_score_adj %d; want its original %d", got, before)
	}
}

// descendants lists the pids below pid.
func descendants(t *testing.T, pid int) []int {
	t.Helper()
	kids, err := childrenByParent()
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	queue := kids[pid]
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		out = append(out, p)
		queue = append(queue, kids[p]...)
	}
	return out
}

func TestApplyOOMScoreReachesTheTree(t *testing.T) {
	orig := oomScore(t, os.Getpid())
	// A grandchild that lowers its own score back to the original, as
	// Chrome's zygote does for its renderers.
	script := "sh -c 'echo " + strconv.Itoa(orig) + " > /proc/self/oom_score_adj; exec sleep 30' & wait"
	h, err := spawnEnv(context.Background(), t.TempDir(), "sh", []string{"-c", script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close(context.Background()) }()
	// A process the test starts itself, outside spawnEnv.
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	otherBefore := oomScore(t, other.Process.Pid)

	var lowered int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && lowered == 0; time.Sleep(50 * time.Millisecond) {
		for _, p := range descendants(t, h.PID()) {
			if b, err := os.ReadFile("/proc/" + strconv.Itoa(p) + "/comm"); err == nil && strings.TrimSpace(string(b)) == "sleep" {
				if oomScore(t, p) == orig {
					lowered = p
				}
			}
		}
	}
	if lowered == 0 {
		t.Fatal("the grandchild never lowered its score")
	}
	if err := ApplyOOMScore(); err != nil {
		t.Fatal(err)
	}
	if got := oomScore(t, lowered); got != oomScoreMax {
		t.Errorf("grandchild after ApplyOOMScore: %d; want %d", got, oomScoreMax)
	}
	if got := oomScore(t, other.Process.Pid); got != otherBefore {
		t.Errorf("a process Omnisnap did not start: %d; want %d", got, otherBefore)
	}
	if got := oomScore(t, os.Getpid()); got != orig {
		t.Errorf("the test process: %d; want %d", got, orig)
	}
}

func TestParentOf(t *testing.T) {
	for in, want := range map[string]int{
		"123 (sleep) S 45 123 123 0":           45,
		"9 (Web Content (x)) R 7 9 9 0 -1 ...": 7,
		"garbage":                              0,
	} {
		got, ok := parentOf([]byte(in))
		if got != want || ok != (want != 0) {
			t.Errorf("parentOf(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}

func TestClosedHandleIsNotReapplied(t *testing.T) {
	h, err := spawnEnv(context.Background(), t.TempDir(), "sleep", []string{"30"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := liveHandles.Load(h); !ok {
		t.Fatal("a started handle is not live")
	}
	_ = h.Close(context.Background())
	if _, ok := liveHandles.Load(h); ok {
		t.Error("a closed handle is still live")
	}
}
