package viewer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

func fixture(t *testing.T) schema.Summary {
	t.Helper()
	b, err := os.ReadFile("../../testdata/viewer/m0.json")
	if err != nil {
		t.Fatal(err)
	}
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
