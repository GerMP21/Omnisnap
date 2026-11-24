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
	kb := "K9"
	report := []schema.Cause{{
		ID: "x|width|div#target", Tier: schema.TierReport, Title: "Width differs",
		Kinds: []string{"width"}, Consequences: []string{"structural"}, Selector: "div#target",
		Boxes: schema.Boxes{Baseline: &[4]float64{8, 40, 300, 20}}, Delta: schema.Delta{DW: 12}, KB: &kb,
		KBDetail: &schema.KBEntry{ID: "K9", Title: "Width differs", Explanation: "the table is wider", Fix: "table-layout: fixed, or <wbr> breaks"},
		Evidence: schema.Evidence{}, Count: 2, Instances: []string{"div#target", "div#second"},
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

// Task 80: one comment, identified by its hidden marker. It opens with a
// verdict and a row per page, lists each Report cause once with every browser
// that shows it and where it is, says what could not be checked, and links the
// report.
func TestRenderComment(t *testing.T) {
	s := mergedSummary(t)
	md := RenderComment(s, CommentLinks{Report: "https://ci/report", Run: "https://ci/run"})
	for _, want := range []string{
		commentMarker,
		"## Omnisnap: cross-browser layout check",
		"> [!WARNING]",
		"**1 layout issue on 1 of 1 page checked.**",
		"compared with Chrome on the same machine",
		"| [home](https://example.com/) | ⚠️ 1 issue in Safari on macOS, Firefox on Linux |",
		"### home",
		"**1. Width differs** at `div#target`",
		"**Where:** about 40 px from the top of the page",
		"- **Seen in** (compared with Chrome):\n  - Safari on macOS, Firefox on Linux: 12 px wider\n",
		"**What a visitor sees:** the surrounding layout shifts",
		"**Why:** the table is wider",
		"**Suggested fix:** table-layout: fixed, or &lt;wbr&gt; breaks",
		"**Same cause also affects:** `div#second`",
		"### Not checked",
		"| WebKitGTK on macOS | all pages | skipped: not installed |",
		"[Open the full report](https://ci/report)",
		"[workflow run](https://ci/run)",
		"Report only",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("comment missing %q:\n%s", want, md)
		}
	}
	// The issue is listed once, not once per browser.
	if n := strings.Count(md, "Width differs"); n != 1 {
		t.Errorf("the issue appears %d times; want once", n)
	}
	// Info causes are counted, not listed.
	if strings.Contains(md, "Height differs") {
		t.Error("an Info cause appears in the PR comment")
	}
	if !strings.Contains(md, "also found 2 differences on this page that are normal browser behaviour") {
		t.Errorf("the Info count is missing:\n%s", md)
	}
	// Internal vocabulary stays in the report.
	for _, jargon := range []string{"comparison(s)", " info", "report cause", "(width)"} {
		if strings.Contains(md, jargon) {
			t.Errorf("comment uses internal wording %q", jargon)
		}
	}
	// One marker only: the Action upserts by it.
	if strings.Count(md, commentMarker) != 1 {
		t.Errorf("%d markers", strings.Count(md, commentMarker))
	}
}

// A failed capture is named in Not checked, and the page row says so (the
// e2e workflows grep for "| capture failed |").
func TestRenderCommentNotChecked(t *testing.T) {
	s := mergedSummary(t)
	s.Results[0].Status = schema.StatusFailed
	s.Results[1].Status = schema.StatusUnreachable
	md := RenderComment(s, CommentLinks{})
	for _, want := range []string{
		"> [!CAUTION]",
		"**Nothing was compared.**",
		"| [home](https://example.com/) | ❌ not checked |",
		"| Safari on macOS | home | capture failed |",
		"| Firefox on Linux | home | page could not be loaded (unreachable, or a bot check) |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("comment missing %q:\n%s", want, md)
		}
	}
}

// No Report cause anywhere: one clear line, not an empty comment.
func TestRenderCommentClean(t *testing.T) {
	s := mergedSummary(t)
	for i := range s.Results {
		s.Results[i].Causes = nil
	}
	md := RenderComment(s, CommentLinks{})
	for _, want := range []string{"> [!TIP]", "**No layout issues found** on 1 page in 2 browsers.", "✅ no issues in 2 browsers"} {
		if !strings.Contains(md, want) {
			t.Errorf("comment missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "### home") {
		t.Error("a page with no issues gets its own section")
	}
}

// A page whose engines loaded different states is flagged as unreliable.
func TestRenderCommentDiverged(t *testing.T) {
	s := mergedSummary(t)
	s.Results[0].Status = schema.StatusDiverged
	md := RenderComment(s, CommentLinks{})
	for _, want := range []string{"results may be unreliable", "Safari on macOS loaded a different version of this page"} {
		if !strings.Contains(md, want) {
			t.Errorf("comment missing %q:\n%s", want, md)
		}
	}
}

func TestDescribeDelta(t *testing.T) {
	for _, c := range []struct {
		d    schema.Delta
		want string
	}{
		{schema.Delta{DW: 12}, "12 px wider"},
		{schema.Delta{DW: -3.4, DH: 40}, "3.4 px narrower, 40 px taller"},
		{schema.Delta{DLn: 2}, "2 more lines"},
		{schema.Delta{DLn: -1, DX: 4, DY: -20}, "1 fewer line, moved 4 px right and 20 px up"},
		{schema.Delta{DW: 0.2}, ""},
	} {
		if got := describeDelta(c.d); got != c.want {
			t.Errorf("describeDelta(%+v) = %q, want %q", c.d, got, c.want)
		}
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
	if !strings.Contains(md, "Nothing was compared") || !strings.Contains(md, commentMarker) {
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
