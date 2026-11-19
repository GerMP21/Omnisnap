package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage"
)

// FindingsFile is <slug>/findings.json: the full evidence of every cause, per
// engine pair (SPEC-bundle-schema.md). summary.json carries the compact form;
// this file is what the viewer and the lab read for details.
type FindingsFile struct {
	Slug  string         `json:"slug"`
	URL   string         `json:"url"`
	Pairs []FindingsPair `json:"pairs"`
}

// FindingsCause is a cause plus where it came from: side "A" (baseline) or
// "B" (compared engine), and the node's uid in that side's capture. The
// fields exist only here: summary.json keeps the published schema, and the
// mutant lab joins labels by identity instead of by selector
// (SPEC-ground-truth-mutant-policy.md).
type FindingsCause struct {
	schema.Cause
	Node *int   `json:"nodeId,omitempty"`
	Side string `json:"side,omitempty"`
	// Fingerprint is the cause's class across pages (Task 56): the corpus
	// table counts the pages that show it.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// MarshalJSON emits the summary form of the cause plus the identity fields.
// Cause has its own MarshalJSON (empty slices, not null), and a method on an
// embedded field is promoted, so encoding the struct directly would silently
// drop nodeId and side.
func (f FindingsCause) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(f.Cause)
	if err != nil {
		return nil, err
	}
	if f.Node == nil && f.Side == "" && f.Fingerprint == "" {
		return base, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	if f.Node != nil {
		b, err := json.Marshal(*f.Node)
		if err != nil {
			return nil, err
		}
		m["nodeId"] = b
	}
	if f.Side != "" {
		b, err := json.Marshal(f.Side)
		if err != nil {
			return nil, err
		}
		m["side"] = b
	}
	if f.Fingerprint != "" {
		b, err := json.Marshal(f.Fingerprint)
		if err != nil {
			return nil, err
		}
		m["fingerprint"] = b
	}
	return json.Marshal(m)
}

// FindingsPair is one engine's causes on a page.
type FindingsPair struct {
	Engine   string          `json:"engine"`
	Baseline string          `json:"baseline"`
	Causes   []FindingsCause `json:"causes"`
	Symptoms []Symptom       `json:"symptoms"`
	Warnings []string        `json:"warnings"`
	Dynamic  schema.Sides    `json:"dynamic"`
	Blocked  schema.Sides    `json:"blocked"`
	Stats    schema.Stats    `json:"stats"`
	// Features are the model inputs per cause id (Task 74's dataset rows).
	Features map[string][]float64 `json:"features,omitempty"`
}

// Symptom is overflow another cause already explains.
type Symptom struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind"`
}

// CausesWithRefs attaches each cause's identity to the findings form.
func CausesWithRefs(causes []schema.Cause, refs map[string]triage.CauseRef) []FindingsCause {
	out := make([]FindingsCause, 0, len(causes))
	for _, c := range causes {
		fc := FindingsCause{Cause: c}
		if ref, ok := refs[c.ID]; ok {
			node := ref.Node
			fc.Node, fc.Side, fc.Fingerprint = &node, ref.Side, ref.Fingerprint
		}
		out = append(out, fc)
	}
	return out
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
