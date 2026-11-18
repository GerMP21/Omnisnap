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

// DatasetMeta is what the trainer needs besides the rows.
type DatasetMeta struct {
	CleanPairs int `json:"cleanPairs"`
}

// findingsFile mirrors the pipeline's findings.json for the fields the export
// reads.
type findingsFile struct {
	Slug  string `json:"slug"`
	Pairs []struct {
		Engine   string                `json:"engine"`
		Causes   []struct{ ID string } `json:"causes"`
		Features map[string][]float64  `json:"features"`
	} `json:"pairs"`
}

// RowsFromBundle reads every <bundle>/<slug>/findings.json. group is recorded
// as the row group ("planted", "crawl", "mut", ...); label is the row label.
func RowsFromBundle(bundleDir, group string, label int) ([]DatasetRow, error) {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return nil, fmt.Errorf("groundtruth: dataset: %w", err)
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
			return nil, fmt.Errorf("groundtruth: %s/findings.json: %w", e.Name(), err)
		}
		for _, pair := range doc.Pairs {
			for _, c := range pair.Causes {
				feats := pair.Features[c.ID]
				if len(feats) != len(triage.FeatureNames) {
					continue // no vector for this cause: skip, do not pad
				}
				rows = append(rows, DatasetRow{
					Site: e.Name(), Page: e.Name(), Pair: pair.Engine, Group: group,
					ID: c.ID, Label: label, Features: feats,
				})
			}
		}
	}
	return rows, nil
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
