package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

func mergedSummary(t *testing.T) schema.Summary {
	t.Helper()
	vp := schema.Viewport{Width: 1280, Height: 800, DeviceScaleFactor: 1}
	j := schema.Job{Pages: []schema.PageRef{{URL: "https://example.com/", Slug: "home"}}, Viewport: &vp}
	s := schema.NewSummary("1.0.0", j)
	s.Generated = time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	s.Hosts = []schema.Host{
		{Machine: schema.Machine{OS: "linux", Arch: "amd64", ID: "ubuntu"}},
		{Machine: schema.Machine{OS: "darwin", Arch: "arm64", ID: "macos"}},
	}
	mk := func(host, osName, engine string, report int, causes []schema.Cause) schema.Result {
		return schema.Result{
			Host:     schema.Machine{OS: osName, Arch: "amd64", ID: host},
			Page:     schema.ResultPage{Slug: "home", URL: "https://example.com/"},
			Baseline: schema.ResultBaseline{Engine: "chrome", Version: "157"},
			Engine:   schema.ResultEngine{Engine: engine, Family: schema.FamilyBlink, Profile: schema.ProfileFresh},
			Status:   schema.StatusOK, Warnings: []string{}, Stats: schema.Stats{Report: report, Info: 1, MatchRate: 1},
			Causes: causes, Intent: []schema.IntentViolation{},
			Blocked: schema.Sides{Baseline: []string{}, Engine: []string{}},
			Dynamic: schema.Sides{Baseline: []string{}, Engine: []string{}},
			Shots: &schema.Shots{
				Baseline: schema.ShotRef{Src: host + "/home/chrome-a.png", W: 1280, H: 800},
				Engine:   schema.ShotRef{Src: host + "/home/" + engine + "-a.png", W: 1280, H: 800},
			},
		}
	}
	report := []schema.Cause{{
		ID: "x|width|div#target", Tier: schema.TierReport, Title: "Width differs",
		Kinds: []string{"width"}, Consequences: []string{"structural"}, Selector: "div#target",
		Boxes: schema.Boxes{}, Delta: schema.Delta{}, KB: nil, Evidence: schema.Evidence{},
		Count: 1, Instances: []string{"div#target"},
	}}
	info := []schema.Cause{{
		ID: "y|height|p", Tier: schema.TierInfo, Title: "Height differs",
		Kinds: []string{"height"}, Consequences: []string{}, Selector: "p",
		KB: nil, Evidence: schema.Evidence{}, Count: 1, Instances: []string{"p"},
	}}
	s.Results = []schema.Result{
		mk("macos", "darwin", "safari", 1, report),
		mk("ubuntu", "linux", "firefox", 1, append(append([]schema.Cause{}, report...), info...)),
	}
	s.Pages = []schema.PageTotal{{Slug: "home", URL: "https://example.com/", Report: 2, Info: 1}}
	s.Skipped = []schema.Skipped{{Host: "macos", Engine: "webkitgtk", Reason: "not installed"}}
	return s
}

// Task 80: one comment, identified by its hidden marker, with the top report
// causes per page/engine, the notes and the links.
func TestRenderComment(t *testing.T) {
	s := mergedSummary(t)
	md := RenderComment(s, CommentLinks{Report: "https://ci/report", Run: "https://ci/run"})
	for _, want := range []string{
		commentMarker,
		"## Omnisnap: cross-engine layout",
		"hosts: macos, ubuntu",
		"### home",
		"**safari** on macos vs chrome",
		"Width differs",
		"div#target",
		"macos: webkitgtk skipped: not installed",
		"[merged report](https://ci/report)",
		"[workflow run](https://ci/run)",
		"Report only",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("comment missing %q:\n%s", want, md)
		}
	}
	// Info causes are not in the comment.
	if strings.Contains(md, "Height differs") {
		t.Error("an Info cause appears in the PR comment")
	}
	// One marker only: the Action upserts by it.
	if strings.Count(md, commentMarker) != 1 {
		t.Errorf("%d markers", strings.Count(md, commentMarker))
	}
}

func TestRenderCommentTruncates(t *testing.T) {
	s := mergedSummary(t)
	// Many pages and causes: the output exceeds the limit.
	for i := range 400 {
		r := s.Results[1]
		r.Page.Slug = "page-" + strings.Repeat("x", i%7) + "-" + string(rune('a'+i%26)) + "-" + itoa(i)
		s.Results = append(s.Results, r)
	}
	md := RenderComment(s, CommentLinks{Report: "https://ci/report", Limit: 2000})
	if len(md) > 2100 {
		t.Errorf("comment is %d bytes; want <= 2100 with the truncation note", len(md))
	}
	if !strings.Contains(md, "truncated") || !strings.Contains(md, "https://ci/report") {
		t.Errorf("truncated comment has no pointer to the report:\n%s", md[len(md)-200:])
	}
	if !strings.Contains(md, commentMarker) {
		t.Error("truncation removed the marker")
	}
}

// A merged summary with no results still explains itself.
func TestRenderCommentEmpty(t *testing.T) {
	s := mergedSummary(t)
	s.Results = nil
	md := RenderComment(s, CommentLinks{})
	if !strings.Contains(md, "No comparisons") || !strings.Contains(md, commentMarker) {
		t.Errorf("empty comment:\n%s", md)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The Action writes `comment <dir> --report-url ...`; flag.Parse stops at the
// first non-flag, so both orders must work.
func TestRunCommentAcceptsEitherOrder(t *testing.T) {
	dir := t.TempDir()
	b, err := json.Marshal(mergedSummary(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--report-url", "https://ci/report", dir},
		{dir, "--report-url", "https://ci/report"},
	} {
		if code := runComment(args); code != ExitOK {
			t.Errorf("%v: exit %d, want %d", args, code, ExitOK)
		}
	}
}
