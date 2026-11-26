package cli

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/discovery"
)

// Change cap-live-browsers: at most --max-browsers live browsers; more
// engines run in consecutive groups, in job order with the baseline first.

func fiveEngines() []discovery.Browser {
	return []discovery.Browser{blink("chrome"), gecko("firefox"), webkit("webkitgtk"), blink("edge"), blink("brave")}
}

func TestMaxBrowsersFlag(t *testing.T) {
	for _, v := range []string{"0", "-1", "two", "2.5"} {
		t.Run(v, func(t *testing.T) {
			m := &fakeMachine{browsers: fiveEngines()}
			code, _, stderr := run(t, m, "https://example.com/", "--out", t.TempDir(), "--max-browsers", v)
			if code != ExitUsage || !strings.Contains(stderr, "max-browsers") || !strings.Contains(stderr, "1 or more") {
				t.Errorf("exit %d, stderr %q; want %d naming the flag and the accepted values", code, stderr, ExitUsage)
			}
			if len(m.launched) != 0 {
				t.Errorf("launched %v before rejecting the flag", m.launched)
			}
		})
	}
	for name, args := range map[string][]string{"default": nil, "eight": {"--max-browsers", "8"}} {
		t.Run(name, func(t *testing.T) {
			m := &fakeMachine{browsers: fiveEngines()}
			a := append([]string{"https://example.com/", "--out", t.TempDir(), "--quiet", "--runs", "1"}, args...)
			if code, _, stderr := run(t, m, a...); code != ExitOK {
				t.Fatalf("exit %d, %s", code, stderr)
			}
			want := 3
			if name == "eight" {
				want = 5 // all five live together: one group
			}
			if m.maxLive != want {
				t.Errorf("at most %d live; want %d", m.maxLive, want)
			}
		})
	}
}

// launchesAndCloses keeps the log's launch and close lines.
func launchesAndCloses(m *fakeMachine) string {
	var out []string
	for _, l := range m.log {
		if !strings.HasPrefix(l, "nav ") {
			out = append(out, l)
		}
	}
	return strings.Join(out, ", ")
}

func TestGroupsInJobOrder(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines()}
	if code, _, stderr := run(t, m, "https://example.com/", "https://example.org/", "--out", t.TempDir(), "--quiet", "--runs", "1"); code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	got := launchesAndCloses(m)
	// The close order within a group is a map's; check the groups instead.
	g1, g2, ok := strings.Cut(got, ", launch edge")
	if !ok || !strings.HasPrefix(g1, "launch chrome, launch firefox, launch webkitgtk, close ") ||
		strings.Count(g1, "close ") != 3 || !strings.HasPrefix(g2, ", launch brave, close ") {
		t.Fatalf("launch/close order %q; want [chrome firefox webkitgtk] closed before [edge brave]", got)
	}
	if m.maxLive != 3 {
		t.Errorf("at most %d live; want 3", m.maxLive)
	}
	// Each group captures every page before the next group starts.
	firstEdge := -1
	for i, l := range m.log {
		if l == "launch edge" {
			firstEdge = i
		}
	}
	for i, l := range m.log {
		group1 := strings.HasPrefix(l, "nav chrome ") || strings.HasPrefix(l, "nav firefox ") || strings.HasPrefix(l, "nav webkitgtk ")
		if group1 && i > firstEdge {
			t.Errorf("%q after group 2 launched", l)
		}
	}
	if n := len(m.navs); n != 10 {
		t.Errorf("%d navigations; want 10 (5 engines x 2 pages x 1 run)", n)
	}
}

func TestBaselineListedLastLaunchesFirst(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines()}
	if code, _, stderr := run(t, m, "https://example.com/", "--out", t.TempDir(), "--quiet", "--runs", "1",
		"--engines", "firefox,webkitgtk,edge,brave,chrome", "--baseline", "chrome"); code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.HasPrefix(launchesAndCloses(m), "launch chrome, launch firefox, launch webkitgtk, close ") {
		t.Errorf("order %q; want the baseline in the first group", launchesAndCloses(m))
	}
}

func TestLaunchFailuresDoNotUseTheCap(t *testing.T) {
	m := &fakeMachine{browsers: []discovery.Browser{blink("chrome"), blink("chromium"), blink("edge"), gecko("firefox"), webkit("webkitgtk")},
		failLaunch: map[string]bool{"chromium": true, "edge": true}}
	out := t.TempDir()
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--runs", "1"); code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if got := launchesAndCloses(m); !strings.HasPrefix(got, "launch chrome, launch firefox, launch webkitgtk, close ") || strings.Count(got, "launch ") != 3 {
		t.Errorf("order %q; want chrome, firefox and webkitgtk live together in one group", got)
	}
	s := readSummary(t, out)
	reasons := map[string]string{}
	for _, sk := range s.Skipped {
		reasons[sk.Engine] = sk.Reason
	}
	for _, id := range []string{"chromium", "edge"} {
		if !strings.HasPrefix(reasons[id], "launch failed: ") {
			t.Errorf("%s skipped as %q; want launch failed with its error", id, reasons[id])
		}
	}
}

func TestUnreachableInGroupOneIsSkippedLater(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines(), failURL: map[string]bool{"https://down.example/": true}}
	out := t.TempDir()
	if code, _, stderr := run(t, m, "https://example.com/", "https://down.example/", "--out", out, "--quiet", "--runs", "1"); code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	for _, n := range m.navs {
		if strings.HasSuffix(n, " https://down.example/") && !strings.HasPrefix(n, "chrome ") {
			t.Errorf("%q: only the baseline's run a may open the unreachable page", n)
		}
	}
	s := readSummary(t, out)
	failed := 0
	for _, p := range s.Pages {
		if p.Slug == "down.example" {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("down.example listed %d times in pages; want once", failed)
	}
}

func TestFontCheckInLaterGroups(t *testing.T) {
	// Firefox (the fake resolves sans-serif to DejaVu Sans) lands in group 2,
	// after the baseline's browser is closed.
	m := &fakeMachine{browsers: []discovery.Browser{blink("chrome"), blink("edge"), blink("brave"), gecko("firefox")}}
	code, stdout, stderr := run(t, m, "https://example.com/", "--out", t.TempDir(), "--runs", "1")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "font parity: chrome resolves sans-serif") || !strings.Contains(stdout, "firefox to DejaVu Sans") {
		t.Errorf("stdout %q; want the group-2 engine compared with the baseline", stdout)
	}
}

func TestPageEventsAcrossGroups(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines()}
	code, stdout, stderr := run(t, m, "https://example.com/", "https://example.org/", "--out", t.TempDir(), "--json")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	starts, dones := map[string]int{}, map[string]int{}
	open := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		var ev struct{ Type, Page string }
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		switch ev.Type {
		case "page-start":
			starts[ev.Page]++
			open[ev.Page] = true
		case "page-done":
			dones[ev.Page]++
			open[ev.Page] = false
		case "capture":
			if !open[ev.Page] {
				t.Errorf("capture of %s outside its page-start/page-done", ev.Page)
			}
		}
	}
	for _, slug := range []string{"example.com", "example.org"} {
		if starts[slug] != 1 || dones[slug] != 1 {
			t.Errorf("%s: %d page-start, %d page-done; want one each", slug, starts[slug], dones[slug])
		}
	}
}

func TestResumePerGroup(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines()}
	out := t.TempDir()
	// Group 1 finished, then the run stopped: the same three engines alone.
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--runs", "1", "--engines", "chrome,firefox,webkitgtk"); code != ExitOK {
		t.Fatalf("first run: exit %d, %s", code, stderr)
	}
	m.navs = nil
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--runs", "1", "--resume"); code != ExitOK {
		t.Fatalf("resumed run: exit %d, %s", code, stderr)
	}
	if strings.Join(m.navs, ",") != "edge https://example.com/,brave https://example.com/" &&
		strings.Join(m.navs, ",") != "brave https://example.com/,edge https://example.com/" {
		t.Errorf("resumed navigations %v; want only group 2's engines", m.navs)
	}
}

func TestOOMScoreAfterEverySession(t *testing.T) {
	m := &fakeMachine{browsers: fiveEngines()}
	if code, _, stderr := run(t, m, "https://example.com/", "--out", t.TempDir(), "--quiet"); code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if m.sessions == 0 || m.oomCalls != m.sessions {
		t.Errorf("%d oom calls for %d sessions; want one per session", m.oomCalls, m.sessions)
	}
}
