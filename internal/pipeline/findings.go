package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// FindingsFile is <slug>/findings.json: the full evidence of every cause, per
// engine pair (SPEC-bundle-schema.md). summary.json carries the compact form;
// this file is what the viewer and the lab read for details.
type FindingsFile struct {
	Slug  string         `json:"slug"`
	URL   string         `json:"url"`
	Pairs []FindingsPair `json:"pairs"`
}

// FindingsPair is one engine's causes on a page.
type FindingsPair struct {
	Engine   string         `json:"engine"`
	Baseline string         `json:"baseline"`
	Causes   []schema.Cause `json:"causes"`
	Symptoms []Symptom      `json:"symptoms"`
	Warnings []string       `json:"warnings"`
	Dynamic  schema.Sides   `json:"dynamic"`
	Blocked  schema.Sides   `json:"blocked"`
	Stats    schema.Stats   `json:"stats"`
	// Features are the model inputs per cause id (Task 74's dataset rows).
	Features map[string][]float64 `json:"features,omitempty"`
}

// Symptom is overflow another cause already explains.
type Symptom struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind"`
}

// WriteFindings writes <out>/<slug>/findings.json atomically.
func WriteFindings(out, slug, url string, pairs []FindingsPair) (string, error) {
	dir := filepath.Join(out, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("pipeline: %w", err)
	}
	f := FindingsFile{Slug: slug, URL: url, Pairs: pairs}
	if f.Pairs == nil {
		f.Pairs = []FindingsPair{}
	}
	rel := filepath.ToSlash(filepath.Join(slug, "findings.json"))
	if err := fsutil.WriteAtomic(filepath.Join(dir, "findings.json"), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(f)
	}); err != nil {
		return "", err
	}
	return rel, nil
}
