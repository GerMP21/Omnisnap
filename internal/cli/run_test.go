package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if len(s.Skipped) != 1 || s.Skipped[0].Engine != "firefox" {
		t.Errorf("skipped %+v; want firefox (no driver yet)", s.Skipped)
	}
	if len(s.Results) != 1 {
		t.Fatalf("%d results; want 1 (baseline only)", len(s.Results))
	}
	r := s.Results[0]
	if r.Engine.Engine != "chromium" || r.Baseline.Engine != "chromium" || r.Status != schema.StatusOK || r.Shots == nil {
		t.Errorf("result %+v; want chromium alone, ok, with screenshots", r)
	}
	if len(r.Blocked.Engine) != 1 || r.Blocked.Engine[0] != "ads.example.net" {
		t.Errorf("blocked %+v; want the blocked host", r.Blocked)
	}
	slug := r.Page.Slug
	for _, f := range []string{"page.json", "chromium-a.json", "chromium-a.png", "chromium-b.json", "chromium-b.png"} {
		if _, err := os.Stat(filepath.Join(out, slug, f)); err != nil {
			t.Error(err)
		}
	}
	host, err := os.ReadFile(filepath.Join(out, "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "host", host)
	if _, err := os.Stat(filepath.Join(out, "report.html")); err != nil {
		t.Error(err)
	}
	if len(m.launched) != 1 || len(m.closed) != 1 {
		t.Errorf("launched %v, closed %v; want chromium launched once and closed", m.launched, m.closed)
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
