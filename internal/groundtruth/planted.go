// Package groundtruth holds the known answers behind every precision and
// recall claim (SPEC-ground-truth.md): the planted suite, and later mutation
// testing and the training dataset.
package groundtruth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Kind says how a planted-suite page is scored.
type Kind string

// Page kinds, as in the spike's planted/pages.mjs.
const (
	KindPlanted Kind = "planted" // one planted divergence in Firefox: counts toward recall
	KindControl Kind = "control" // no divergence: must produce no Report cause
	KindNative  Kind = "native"  // genuine form-control differences, only on OnlyTags
	KindKB      Kind = "kb"      // a knowledge-base fixture: differences only as explained Info
)

// ExpectGroup is satisfied when a finding sits on an element whose id is in AnyOf.
type ExpectGroup struct {
	AnyOf []string `json:"anyOf"`
}

// Page is one planted-suite page's answer (testdata/planted/expectations.json,
// exported by spikes/layout-diff/tools/export-planted.mjs). Fields are absent
// where the spike leaves them out; MaxFindings keeps "absent" apart from 0.
type Page struct {
	Slug        string        `json:"slug"`
	Kind        Kind          `json:"kind"`
	Expect      []ExpectGroup `json:"expect,omitempty"`
	MaxFindings *int          `json:"maxFindings,omitempty"`
	OnlyTags    []string      `json:"onlyTags,omitempty"`
	KB          string        `json:"kb,omitempty"`
	Intent      bool          `json:"intent,omitempty"`
}

// LoadPlanted reads expectations.json from dir and checks that every page's
// HTML file exists. Unknown fields are an error: they mean the spike's answer
// format changed and the scoring here would silently ignore part of it.
func LoadPlanted(dir string) ([]Page, error) {
	b, err := os.ReadFile(filepath.Join(dir, "expectations.json"))
	if err != nil {
		return nil, fmt.Errorf("groundtruth: %w", err)
	}
	var doc struct {
		Source string `json:"source"`
		Pages  []Page `json:"pages"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("groundtruth: expectations.json: %w", err)
	}
	seen := map[string]bool{}
	for _, p := range doc.Pages {
		switch {
		case p.Slug == "" || filepath.Base(p.Slug) != p.Slug:
			return nil, fmt.Errorf("groundtruth: invalid slug %q", p.Slug)
		case seen[p.Slug]:
			return nil, fmt.Errorf("groundtruth: duplicate slug %q", p.Slug)
		}
		switch p.Kind {
		case KindPlanted, KindControl, KindNative, KindKB:
		default:
			return nil, fmt.Errorf("groundtruth: %s: unknown kind %q", p.Slug, p.Kind)
		}
		if _, err := os.Stat(filepath.Join(dir, p.Slug+".html")); err != nil {
			return nil, fmt.Errorf("groundtruth: %s: %w", p.Slug, err)
		}
		seen[p.Slug] = true
	}
	return doc.Pages, nil
}
