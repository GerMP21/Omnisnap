//go:build windows && browsers

package process

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Task 31: TerminateProcess on omnisnap itself — no handler runs, nothing is
// closed in order — must still take every browser down. omnisnap holds one
// kill-on-close Job Object per browser (job_windows.go); the job's last
// handle goes with omnisnap's process, and "closing the last job object
// handle terminates all associated processes", child processes included
// ("by default any child processes it creates using CreateProcess are also
// associated with the job").
// Source: https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects
// and JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE in
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_limit_information
//
// The test runs the real binary against a page that never answers, so the
// job is mid-capture with Chrome, Edge and Firefox (the windows-latest
// image's browsers) and their renderers running, kills omnisnap, and waits on
// every process it had started.
func TestTerminatingOmnisnapKillsEveryBrowser(t *testing.T) {
	bin := buildOmnisnap(t)
	hang := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // the page never loads: the job stays mid-capture
		case <-hang:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(hang)

	logPath := filepath.Join(t.TempDir(), "omnisnap.log")
	logf, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logf.Close() }()
	cmd := exec.Command(bin, srv.URL+"/hang", "--engines", "chrome,edge,firefox",
		"--out", t.TempDir(), "--allow-small-pages", "--no-audit")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	omnisnap := uint32(cmd.Process.Pid)
	defer func() { _ = cmd.Process.Kill() }()

	// Every engine launches before the first page (cli/run.go), so all three
	// browsers are up while the baseline waits on the page.
	want := []string{"chrome.exe", "msedge.exe", "firefox.exe"}
	var tree []proc
	deadline := time.Now().Add(90 * time.Second)
	for {
		tree = descendants(t, omnisnap)
		if missing := missingNames(tree, want); len(missing) == 0 {
			break
		} else if time.Now().After(deadline) || exited(cmd) {
			log, _ := os.ReadFile(logPath)
			if os.Getenv("CI") == "" {
				t.Skipf("%v did not start under omnisnap (not every browser is installed here?):\n%s", missing, log)
			}
			t.Fatalf("%v did not start under omnisnap:\n%s", missing, log)
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Let the browsers start their renderers: those must die too.
	time.Sleep(2 * time.Second)
	tree = descendants(t, omnisnap)
	t.Logf("omnisnap %d runs %d processes: %s", omnisnap, len(tree), summary(tree))

	// Handles taken before the kill: waiting on them cannot hit a reused pid.
	var waits []waitable
	for _, p := range tree {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, p.pid)
		if err != nil {
			continue // already gone
		}
		waits = append(waits, waitable{proc: p, h: h})
	}
	defer func() {
		for _, w := range waits {
			_ = windows.CloseHandle(w.h)
		}
	}()

	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, omnisnap)
	if err != nil {
		t.Fatalf("OpenProcess omnisnap: %v", err)
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		t.Fatalf("TerminateProcess omnisnap: %v", err)
	}
	_ = windows.CloseHandle(h)
	_ = cmd.Wait()

	end := time.Now().Add(15 * time.Second)
	var survivors []string
	for _, w := range waits {
		ms := uint32(max(time.Until(end), 0) / time.Millisecond)
		if ev, _ := windows.WaitForSingleObject(w.h, ms); ev != windows.WAIT_OBJECT_0 {
			survivors = append(survivors, fmt.Sprintf("%s (%d)", w.name, w.pid))
		}
	}
	if len(survivors) > 0 {
		t.Errorf("%d of %d processes survived TerminateProcess on omnisnap: %s", len(survivors), len(waits), strings.Join(survivors, ", "))
	}
	// Nothing started meanwhile under the dead tree either. Every process of
	// the tree has exited by now, so a process created after this moment
	// cannot be its child, whatever its parent id says: the id outlives the
	// parent and is reused (see descendants). On PR #23's first run
	// (46011265973) this check flagged an msedge.exe whose parent id was one
	// of the tree's, with every process of the tree gone.
	allExited := filetimeNow()
	known := map[uint32]bool{omnisnap: true}
	for _, p := range tree {
		known[p.pid] = true
	}
	for _, p := range snapshot(t) {
		if !known[p.parent] || known[p.pid] || !isBrowser(p.name) {
			continue
		}
		switch c := creationTime(p.pid); {
		case c == 0:
			t.Logf("%s (%d), parent id %d of the killed tree: exited before it could be read", p.name, p.pid, p.parent)
		case c > allExited:
			t.Logf("%s (%d), parent id %d of the killed tree: started %v after the tree exited, so the id was reused",
				p.name, p.pid, p.parent, time.Duration(c-allExited)*100)
		default:
			t.Errorf("%s (%d), a child of the killed tree, is running (started %v before the tree exited)",
				p.name, p.pid, time.Duration(allExited-c)*100)
		}
	}
}

// filetimeNow is the current time in creationTime's units.
func filetimeNow() int64 {
	ft := windows.NsecToFiletime(time.Now().UnixNano())
	return int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
}

type proc struct {
	pid, parent uint32
	name        string
}

type waitable struct {
	proc
	h windows.Handle
}

// snapshot lists every process (Toolhelp: th32ParentProcessID is "the
// process that created this process").
// Source: https://learn.microsoft.com/en-us/windows/win32/api/tlhelp32/ns-tlhelp32-processentry32
func snapshot(t *testing.T) []proc {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatalf("CreateToolhelp32Snapshot: %v", err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, proc{pid: e.ProcessID, parent: e.ParentProcessID, name: strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))})
	}
	return out
}

// descendants is every process below root.
//
// th32ParentProcessID outlives the parent, and Windows reuses process ids, so
// a parent id alone can name the wrong process: on 2026-11-25 a browser of
// the test reused the id of csrss.exe's long-gone creator, and the "tree"
// took in csrss, wininit, services and every svchost (171 processes). A link
// counts only when the child was created after the parent: a process cannot
// be older than its creator. A process whose creation time cannot be read
// (already gone, or not queryable) is left out; the browsers under test can
// always be queried.
// Source: https://learn.microsoft.com/en-us/windows/win32/api/tlhelp32/ns-tlhelp32-processentry32
// ("th32ParentProcessID ... If the parent process exits, the ID may be reused")
func descendants(t *testing.T, root uint32) []proc {
	t.Helper()
	all := snapshot(t)
	created := map[uint32]int64{}
	createdAt := func(pid uint32) (int64, bool) {
		if c, ok := created[pid]; ok {
			return c, c != 0
		}
		c := creationTime(pid)
		created[pid] = c
		return c, c != 0
	}
	in := map[uint32]bool{root: true}
	var out []proc
	for changed := true; changed; {
		changed = false
		for _, p := range all {
			if !in[p.parent] || in[p.pid] || p.pid == 0 {
				continue
			}
			child, ok1 := createdAt(p.pid)
			parent, ok2 := createdAt(p.parent)
			if !ok1 || !ok2 || child < parent {
				continue // a stale parent id: the creator's id was reused
			}
			in[p.pid] = true
			out = append(out, p)
			changed = true
		}
	}
	return out
}

// creationTime is when pid started, in 100 ns units since 1601, or 0 when it
// cannot be read.
func creationTime(pid uint32) int64 {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0
	}
	return int64(created.HighDateTime)<<32 | int64(created.LowDateTime)
}

func missingNames(tree []proc, want []string) []string {
	var missing []string
	for _, w := range want {
		found := false
		for _, p := range tree {
			found = found || p.name == w
		}
		if !found {
			missing = append(missing, w)
		}
	}
	return missing
}

func isBrowser(name string) bool {
	return name == "chrome.exe" || name == "msedge.exe" || name == "firefox.exe"
}

func summary(tree []proc) string {
	n := map[string]int{}
	for _, p := range tree {
		n[p.name]++
	}
	var parts []string
	for name, c := range n {
		parts = append(parts, fmt.Sprintf("%d %s", c, name))
	}
	return strings.Join(parts, ", ")
}

func exited(cmd *exec.Cmd) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		return true
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	return windows.GetExitCodeProcess(h, &code) != nil || code != 259 // STILL_ACTIVE
}

// buildOmnisnap builds ./cmd/omnisnap from this module.
func buildOmnisnap(t *testing.T) string {
	t.Helper()
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	root := filepath.Dir(strings.TrimSpace(string(gomod)))
	bin := filepath.Join(t.TempDir(), "omnisnap.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/omnisnap")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}
