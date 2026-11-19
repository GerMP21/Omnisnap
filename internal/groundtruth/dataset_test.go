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

// writeBundle writes <dir>/<slug>/{findings.json,labels.json}.
func writeBundle(t *testing.T, dir, slug string, warnings []string, causeIDs []string, labels map[string]any) {
	t.Helper()
	page := filepath.Join(dir, slug)
	if err := os.MkdirAll(page, 0o755); err != nil {
		t.Fatal(err)
	}
	feats := map[string]any{}
	causes := make([]any, 0, len(causeIDs))
	for _, id := range causeIDs {
		causes = append(causes, map[string]any{"id": id})
		v := make([]float64, len(triage.FeatureNames))
		feats[id] = v
	}
	doc := map[string]any{
		"slug": slug,
		"pairs": []any{map[string]any{
			"engine": "firefox", "causes": causes, "features": feats, "warnings": warnings,
		}},
	}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(page, "findings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if labels != nil {
		lb, _ := json.Marshal(labels)
		if err := os.WriteFile(filepath.Join(page, "labels.json"), lb, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A mutant contributes its positive causes only (labels.json), and the
// counters match the spike's metadata.
func TestDatasetMutantPositives(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "p~firefox~0", nil,
		[]string{"p@firefox|width|div#a", "p@firefox|position|div#b"},
		map[string]any{"op": "width-shrink", "effective": true,
			"causes": []string{"p@firefox|width|div#a"}})
	rows, stats, err := RowsFromBundleWithStats(dir, "mut", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Label != 1 || rows[0].ID != "p@firefox|width|div#a" {
		t.Fatalf("rows = %+v", rows)
	}
	if stats.Mutants != 1 || stats.Effective != 1 || stats.Pos != 1 || stats.Neg != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

// An ineffective mutant has no positives and contributes no rows, but it is
// counted (Task 71: a recorded hard negative, not a silent drop).
func TestDatasetIneffectiveMutant(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "p~firefox~1", nil, []string{"p@firefox|width|div#a"},
		map[string]any{"op": "border", "effective": false, "causes": []string{}})
	rows, stats, err := RowsFromBundleWithStats(dir, "mut", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v; want none", rows)
	}
	if stats.Mutants != 1 || stats.Effective != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

// A page whose engines rendered different states is skipped, like dataset.mjs.
func TestDatasetSkipsDivergedPages(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "diverged.com",
		[]string{"engines rendered different page states: 100 vs 130 elements, page height 900 vs 1200px"},
		[]string{"diverged.com@firefox|width|div#a"}, nil)
	writeBundle(t, dir, "kept.com", nil, []string{"kept.com@firefox|width|div#a"}, nil)
	rows, stats, err := RowsFromBundleWithStats(dir, "crawl", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Site != "kept.com" {
		t.Fatalf("rows = %+v", rows)
	}
	if stats.SkippedDiverged != 1 || stats.Neg != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

// Without labels.json an older mutant bundle still exports with the uniform
// label.
func TestDatasetMutantWithoutLabels(t *testing.T) {
	dir := t.TempDir()
	writeBundle(t, dir, "p~firefox~2", nil, []string{"p@firefox|width|div#a"}, nil)
	rows, _, err := RowsFromBundleWithStats(dir, "mut", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Label != 1 {
		t.Errorf("rows = %+v", rows)
	}
}

// The novelty column is recomputed from the corpus table, because a findings
// file written before the first crawl finished stores a zero there.
func TestDatasetNoveltyFromTable(t *testing.T) {
	feats := make([]float64, len(triage.FeatureNames))
	idx := triage.FeatureIndex("novelty_other_pages")
	table, err := triage.LoadTable([]byte(`{"total":100,"fingerprints":{"fp-a":{"n":50,"pages":40}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := patchNovelty(feats, "fp-a", "p", table)
	if got[idx] != 39 {
		t.Errorf("novelty = %v; want 39 other pages", got[idx])
	}
	if feats[idx] != 0 {
		t.Error("patchNovelty wrote into the caller's slice")
	}
	// A cause with no fingerprint, or no table, keeps the stored value.
	if got := patchNovelty(feats, "", "p", table); got[idx] != 0 {
		t.Errorf("no fingerprint: novelty = %v; want 0", got[idx])
	}
	feats[idx] = 5
	if got := patchNovelty(feats, "fp-a", "p", nil); got[idx] != 5 {
		t.Errorf("no table: novelty = %v; want the stored 5", got[idx])
	}
}
