package capture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// outcomeLog collects outcomes from the lanes (they report concurrently).
type outcomeLog struct {
	mu   sync.Mutex
	list []Outcome
}

func (l *outcomeLog) add(o Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, o)
}

func (l *outcomeLog) runs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.list))
	for i, o := range l.list {
		out[i] = o.Engine + " " + o.Run
	}
	return out
}

func (l *outcomeLog) errors() map[string]error {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]error{}
	for _, o := range l.list {
		out[o.Engine+" "+o.Run] = o.Err
	}
	return out
}

// scriptedEngine builds an engine whose pages are configured by tweak.
func scriptedEngine(id string, calls *int, tweak func(*fakeSession)) Engine {
	return Engine{ID: id, Family: schema.FamilyBlink, Version: "test",
		NewSession: func(context.Context) (drivers.Session, error) {
			*calls++
			f := newFake()
			f.nodes = 60 // a real page, not a bot wall
			if tweak != nil {
				tweak(f)
			}
			return f, nil
		}}
}

func TestPolicyBaselineTimeoutMarksUnreachable(t *testing.T) {
	out := t.TempDir()
	var baseCalls, otherCalls int
	base := scriptedEngine("chrome", &baseCalls, func(f *fakeSession) { f.navErr = drivers.ErrTimeout })
	other := scriptedEngine("firefox", &otherCalls, nil)
	page := schema.PageRef{Slug: "p", URL: "https://example.com/"}
	log := &outcomeLog{}
	err := RunPage(context.Background(), Plan{Out: out, Baseline: "chrome", Runs: 2, Engines: []Engine{base, other}}, page, log.add)
	if err != nil {
		t.Fatal(err)
	}
	if baseCalls != 1 || otherCalls != 0 {
		t.Errorf("sessions: baseline %d, other %d; want 1 and 0 (unreachable pages are not captured elsewhere)", baseCalls, otherCalls)
	}
	if got := log.runs(); len(got) != 1 || got[0] != "chrome a" {
		t.Errorf("outcomes %v; want only the baseline's run a", got)
	}
	f, err := ReadFailure(out, "p")
	if err != nil || !strings.Contains(f.Reason, "timed out") {
		t.Errorf("failed.json = %+v, %v; want a timeout reason", f, err)
	}
}

func TestPolicyBotWallMarksUnreachable(t *testing.T) {
	out := t.TempDir()
	var baseCalls, otherCalls int
	base := scriptedEngine("chrome", &baseCalls, func(f *fakeSession) { f.nodes = botWallNodes - 1 })
	other := scriptedEngine("firefox", &otherCalls, nil)
	page := schema.PageRef{Slug: "p", URL: "https://example.com/"}
	err := RunPage(context.Background(), Plan{Out: out, Baseline: "chrome", Runs: 2, Engines: []Engine{base, other}}, page, func(Outcome) {})
	if err != nil {
		t.Fatal(err)
	}
	if baseCalls != 1 || otherCalls != 0 {
		t.Errorf("sessions: baseline %d, other %d; want 1 and 0", baseCalls, otherCalls)
	}
	f, err := ReadFailure(out, "p")
	if err != nil || !strings.Contains(f.Reason, "bot wall") {
		t.Errorf("failed.json = %+v, %v; want a bot-wall reason", f, err)
	}
}

func TestPolicyFailedRunASkipsRunB(t *testing.T) {
	out := t.TempDir()
	var baseCalls, otherCalls int
	base := scriptedEngine("chrome", &baseCalls, nil)
	other := scriptedEngine("firefox", &otherCalls, func(f *fakeSession) { f.navErr = errors.New("boom") })
	page := schema.PageRef{Slug: "p", URL: "https://example.com/"}
	log := &outcomeLog{}
	if err := RunPage(context.Background(), Plan{Out: out, Baseline: "chrome", Runs: 2, Engines: []Engine{base, other}}, page, log.add); err != nil {
		t.Fatal(err)
	}
	// The baseline's run a runs first (policy); then, in the lane, every run a
	// before run b, and the failed engine's run b is skipped.
	want := []string{"chrome a", "firefox a", "chrome b"}
	if got := log.runs(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("outcomes %v; want %v (the failed engine skips run b)", got, want)
	}
	if log.errors()["firefox a"] == nil {
		t.Error("firefox run a has no error")
	}
}

func TestPolicyFullCaptureNeverSkips(t *testing.T) {
	out := t.TempDir()
	var baseCalls, otherCalls int
	// A planted page with a handful of elements: the bot-wall rule is off.
	base := scriptedEngine("chrome", &baseCalls, func(f *fakeSession) { f.nodes = 1 })
	other := scriptedEngine("firefox", &otherCalls, func(f *fakeSession) { f.nodes = 1 })
	page := schema.PageRef{Slug: "planted", URL: "http://127.0.0.1/p.html"}
	log := &outcomeLog{}
	if err := RunPage(context.Background(), Plan{Out: out, Baseline: "chrome", Runs: 2, FullCapture: true,
		Engines: []Engine{base, other}}, page, log.add); err != nil {
		t.Fatal(err)
	}
	if baseCalls != 2 || otherCalls != 2 {
		t.Errorf("sessions: %d and %d; planted pages are always fully captured (2 each)", baseCalls, otherCalls)
	}
	if _, err := ReadFailure(out, "planted"); !os.IsNotExist(err) {
		t.Errorf("failed.json written for a planted page: %v", err)
	}
}

func TestUnreachableReasonPrefersNavigations(t *testing.T) {
	for _, tc := range []struct {
		res  Result
		err  error
		want bool
	}{
		{Result{Nodes: 100}, drivers.ErrNavigation, true},
		{Result{Nodes: botWallNodes}, nil, false},
		{Result{Nodes: botWallNodes - 1}, nil, true},
		{Result{Nodes: 0}, errors.New("other"), false},
	} {
		if _, got := unreachable(tc.res, tc.err); got != tc.want {
			t.Errorf("unreachable(%+v, %v) = %v; want %v", tc.res, tc.err, got, tc.want)
		}
	}
}

func TestFailureIsWrittenAtomically(t *testing.T) {
	out := t.TempDir()
	if err := WriteFailure(out, schema.PageRef{Slug: "p", URL: "https://example.com/"}, "why"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "p", "failed.json")); err != nil {
		t.Error(err)
	}
	// No temp file is left behind.
	entries, _ := os.ReadDir(filepath.Join(out, "p"))
	for _, e := range entries {
		if e.Name() != "failed.json" {
			t.Errorf("leftover file %s", e.Name())
		}
	}
}

// Resume at the capture level: a kill and a resume recapture nothing already
// attempted (Task 26, success criterion 5).
func TestResumeRecapturesNothingAlreadyAttempted(t *testing.T) {
	out := t.TempDir()
	page := schema.PageRef{Slug: "p", URL: "https://example.com/"}
	// A killed run left run a done and run b never started.
	write(t, out, page.Slug, "chrome-a.json", "chrome-a.png")
	var calls int
	e := scriptedEngine("chrome", &calls, nil)
	log := &outcomeLog{}
	plan := Plan{Out: out, Baseline: "chrome", Runs: 2, Engines: []Engine{e}, Resume: true}
	if err := RunPage(context.Background(), plan, page, log.add); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("sessions %d; want 1 (only the missing run b)", calls)
	}
	if got := log.runs(); len(got) != 1 || got[0] != "chrome b" {
		t.Errorf("outcomes %v; want only chrome b", got)
	}
	// Now everything exists: nothing is opened.
	calls = 0
	log = &outcomeLog{}
	if err := RunPage(context.Background(), plan, page, log.add); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("sessions %d; want 0 (nothing left to capture)", calls)
	}
}

// --retry-failed repeats a failed capture; without it, the error file is an
// attempt and is left alone.
func TestResumeRetryFailed(t *testing.T) {
	for _, retry := range []bool{false, true} {
		out := t.TempDir()
		page := schema.PageRef{Slug: "p", URL: "https://example.com/"}
		write(t, out, page.Slug, "chrome-a.error.txt")
		var calls int
		e := scriptedEngine("chrome", &calls, func(f *fakeSession) { f.navErr = drivers.ErrTimeout })
		plan := Plan{Out: out, Baseline: "chrome", Runs: 2, Engines: []Engine{e}, Resume: true, RetryFailed: retry}
		if err := RunPage(context.Background(), plan, page, func(Outcome) {}); err != nil {
			t.Fatal(err)
		}
		want := 0
		if retry {
			want = 1 // run a retried; it fails again, so run b is still skipped
		}
		if calls != want {
			t.Errorf("retry=%v: sessions %d; want %d", retry, calls, want)
		}
	}
}
