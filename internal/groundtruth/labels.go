package groundtruth

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// LabelFile is <mutant>/labels.json: what the injected bug did, in both forms
// a consumer needs. Positives are selectors for people; PositiveUIDs are the
// clean capture's node ids, which is the exact join the dataset uses (because
// selectors drift when a hidden or reordered sibling changes :nth-of-type).
// See SPEC-ground-truth-mutant-policy.md.
type LabelFile struct {
	Op           string   `json:"op"`
	Target       string   `json:"target"`
	Scope        int      `json:"scope"`
	Effective    bool     `json:"effective"`
	Changed      int      `json:"changed"`
	Positives    []string `json:"positives"`
	PositiveUIDs []int    `json:"positiveUids"`
	Attempts     int      `json:"attempts,omitempty"` // set only with --require-effective
}

// WriteLabels writes one mutant's labels.json atomically.
func WriteLabels(dir string, m Mutant, mutation *schema.Mutation, label MutantLabel) error {
	doc := LabelFile{
		Target: label.Target, Scope: label.Scope, Effective: label.Effective,
		Changed: label.Changed, Positives: label.Positives,
	}
	if mutation != nil {
		doc.Op, doc.Attempts = mutation.Op, mutation.Attempts
	}
	// Map iteration is random; the file is a build artifact and must be
	// reproducible.
	for uid := range label.PositiveUIDs {
		doc.PositiveUIDs = append(doc.PositiveUIDs, uid)
	}
	slices.Sort(doc.PositiveUIDs)
	if doc.Positives == nil {
		doc.Positives = []string{}
	}
	if doc.PositiveUIDs == nil {
		doc.PositiveUIDs = []int{}
	}
	path := filepath.Join(dir, m.Slug, "labels.json")
	if err := fsutil.WriteAtomic(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	}); err != nil {
		return fmt.Errorf("groundtruth: labels: %w", err)
	}
	return nil
}
