package groundtruth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/triage"
)

// Dataset export (Task 74): training rows from the lab's findings.json files.
// A row's label is 1 for an injected bug's cause, 0 for a natural cause of a
// clean page, -1 for planted pages (an independent test, not training data).

// DatasetRow is one training row.
type DatasetRow struct {
	Site, Page, Pair, Group, ID string
	Label                       int
	Features                    []float64
}

// DatasetMeta is what the trainer needs besides the rows. The counters mirror
// the spike's dataset.mjs metadata.
type DatasetMeta struct {
	CleanPairs      int `json:"cleanPairs"`
	Pos             int `json:"pos"`
	Neg             int `json:"neg"`
	Mutants         int `json:"mutants"`
	Effective       int `json:"effective"`
	SkippedDiverged int `json:"skippedDiverged"`
}

// BundleStats is what one bundle contributed.
type BundleStats struct {
	Pos, Neg, Mutants, Effective, SkippedDiverged int
}

// findingsFile mirrors the pipeline's findings.json for the fields the export
// reads.
type findingsFile struct {
	Slug  string `json:"slug"`
	Pairs []struct {
		Engine string `json:"engine"`
		Causes []struct {
			ID          string `json:"id"`
			Fingerprint string `json:"fingerprint"`
		} `json:"causes"`
		Features map[string][]float64 `json:"features"`
		Warnings []string             `json:"warnings"`
	} `json:"pairs"`
}

// RowsFromBundle reads every <bundle>/<slug>/findings.json. group is recorded
// as the row group ("planted", "crawl", "mut", ...); label is the row label.
func RowsFromBundle(bundleDir, group string, label int) ([]DatasetRow, error) {
	rows, _, err := RowsFromBundleWithStats(bundleDir, group, label)
	return rows, err
}

// RowsFromBundleWithStats is RowsFromBundle plus the counters the trainer's
// metadata needs.
//
// Two behaviours follow the spike's dataset.mjs: a page whose engines rendered
// different states is skipped (its causes would be labelled as bugs they are
// not), and a mutant contributes only its positive causes, read from
// labels.json (Task 71). A mutant without labels.json falls back to the
// uniform label, so older bundles still export.
func RowsFromBundleWithStats(bundleDir, group string, label int) ([]DatasetRow, BundleStats, error) {
	var stats BundleStats
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return nil, stats, fmt.Errorf("groundtruth: dataset: %w", err)
	}
	var rows []DatasetRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(bundleDir, e.Name(), "findings.json"))
		if err != nil {
			continue // not a page directory
		}
		var doc findingsFile
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, stats, fmt.Errorf("groundtruth: %s/findings.json: %w", e.Name(), err)
		}
		diverged := false
		for _, pair := range doc.Pairs {
			for _, w := range pair.Warnings {
				if strings.Contains(w, "different page states") {
					diverged = true
				}
			}
		}
		if diverged {
			stats.SkippedDiverged++
			continue
		}
		positives := map[string]bool{}
		hasLabels := false
		if group == "mut" {
			if lab, err := readLabels(filepath.Join(bundleDir, e.Name(), "labels.json")); err == nil && lab.Causes != nil {
				hasLabels = true
				stats.Mutants++
				if lab.Effective {
					stats.Effective++
				}
				for _, id := range lab.Causes {
					positives[id] = true
				}
			}
		}
		for _, pair := range doc.Pairs {
			for _, c := range pair.Causes {
				rowLabel := label
				if group == "mut" && hasLabels {
					if !positives[c.ID] {
						continue // a mutant contributes its positives only
					}
					rowLabel = 1
				}
				feats := pair.Features[c.ID]
				if len(feats) != len(triage.FeatureNames) {
					continue // no vector for this cause: skip, do not pad
				}
				feats = patchNovelty(feats, c.Fingerprint, e.Name(), triage.ShippedTable())
				rows = append(rows, DatasetRow{
					Site: e.Name(), Page: e.Name(), Pair: pair.Engine, Group: group,
					ID: c.ID, Label: rowLabel, Features: feats,
				})
				if rowLabel == 1 {
					stats.Pos++
				} else {
					stats.Neg++
				}
			}
		}
	}
	return rows, stats, nil
}

// readLabels reads a mutant's labels.json (Task 71).
func readLabels(path string) (LabelFile, error) {
	var lab LabelFile
	b, err := os.ReadFile(path)
	if err != nil {
		return lab, err
	}
	if err := json.Unmarshal(b, &lab); err != nil {
		return lab, err
	}
	return lab, nil
}

// CleanPairs counts the compared engine pairs in a bundle: the denominator of
// "Report causes per clean page pair".
func CleanPairs(bundleDir string) (int, error) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return 0, nil
	}
	pairs := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(bundleDir, e.Name(), "findings.json"))
		if err != nil {
			continue
		}
		var doc findingsFile
		if err := json.Unmarshal(b, &doc); err != nil {
			continue
		}
		pairs += len(doc.Pairs)
	}
	return pairs, nil
}

// WriteDataset writes the CSV the trainer reads: the identity columns, the
// label, then the 82 features in FeatureNames order.
func WriteDataset(w io.Writer, rows []DatasetRow) error {
	header := append([]string{"site", "page", "pair", "group", "id", "label"}, triage.FeatureNames...)
	if _, err := io.WriteString(w, strings.Join(header, ",")+"\n"); err != nil {
		return err
	}
	for _, r := range rows {
		if len(r.Features) != len(triage.FeatureNames) {
			return fmt.Errorf("groundtruth: dataset: row %s has %d features", r.ID, len(r.Features))
		}
		fields := []string{csvField(r.Site), csvField(r.Page), csvField(r.Pair), csvField(r.Group), csvField(r.ID), strconv.Itoa(r.Label)}
		for _, f := range r.Features {
			fields = append(fields, strconv.FormatFloat(f, 'g', -1, 64))
		}
		if _, err := io.WriteString(w, strings.Join(fields, ",")+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// WriteDatasetMeta writes dataset-meta.json.
func WriteDatasetMeta(w io.Writer, meta DatasetMeta) error {
	return json.NewEncoder(w).Encode(meta)
}

func csvField(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// patchNovelty fills the novelty column from the corpus table.
//
// Novelty is cross-corpus, so the table only exists after a crawl finishes: a
// findings file written before it stores a zero there. Recomputing the column
// at export time makes the row carry the number inference will compute later,
// whatever order the pipeline ran in.
func patchNovelty(feats []float64, fingerprint, slug string, table *triage.Table) []float64 {
	idx := triage.FeatureIndex("novelty_other_pages")
	if idx < 0 || idx >= len(feats) || fingerprint == "" || table == nil {
		return feats
	}
	out := append([]float64(nil), feats...)
	out[idx] = 0
	if n := table.Surprise(fingerprint, slug); n != nil {
		out[idx] = float64(n.OtherPages)
	}
	return out
}
