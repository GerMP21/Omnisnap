package groundtruth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/triage"
)

// Task 74: rows and columns equal the feature list, labels travel, and a cause
// without a vector is skipped rather than padded.
func TestDatasetExport(t *testing.T) {
	dir := t.TempDir()
	slug := filepath.Join(dir, "example.com")
	if err := os.MkdirAll(slug, 0o755); err != nil {
		t.Fatal(err)
	}
	feats := make([]float64, len(triage.FeatureNames))
	for i := range feats {
		feats[i] = float64(i) / 10
	}
	doc := map[string]any{
		"slug": "example.com",
		"pairs": []any{map[string]any{
			"engine": "firefox",
			"causes": []any{
				map[string]any{"id": "example.com@firefox|width|div#x"},
				map[string]any{"id": "example.com@firefox|height|p"}, // no vector
			},
			"features": map[string]any{
				"example.com@firefox|width|div#x": feats,
			},
		}},
	}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(slug, "findings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := RowsFromBundle(dir, "crawl", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "example.com@firefox|width|div#x" || rows[0].Label != 0 {
		t.Fatalf("rows = %+v", rows)
	}
	var buf bytes.Buffer
	if err := WriteDataset(&buf, rows); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines", len(lines))
	}
	header := strings.Split(lines[0], ",")
	if len(header) != 6+len(triage.FeatureNames) || header[0] != "site" || header[len(header)-1] != triage.FeatureNames[len(triage.FeatureNames)-1] {
		t.Errorf("header %v", header)
	}
	first := strings.Split(lines[1], ",")
	if first[3] != "crawl" || first[5] != "0" || len(first) != len(header) {
		t.Errorf("row %v", first)
	}
	// Clean pairs count for the false-alarm denominator.
	if n, err := CleanPairs(dir); err != nil || n != 1 {
		t.Errorf("CleanPairs = %d, %v", n, err)
	}
}
