package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
)

func run(t *testing.T, m *fakeMachine, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := main0(context.Background(), append([]string{"omnisnap"}, args...), &stdout, &stderr, m.env())
	return code, stdout.String(), stderr.String()
}

func readSummary(t *testing.T, out string) schema.Summary {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "summary", b)
	var s schema.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUsageErrors(t *testing.T) {
	dir := t.TempDir()
	badJob := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(badJob, []byte(`{"pages":[{"url":"https://example.com/"}],"engine":["chrome"]}`), 0o644)
	emptyJob := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(emptyJob, []byte(`{"pages":[]}`), 0o644)
	for name, args := range map[string][]string{
		"no arguments":      {},
		"unknown command":   {"frobnicate"},
		"run without file":  {"run"},
		"missing job file":  {"run", filepath.Join(dir, "nope.json")},
		"unknown job field": {"run", badJob},
		"job without pages": {"run", emptyJob},
		"unknown flag":      {"https://example.com/", "--colour"},
		"bad viewport":      {"https://example.com/", "--viewport", "12x"},
		"zero runs":         {"https://example.com/", "--runs", "0"},
		"not http":          {"ftp://example.com/"},
		"browsers extra":    {"browsers", "extra"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := run(t, &fakeMachine{browsers: nil}, args...)
			if code != ExitUsage || stderr == "" || stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q; want %d, a message on stderr only", code, stdout, stderr, ExitUsage)
			}
		})
	}
}

func TestNoBaselineIsExit2(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, gecko("firefox"))
	code, _, stderr := run(t, m, "https://example.com/", "--out", t.TempDir())
	if code != ExitNoBrowser || !strings.Contains(stderr, "baseline") {
		t.Errorf("exit %d, stderr %q; want %d naming the baseline", code, stderr, ExitNoBrowser)
	}
	m.browsers = append(m.browsers, blink("chromium"))
	code, _, stderr = run(t, m, "https://example.com/", "--out", t.TempDir(), "--baseline", "edge")
	if code != ExitNoBrowser || !strings.Contains(stderr, "edge") {
		t.Errorf("requested baseline missing: exit %d, stderr %q", code, stderr)
	}
}

func TestEveryCaptureFailedIsExit3(t *testing.T) {
	m := &fakeMachine{browsers: nil, failNav: map[string]bool{"chromium": true}}
	m.browsers = append(m.browsers, blink("chromium"))
	out := t.TempDir()
	code, _, _ := run(t, m, "https://example.com/", "--out", out, "--quiet")
	if code != ExitAllFailed {
		t.Errorf("exit %d; want %d", code, ExitAllFailed)
	}
	s := readSummary(t, out) // a bundle is still written
	if len(s.Results) != 1 || s.Results[0].Status != schema.StatusFailed {
		t.Errorf("results %+v; want one failed result", s.Results)
	}
}

func TestInternalErrorIsExit4(t *testing.T) {
	code, _, stderr := run(t, &fakeMachine{panicOn: true}, "https://example.com/", "--out", t.TempDir())
	if code != ExitInternal || !strings.Contains(stderr, "bug") {
		t.Errorf("exit %d, stderr %q; want %d asking for a report", code, stderr, ExitInternal)
	}
}

// omnisnap <url> --json: every stdout line is a valid progress event, the
// last is done, and the bundle is complete and schema-valid.
func TestURLRunWritesBundle(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	out := t.TempDir()
	code, stdout, stderr := run(t, m, "https://example.com/pricing", "--out", out, "--json")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	events := lines(stdout)
	for _, l := range events {
		schematest.Validate(t, "progress", []byte(l))
	}
	var last struct{ Type, Summary string }
	_ = json.Unmarshal([]byte(events[len(events)-1]), &last)
	if last.Type != "done" || last.Summary != filepath.Join(out, "summary.json") {
		t.Errorf("last event %s; want done with the summary path", events[len(events)-1])
	}

	s := readSummary(t, out)
	if len(s.Skipped) != 0 {
		t.Errorf("skipped %+v; both families have a driver since Phase 3", s.Skipped)
	}
	// The baseline is captured but is not a result of its own: the one result
	// compares the second engine with it.
	if len(s.Results) != 1 || s.Results[0].Engine.Engine != "firefox" {
		t.Fatalf("results %+v; want one for firefox against the chromium baseline", s.Results)
	}
	r := s.Results[0]
	if r.Baseline.Engine != "chromium" || r.Status != schema.StatusOK || r.Shots == nil {
		t.Errorf("result %+v; want ok against the chromium baseline, with screenshots", r)
	}
	if len(r.Blocked.Engine) != 1 || r.Blocked.Engine[0] != "ads.example.net" {
		t.Errorf("blocked %+v; want the blocked host", r.Blocked)
	}
	slug := r.Page.Slug
	for _, eng := range []string{"chromium", "firefox"} {
		for _, f := range []string{eng + "-a.json", eng + "-a.png", eng + "-b.json", eng + "-b.png"} {
			if _, err := os.Stat(filepath.Join(out, slug, f)); err != nil {
				t.Error(err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(out, slug, "page.json")); err != nil {
		t.Error(err)
	}
	host, err := os.ReadFile(filepath.Join(out, "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "host", host)
	if _, err := os.Stat(filepath.Join(out, "report.html")); err != nil {
		t.Error(err)
	}
	if len(m.launched) != 2 || len(m.closed) != 2 {
		t.Errorf("launched %v, closed %v; want each engine launched once and closed", m.launched, m.closed)
	}
}

// run job.json: two Blink engines compared, captured interleaved (run a of
// every engine, then run b) so a live page cannot drift between engines.
func TestRunJobFileInterleaves(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), blink("chrome"))
	out := t.TempDir()
	job := filepath.Join(t.TempDir(), "job.json")
	_ = os.WriteFile(job, []byte(`{"pages":[{"url":"https://example.com/","slug":"home"}],"engines":["chromium"],"output":"`+out+`"}`), 0o644)
	code, stdout, stderr := run(t, m, "run", job)
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := []string{"chrome https://example.com/", "chromium https://example.com/", "chrome https://example.com/", "chromium https://example.com/"}
	if strings.Join(m.navs, " | ") != strings.Join(want, " | ") {
		t.Errorf("navigations %q; want baseline chrome added and runs interleaved", m.navs)
	}
	s := readSummary(t, out)
	if len(s.Results) != 1 || s.Results[0].Baseline.Engine != "chrome" || s.Results[0].Engine.Engine != "chromium" {
		t.Errorf("results %+v; want chromium compared with chrome", s.Results)
	}
	if !strings.Contains(stdout, filepath.Join(out, "report.html")) {
		t.Errorf("stdout %q; want the report path", stdout)
	}
}

func TestFlagsAfterURL(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"))
	out := t.TempDir()
	if code, _, stderr := run(t, m, "https://example.com/", "--runs", "1", "--out", out, "--quiet"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(m.navs) != 1 {
		t.Errorf("navigations %q; want one (--runs 1)", m.navs)
	}
}

func TestBrowsersJSON(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	code, stdout, _ := run(t, m, "browsers", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	// The list validates as the engine list of host.json (SPEC-discovery.md, success criterion 5).
	schematest.Validate(t, "host", []byte(`{"os":"linux","arch":"amd64","engines":`+stdout+`}`))
	code, stdout, _ = run(t, m, "browsers")
	if code != ExitOK || !strings.Contains(stdout, "chromium") || !strings.Contains(stdout, "firefox") {
		t.Errorf("exit %d, %q", code, stdout)
	}
}

func TestSlugFromURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/":                "example.com",
		"http://127.0.0.1:8080/01-card-width": "127.0.0.1-8080-01-card-width",
		"https://Example.com/A/B.html?x=1#y":  "example.com-a-b.html",
	} {
		if got := slugFromURL(in); got != want {
			t.Errorf("slugFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// --resume (Task 26): a page captured to the end is not captured again, and a
// page with one run missing is completed without touching the rest.
func TestResumeSkipsDonePages(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	out := t.TempDir()
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--resume"); code != ExitOK {
		t.Fatalf("first run: exit %d, %s", code, stderr)
	}
	if len(m.navs) != 4 {
		t.Fatalf("first run navigations %v; want 4 (2 engines x 2 runs)", m.navs)
	}
	m.navs = nil
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--resume"); code != ExitOK {
		t.Fatalf("resumed run: exit %d, %s", code, stderr)
	}
	if len(m.navs) != 0 {
		t.Errorf("resumed run navigations %v; want none (the page is done)", m.navs)
	}
	// Drop one run: only that capture is repeated.
	slug := "example.com"
	if err := os.Remove(filepath.Join(out, slug, "firefox-b.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, slug, "firefox-b.png")); err != nil {
		t.Fatal(err)
	}
	m.navs = nil
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--resume"); code != ExitOK {
		t.Fatalf("second resumed run: exit %d, %s", code, stderr)
	}
	if len(m.navs) != 1 || !strings.HasPrefix(m.navs[0], "firefox ") {
		t.Errorf("navigations %v; want only firefox's missing run", m.navs)
	}
}

// An unreachable page is skipped by --resume; --retry-failed retries it.
func TestResumeSkipsUnreachableUnlessRetryFailed(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"))
	out := t.TempDir()
	page := schema.PageRef{URL: "https://example.com/", Slug: "example.com"}
	if err := capture.WriteFailure(out, page, "baseline timed out"); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--resume"); code != ExitOK {
		t.Fatalf("resume: exit %d, %s", code, stderr)
	}
	if len(m.navs) != 0 {
		t.Errorf("navigations %v; want none (unreachable, not retried)", m.navs)
	}
	if code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--quiet", "--resume", "--retry-failed"); code != ExitOK {
		t.Fatalf("retry-failed: exit %d, %s", code, stderr)
	}
	if len(m.navs) != 2 {
		t.Errorf("navigations %v; want 2 (the retried page, both runs)", m.navs)
	}
}

// check-fonts (Task 28): one browser per family, a table, and a warning naming
// the fonts that differ.
func TestCheckFonts(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"), blink("chrome"))
	code, stdout, stderr := run(t, m, "check-fonts")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "chromium") || !strings.Contains(stdout, "firefox") {
		t.Errorf("stdout %q; want one line per engine and family", stdout)
	}
	if !strings.Contains(stdout, "warning: font parity") || !strings.Contains(stdout, "DejaVu Sans") {
		t.Errorf("stdout %q; want the parity warning naming the font", stdout)
	}
	// One browser per family: chrome (another Blink) was not launched.
	if strings.Join(m.launched, ",") != "chromium,firefox" {
		t.Errorf("launched %v; want one per family", m.launched)
	}
	if len(m.closed) != len(m.launched) {
		t.Errorf("closed %v, launched %v", m.closed, m.launched)
	}
}

// The job pre-flight warns on stderr, and never fails the job.
func TestJobFontPreflightWarns(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	out := t.TempDir()
	code, stdout, stderr := run(t, m, "https://example.com/", "--out", out, "--runs", "1")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "font parity") {
		t.Errorf("stdout %q; want the font parity warning", stdout)
	}
}

// install/uninstall (Task 37): usage checks and a cache removal that leaves
// nothing behind.
func TestInstallUsage(t *testing.T) {
	m := &fakeMachine{cacheDir: t.TempDir()}
	for _, args := range [][]string{{"install"}, {"install", "chrome"}, {"install", "firefox", "--channel", "nightly"}, {"uninstall"}} {
		code, _, stderr := run(t, m, args...)
		if code != ExitUsage || stderr == "" {
			t.Errorf("%v: exit %d, stderr %q; want usage", args, code, stderr)
		}
	}
}

func TestUninstallRemovesTheCache(t *testing.T) {
	cache := t.TempDir()
	dir := filepath.Join(cache, "firefox", "999.0", "firefox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "firefox"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(cache, "firefox", "999.0", "omnisnap-source.json")
	if err := os.WriteFile(marker, []byte(`{"version":"999.0","channel":"release"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &fakeMachine{cacheDir: cache}
	code, stdout, stderr := run(t, m, "uninstall", "firefox")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "removed") {
		t.Errorf("stdout %q; want a removal notice", stdout)
	}
	for _, p := range []string{"firefox", "downloads"} {
		if _, err := os.Stat(filepath.Join(cache, p)); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", p)
		}
	}
}

// One engine family gets a hint naming the install command.
func TestSingleFamilyHint(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"))
	code, stdout, stderr := run(t, m, "https://example.com/", "--out", t.TempDir(), "--runs", "1")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "omnisnap install firefox") {
		t.Errorf("stdout %q; want the install hint", stdout)
	}
}

// merge (Task 77) end to end: two bundles from different hosts become one
// bundle and one report.
func TestMergeCommand(t *testing.T) {
	vp := schema.Viewport{Width: 1280, Height: 800, DeviceScaleFactor: 1}
	writeBundleFixture := func(host, osName, engine string) string {
		dir := t.TempDir()
		j := schema.Job{Pages: []schema.PageRef{{URL: "https://example.com/", Slug: "home"}}, Viewport: &vp}
		s := schema.NewSummary("dev", j)
		s.Hosts = []schema.Host{{Machine: schema.Machine{OS: osName, Arch: "amd64", ID: host}}}
		s.Results = []schema.Result{{
			Host:     schema.Machine{OS: osName, Arch: "amd64", ID: host},
			Page:     schema.ResultPage{Slug: "home", URL: "https://example.com/"},
			Baseline: schema.ResultBaseline{Engine: "chrome", Version: "157"},
			Engine:   schema.ResultEngine{Engine: engine, Family: schema.FamilyBlink, Version: "157", Profile: schema.ProfileFresh},
			Status:   schema.StatusOK, Warnings: []string{}, Stats: schema.Stats{MatchRate: 1},
			Shots: &schema.Shots{
				Baseline: schema.ShotRef{Src: "home/chrome-a.png", W: 1280, H: 900},
				Engine:   schema.ShotRef{Src: "home/" + engine + "-a.png", W: 1280, H: 900},
			},
			Causes: []schema.Cause{}, Intent: []schema.IntentViolation{},
			Blocked: schema.Sides{Baseline: []string{}, Engine: []string{}},
			Dynamic: schema.Sides{Baseline: []string{}, Engine: []string{}},
		}}
		s.Pages = []schema.PageTotal{{Slug: "home", URL: "https://example.com/"}}
		b, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"chrome-a.png", engine + "-a.png"} {
			if err := os.WriteFile(filepath.Join(dir, "home", f), []byte("png"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	linux := writeBundleFixture("ubuntu", "linux", "firefox")
	mac := writeBundleFixture("macos", "darwin", "webkit")
	out := t.TempDir()
	code, stdout, stderr := run(t, &fakeMachine{}, "merge", linux, mac, "-o", out)
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !strings.Contains(stdout, "merged 2 bundle(s)") {
		t.Errorf("stdout %q", stdout)
	}
	for _, p := range []string{"summary.json", "report.html", "ubuntu/home/chrome-a.png", "macos/home/webkit-a.png"} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Error(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var merged schema.Summary
	if err := json.Unmarshal(b, &merged); err != nil {
		t.Fatal(err)
	}
	if len(merged.Results) != 2 || len(merged.Hosts) != 2 {
		t.Errorf("merged %d results, %d hosts", len(merged.Results), len(merged.Hosts))
	}
	// A missing -o is a usage error.
	if code, _, _ := run(t, &fakeMachine{}, "merge", linux); code != ExitUsage {
		t.Errorf("without -o: exit %d; want usage", code)
	}
}

// --neutral (Task 67): the result records the neutral profile.
func TestNeutralProfile(t *testing.T) {
	m := &fakeMachine{browsers: nil}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	out := t.TempDir()
	code, _, stderr := run(t, m, "https://example.com/", "--out", out, "--runs", "1", "--neutral")
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	s := readSummary(t, out)
	if len(s.Results) != 1 || s.Results[0].Engine.Profile != schema.ProfileNeutral {
		t.Errorf("result profile %+v; want neutral", s.Results[0].Engine.Profile)
	}
	host, err := os.ReadFile(filepath.Join(out, "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(host, []byte(`"profile": "neutral"`)) {
		t.Errorf("host.json does not record the neutral profile: %s", host)
	}
}
