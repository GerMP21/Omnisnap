package viewer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
)

func fixture(t *testing.T) schema.Summary {
	t.Helper()
	return loadFixture(t, "../../testdata/viewer/m0.json")
}

func mergedFixture(t *testing.T) schema.Summary {
	t.Helper()
	return loadFixture(t, "../../testdata/viewer/merged.json")
}

func loadFixture(t *testing.T, path string) schema.Summary {
	t.Helper()
	b, err := os.ReadFile(path)
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

func render(t *testing.T, s schema.Summary) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, s); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The data island round-trips the summary, and page text cannot close it.
func TestDataIslandIsEscaped(t *testing.T) {
	s := fixture(t)
	html := render(t, s)
	if strings.Count(html, "</script>") != strings.Count(html, "<script") {
		t.Fatalf("unbalanced script tags: page text closed the data island")
	}
	for _, probe := range []string{"</script><script>window.__pwned=1", "<img src=x onerror", "<svg onload"} {
		if strings.Contains(html, probe) {
			t.Errorf("raw markup %q from the page reached the HTML", probe)
		}
	}
	m := regexp.MustCompile(`(?s)<script type="application/json" id="data">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatal("no data island")
	}
	var back schema.Summary
	if err := json.Unmarshal([]byte(m[1]), &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Results) != len(s.Results) || back.Results[0].Warnings[0] != s.Results[0].Warnings[0] {
		t.Error("data island does not round-trip the summary")
	}
}

// XSS payloads from the data must appear as literal text in the document,
// never as markup or executable script.
func TestXSSMarkersRenderAsText(t *testing.T) {
	s := fixture(t)
	// Ensure the fixture carries recognizable XSS markers as data.
	s.Results[0].Warnings = append(s.Results[0].Warnings, "<script>alert('xss')</script>")
	box := [4]float64{0, 0, 0, 0}
	kb := ""
	s.Results[0].Causes = append(s.Results[0].Causes, schema.Cause{
		ID:           "xss-test",
		Tier:         schema.TierInfo,
		Title:        "XSS probe",
		Kinds:        []string{"probe"},
		Consequences: []string{},
		Selector:     "<img src=x onerror=alert(1)>",
		Boxes:        schema.Boxes{Baseline: &box, Engine: &box},
		Delta:        schema.Delta{},
		KB:           &kb,
		Evidence:     schema.Evidence{},
		Count:        1,
		Instances:    []string{"<img src=x onerror=alert(1)>"},
	})
	s.Results[1].Page.URL = "javascript:window.__pwned=4"

	html := render(t, s)
	// The raw tags must not exist as HTML markup.
	for _, marker := range []string{"<script>alert('xss')</script>", "<img src=x onerror=alert(1)>"} {
		if strings.Contains(html, marker) {
			t.Errorf("XSS marker %q reached the HTML unescaped", marker)
		}
	}
	// The payload text must still be present (inside the escaped data island).
	for _, probe := range []string{"window.__pwned", "alert('xss')", "alert(1)"} {
		if !strings.Contains(html, probe) {
			t.Errorf("payload text %q not present in report", probe)
		}
	}
	// A javascript: URL must never become a link href.
	if strings.Contains(html, `href="javascript`) || strings.Contains(html, `href='javascript`) {
		t.Error("javascript: URL used as a link href")
	}
}

// Offline and file:// safe: nothing loaded from the network, no innerHTML.
func TestSelfContained(t *testing.T) {
	html := render(t, fixture(t))
	for _, bad := range []string{`src="http`, `href="http`, `@import`, `url(http`, "innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(html, bad) {
			t.Errorf("report contains %q", bad)
		}
	}
	if !strings.Contains(html, "<title>Omnisnap report</title>") {
		t.Error("no title")
	}
}

// No inline event handlers and no unsafe attributes in the static template or JS.
func TestNoInlineEventHandlers(t *testing.T) {
	html := render(t, fixture(t))
	// Split around the JSON data island so user strings are not counted as handlers.
	parts := strings.SplitN(html, `<script type="application/json" id="data">`, 2)
	before := parts[0]
	after := ""
	if len(parts) == 2 {
		afterParts := strings.SplitN(parts[1], "</script>", 2)
		if len(afterParts) == 2 {
			after = afterParts[1]
		}
	}
	check := before + after
	for _, bad := range []string{"onclick=", "onerror=", "onload=", "onmouseover="} {
		if strings.Contains(check, bad) {
			t.Errorf("report contains inline event handler %q", bad)
		}
	}
}

// Audit findings are rendered as text, with check ids mapped to readable names.
func TestAuditFindingsRendered(t *testing.T) {
	html := render(t, fixture(t))
	for _, want := range []string{"Audit findings", "horizontal page scroll", "clipped text", "overlapping interactive elements"} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing audit text %q", want)
		}
	}
	// Selectors from audit findings are escaped.
	if strings.Contains(html, "<img src=x onerror") {
		t.Error("audit selector reached HTML unescaped")
	}
}

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, fixture(t)); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "report.html" {
		t.Errorf("dir holds %v; want only report.html (no temp files)", entries)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "report.html")); len(b) == 0 {
		t.Error("empty report")
	}
}

// A 50-page summary renders quickly and stays under 2 MB.
func TestFiftyPageReportSizeAndSpeed(t *testing.T) {
	s := largeSummary(50)
	start := time.Now()
	html := render(t, s)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("render took %s; want < 2s", elapsed)
	}
	if len(html) > 2*1024*1024 {
		t.Errorf("report is %d bytes; want < 2 MB", len(html))
	}
}

// A merged bundle (3 hosts, 2 pages) renders a host selector and groups
// results by page slug, then by host id.
func TestMergedReportHasHostSelectorAndGroups(t *testing.T) {
	s := mergedFixture(t)
	if len(s.Hosts) != 3 {
		t.Fatalf("fixture has %d hosts; want 3", len(s.Hosts))
	}
	if len(s.Results) != 6 {
		t.Fatalf("fixture has %d results; want 6", len(s.Results))
	}
	html := render(t, s)
	// The static template contains the host selector skeleton.
	if !strings.Contains(html, `id="host-selector"`) {
		t.Error("missing host selector container")
	}
	if !strings.Contains(html, `id="host-select"`) {
		t.Error("missing host select")
	}
	// The app builds page and host groups at runtime for merged bundles.
	for _, marker := range []string{"page-group", "host-group", "page-header", "host-header"} {
		if !strings.Contains(html, marker) {
			t.Errorf("report missing grouping marker %q", marker)
		}
	}
	// Result anchors stay stable (r0..rN) so existing links keep working.
	if !strings.Contains(html, "id: anchor(i)") {
		t.Error("result anchors are not generated from the result index")
	}
}

func largeSummary(n int) schema.Summary {
	vp := schema.Viewport{Width: 1280, Height: 800}
	s := schema.NewSummary("1.0.0", schema.Job{
		Pages: []schema.PageRef{
			{URL: "https://example.com/", Slug: "home"},
		},
		Engines:  []string{"chrome", "firefox"},
		Baseline: "chrome",
		Viewport: &vp,
	})
	s.Hosts = []schema.Host{{
		Machine: schema.Machine{OS: "linux", Arch: "amd64", ID: "runner"},
		Engines: []schema.HostEngine{{ID: "chrome", Family: schema.FamilyBlink, Version: "155.0", Profile: schema.ProfileFresh}},
	}}
	s.Pages = make([]schema.PageTotal, n)
	s.Results = make([]schema.Result, n)
	for i := 0; i < n; i++ {
		slug := "page-" + strings.Repeat("x", i%10+1)
		s.Pages[i] = schema.PageTotal{Slug: slug, URL: "https://example.com/" + slug, Report: i % 3, Info: i % 2}
		box := [4]float64{0, 0, 100, 100}
		kb := ""
		s.Results[i] = schema.Result{
			Host:     schema.Machine{OS: "linux", Arch: "amd64", ID: "runner"},
			Page:     schema.ResultPage{Slug: slug, URL: "https://example.com/" + slug},
			Baseline: schema.ResultBaseline{Engine: "chrome", Version: "155.0"},
			Engine:   schema.ResultEngine{Engine: "firefox", Family: schema.FamilyGecko, Version: "153.0", Profile: schema.ProfileFresh},
			Status:   schema.StatusOK,
			Warnings: []string{"dynamic region detected"},
			Stats:    schema.Stats{MatchRate: 0.95, Report: i % 3, Info: i % 2, MS: 500},
			Shots: &schema.Shots{
				Baseline: schema.ShotRef{Src: "shots/" + slug + "-chrome.png", W: 1280, H: 1000},
				Engine:   schema.ShotRef{Src: "shots/" + slug + "-firefox.png", W: 1280, H: 1000},
			},
			Causes: []schema.Cause{
				{
					ID: "cause-" + slug, Tier: schema.TierInfo, Title: "Probe",
					Kinds: []string{"probe"}, Consequences: []string{}, Selector: "main > section",
					Boxes: schema.Boxes{Baseline: &box, Engine: &box},
					Delta: schema.Delta{}, KB: &kb, Evidence: schema.Evidence{}, Count: 1, Instances: []string{"main > section"},
				},
			},
			Audit: &schema.ResultAudit{
				Engine: []schema.AuditFinding{
					{Check: "clipped-text", Selector: "p", Box: [4]float64{0, 0, 100, 20}, Severity: "info"},
				},
			},
		}
	}
	return s
}
