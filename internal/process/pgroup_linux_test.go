//go:build linux

package process

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Review finding 7: a browser that exits on its own stays unreaped (a zombie
// holding its pid, and so its process group id) until Close. Close's group
// kill can then never hit a recycled group id.
func TestExitedBrowserIsReapedOnlyByClose(t *testing.T) {
	h, err := spawn(context.Background(), t.TempDir(), "/bin/true", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pid := h.PID()
	select {
	case <-h.exit:
	case <-time.After(5 * time.Second):
		t.Fatal("exit not observed")
	}
	if s := procState(pid); s != "Z" {
		t.Errorf("after exit, before Close: state %q; want Z (exited, pid still held)", s)
	}
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := procState(pid); s != "" && s != "Z" {
		// A recycled pid would be a different, running process: only check it is not ours.
		t.Logf("pid %d reused after Close (state %q)", pid, s)
	} else if s == "Z" {
		t.Errorf("still a zombie after Close: not reaped")
	}
}

// procState is the state letter of /proc/<pid>/stat, "" when it is gone.
func procState(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b[strings.LastIndexByte(string(b), ')')+1:]))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
