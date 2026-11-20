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

// crashyEngine is an engine whose browser dies: crashes says how many of its
// next captures crash (-1: every one), openCrash makes the dead browser
// refuse new pages as a real one does ("open chrome: cdp: connection
// closed"), until a relaunch.
type crashyEngine struct {
	mu        sync.Mutex
	crashes   int
	openCrash bool
	dead      bool
	relaunchs int
	noRestart bool // Relaunch fails
	okFirst   int  // this many sessions open before the crashes start
	opened    int
}

func (c *crashyEngine) engine(id string) Engine {
	return Engine{
		ID: id, Family: schema.FamilyBlink, Version: "test",
		NewSession: func(context.Context) (drivers.Session, error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.dead && c.openCrash {
				return nil, fmt.Errorf("cdp: connection closed: %w", drivers.ErrCrashed)
			}
			c.opened++
			s := newFake()
			s.nodes = 120
			if c.opened > c.okFirst && c.crashes != 0 {
				if c.crashes > 0 {
					c.crashes--
				}
				c.dead = true
				s.navErr = fmt.Errorf("cdp: page: %w", drivers.ErrCrashed)
			}
			return s, nil
		},
		Relaunch: func(context.Context) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.relaunchs++
			if c.noRestart {
				return errors.New("no display")
			}
			c.dead = false
			return nil
		},
	}
}

func steadyEngine(id string) Engine {
	return Engine{ID: id, Family: schema.FamilyGecko, Version: "test",
		NewSession: func(context.Context) (drivers.Session, error) {
			s := newFake()
			s.nodes = 120
			return s, nil
		}}
}

func runPages(t *testing.T, p Plan, slugs ...string) []Outcome {
	t.Helper()
	var mu sync.Mutex
	var got []Outcome
	for _, slug := range slugs {
		err := RunPage(context.Background(), p, schema.PageRef{Slug: slug, URL: "https://example.com/" + slug}, func(o Outcome) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, o)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return got
}

func outcomeLines(os []Outcome) string {
	var b strings.Builder
	for _, o := range os {
		fmt.Fprintf(&b, "%s %s err=%v retried=%v lost=%v\n", o.Engine, o.Run, o.Err != nil, o.Retried, o.Lost != "")
	}
	return b.String()
}

// One crash: the run is taken again in a fresh browser, and the job goes on
// as if nothing happened.
func TestCrashRetriedOnceInAFreshBrowser(t *testing.T) {
	c := &crashyEngine{crashes: 1, openCrash: true}
	p := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 2, AllowSmallPages: true,
		Baseline: "chrome", Lost: NewLost(), Engines: []Engine{c.engine("chrome"), steadyEngine("firefox")}}
	got := runPages(t, p, "one", "two")
	want := "chrome a err=false retried=true lost=false\n" +
		"firefox a err=false retried=false lost=false\n" +
		"chrome b err=false retried=false lost=false\n" +
		"firefox b err=false retried=false lost=false\n"
	if s := outcomeLines(got); !strings.HasPrefix(s, want) || strings.Count(s, "\n") != 8 || strings.Contains(s, "err=true") {
		t.Errorf("outcomes:\n%s\nwant page one to start\n%s\nand every capture of both pages to succeed", s, want)
	}
	if c.relaunchs != 1 {
		t.Errorf("relaunched %d times; want once", c.relaunchs)
	}
	if _, lost := p.Lost.Reason("chrome"); lost {
		t.Error("chrome lost after one crash")
	}
	for _, slug := range []string{"one", "two"} {
		for _, f := range []string{"chrome-a.json", "chrome-b.json", "firefox-a.json", "firefox-b.json"} {
			if _, err := os.Stat(filepath.Join(p.Out, slug, f)); err != nil {
				t.Error(err)
			}
		}
	}
}

// Two crashes in a row: the engine is lost with one outcome naming the crash;
// its later captures are recorded as skipped (no outcome, no attempt), and
// the other engine captures every page.
func TestCrashTwiceLosesTheEngine(t *testing.T) {
	// youtube's run b is where Chrome died in the field (Task 89): run a
	// captures, then every capture crashes.
	c := &crashyEngine{crashes: -1, okFirst: 1, openCrash: true}
	out := t.TempDir()
	base := steadyEngine("chromium")
	base.Family = schema.FamilyBlink
	p := Plan{Out: out, Viewport: schema.DefaultViewport, Runs: 2, AllowSmallPages: true,
		Baseline: "chromium", Lost: NewLost(), Engines: []Engine{base, c.engine("chrome"), steadyEngine("firefox")}}
	all := runPages(t, p, "youtube", "ui")
	lines := outcomeLines(all)
	if n := strings.Count(lines, "chrome "); n != 2 {
		t.Errorf("chrome outcomes:\n%s\nwant 2 (youtube a ok, youtube b lost); later captures add none", lines)
	}
	var lostOutcome *Outcome
	for i := range all {
		if all[i].Lost != "" {
			lostOutcome = &all[i]
		}
	}
	if lostOutcome == nil || lostOutcome.Engine != "chrome" || lostOutcome.Run != "b" {
		t.Fatalf("outcomes:\n%s\nwant chrome's youtube run b to lose it", lines)
	}
	if !strings.Contains(lostOutcome.Lost, "chrome crashed twice on youtube run b") ||
		!strings.Contains(lostOutcome.Lost, "the other engines go on") {
		t.Errorf("lost = %q; want the engine, the page, the run, and that the job goes on", lostOutcome.Lost)
	}
	if c.relaunchs != 1 {
		t.Errorf("relaunched %d times; want once: a lost engine is not restarted again", c.relaunchs)
	}
	// ui: chrome skipped with the crash named; the others captured.
	b, err := os.ReadFile(filepath.Join(out, "ui", "chrome-a.error.txt"))
	if err != nil || !strings.HasPrefix(string(b), "skipped: chrome crashed twice on youtube run b") {
		t.Errorf("ui/chrome-a.error.txt = %q, %v; want the skip naming the crash", b, err)
	}
	for _, f := range []string{"chromium-a.json", "chromium-b.json", "firefox-a.json", "firefox-b.json"} {
		if _, err := os.Stat(filepath.Join(out, "ui", f)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "ui", "chrome-b.error.txt")); err == nil {
		t.Error("ui/chrome-b.error.txt written: run a's skip already ends chrome's runs")
	}
	// Resume counts the skipped page as done; --retry-failed takes it again.
	if st := ResumeState(out, "ui", []string{"chromium", "chrome", "firefox"}, 2); !st.Done {
		t.Errorf("resume state %+v; want ui done", st)
	}
}

// A lost baseline does not make the page unreachable: the other engines are
// captured, and later pages go on without the baseline gate.
func TestLostBaselineIsNotUnreachable(t *testing.T) {
	c := &crashyEngine{crashes: -1}
	p := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 2, Baseline: "chrome", Lost: NewLost(),
		Engines: []Engine{c.engine("chrome"), steadyEngine("firefox")}}
	got := outcomeLines(runPages(t, p, "one", "two"))
	if _, err := os.Stat(filepath.Join(p.Out, "one", "failed.json")); err == nil {
		t.Error("one marked unreachable: a crash is the browser's fault, not the page's")
	}
	for _, slug := range []string{"one", "two"} {
		for _, f := range []string{"firefox-a.json", "firefox-b.json"} {
			if _, err := os.Stat(filepath.Join(p.Out, slug, f)); err != nil {
				t.Errorf("%v\noutcomes:\n%s", err, got)
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(p.Out, "two", "chrome-a.error.txt")); err != nil || !strings.HasPrefix(string(b), "skipped: ") {
		t.Errorf("two/chrome-a.error.txt = %q, %v; want a skip", b, err)
	}
}

// A browser that cannot be started again is lost after its first crash, and
// a refused page (the dead browser) is recorded in the bundle.
func TestCrashWithoutRestart(t *testing.T) {
	c := &crashyEngine{crashes: -1, openCrash: true, noRestart: true}
	p := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 2, Lost: NewLost(),
		Engines: []Engine{c.engine("chrome")}}
	got := runPages(t, p, "one")
	if len(got) != 1 || got[0].Lost == "" || !strings.Contains(got[0].Lost, "did not restart: no display") {
		t.Fatalf("outcomes %+v; want one, losing chrome because it did not restart", got)
	}
	if _, err := os.Stat(filepath.Join(p.Out, "one", "chrome-a.error.txt")); err != nil {
		t.Error(err)
	}
}

// An error that is not a crash is not retried.
func TestNavigationErrorIsNotRetried(t *testing.T) {
	relaunched := false
	e := Engine{ID: "chrome", Family: schema.FamilyBlink,
		NewSession: func(context.Context) (drivers.Session, error) {
			s := newFake()
			s.navErr = fmt.Errorf("x: %w", drivers.ErrNavigation)
			return s, nil
		},
		Relaunch: func(context.Context) error { relaunched = true; return nil }}
	p := Plan{Out: t.TempDir(), Viewport: schema.DefaultViewport, Runs: 1, Lost: NewLost(), Engines: []Engine{e}}
	got := runPages(t, p, "one")
	if relaunched || len(got) != 1 || got[0].Retried || got[0].Lost != "" {
		t.Errorf("outcomes %+v, relaunched %v; a navigation error is the page's, not a crash", got, relaunched)
	}
}
