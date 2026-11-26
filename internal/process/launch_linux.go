//go:build linux

package process

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// oomScoreMax is the highest oom_score_adj: under memory pressure the kernel
// kills a process at this value before any other, and among several the
// largest. Every process Omnisnap launches carries it, so the kernel picks
// one of Omnisnap's browsers rather than the user's own (change
// cap-live-browsers; SPEC-process.md, "Resources").
// Source: https://www.kernel.org/doc/html/latest/filesystems/proc.html
// ("/proc/<pid>/oom_adj & /proc/<pid>/oom_score_adj")
const oomScoreMax = 1000

// afterStart sets the started process's out-of-memory score. Its children
// inherit it; ApplyOOMScore covers any forked before this write. A failure
// (the process already exited, a hardened kernel) is ignored here: the
// score is a safety net, and the next ApplyOOMScore reports it.
// The process group was set before Start (setProcessGroup).
func (h *Handle) afterStart() { _ = setOOMScore(h.cmd.Process.Pid) }

// killTree kills the browser's process group, even after the parent exited:
// children can outlive it.
func (h *Handle) killTree() { killGroup(h.cmd) }

// ApplyOOMScore sets the highest out-of-memory score on the handle's process
// and every descendant that reads less: browsers fork renderers and content
// processes, and Chrome lowers its renderers' scores itself. Descendants are
// found by parent pid, not by process group, because a sandbox may move a
// process out of the group. It never writes to a process outside the tree,
// nor to Omnisnap's own.
func (h *Handle) ApplyOOMScore() error { return applyOOMScore([]int{h.PID()}) }

// ApplyOOMScore re-applies the score to the tree of every live handle, with
// one scan of /proc. The CLI calls it each time it opens a session.
func ApplyOOMScore() error {
	var leaders []int
	liveHandles.Range(func(k, _ any) bool {
		leaders = append(leaders, k.(*Handle).PID())
		return true
	})
	if len(leaders) == 0 {
		return nil
	}
	return applyOOMScore(leaders)
}

func applyOOMScore(leaders []int) error {
	kids, err := childrenByParent()
	if err != nil {
		return err
	}
	self := os.Getpid()
	var errs []error
	seen := map[int]bool{}
	queue := append([]int(nil), leaders...)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] || pid == self {
			continue
		}
		seen[pid] = true
		if err := setOOMScore(pid); err != nil {
			errs = append(errs, err)
		}
		queue = append(queue, kids[pid]...)
	}
	return errors.Join(errs...)
}

// setOOMScore writes the highest score where the process reads less. A
// process that has exited is not an error.
func setOOMScore(pid int) error {
	path := "/proc/" + strconv.Itoa(pid) + "/oom_score_adj"
	cur, err := os.ReadFile(path)
	if err != nil {
		if gone(err) {
			return nil
		}
		return fmt.Errorf("process: oom score of %d: %w", pid, err)
	}
	if v, err := strconv.Atoi(strings.TrimSpace(string(cur))); err == nil && v >= oomScoreMax {
		return nil
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(oomScoreMax)), 0); err != nil && !gone(err) {
		return fmt.Errorf("process: oom score of %d: %w", pid, err)
	}
	return nil
}

func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH)
}

// childrenByParent maps each pid to its children's pids, from the PPid field
// of /proc/<pid>/stat.
func childrenByParent() (map[int][]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("process: %w", err)
	}
	kids := map[int][]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // exited meanwhile
		}
		if ppid, ok := parentOf(stat); ok {
			kids[ppid] = append(kids[ppid], pid)
		}
	}
	return kids, nil
}

// parentOf reads the PPid from a /proc/<pid>/stat line: "pid (comm) state
// ppid ...", where comm may itself hold spaces and parentheses, so the
// fields are counted after the last ')'.
func parentOf(stat []byte) (int, bool) {
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(stat[i+1:]))
	if len(f) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(f[1])
	return ppid, err == nil
}
