// Package triage turns layout-diff causes into the findings a developer reads
// (SPEC-triage.md): a knowledge-base explanation when one fits, the visible
// consequences, a tier (Report or Info), and one ranked cause per mechanism or
// structural signature. It is a port of the spike's lib/triage.mjs.
package triage

import (
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
)

// Context is what triage needs beyond one cause.
type Context struct {
	Engines struct{ A, B string }
	D       map[int]layoutdiff.Delta
	VWA     int
	VWB     int
	A       *layoutdiff.Tree
	B       *layoutdiff.Tree
	// Evidence is the probe context for the mechanism diagnosis; nil when
	// probes were not captured.
	Evidence *evidence.Context
	Thirds   []layoutdiff.Third
	M        *layoutdiff.Match
}

// Options are the extra inputs of Build.
type Options struct {
	Evidence *evidence.Context
	Thirds   []layoutdiff.Third
	M        *layoutdiff.Match
	// Ink is the ink evidence per A uid (Task 49); nil without screenshots.
	Ink map[int]*InkFeature
	// IntentVerdicts maps an A uid to B-violates / A-violates / none, and
	// HasThirds says whether a third family was available at all (Task 56).
	IntentVerdicts map[int]string
	HasThirds      bool
	// Calibration is the page's systematic-engine model (Task 55).
	Calibration *evidence.Calibration
}

// KB is a knowledge-base entry that explains a cause.
type KB struct {
	ID          string
	Title       string
	Explanation string
	Fix         string
	Expected    bool // an expected engine difference
}

// Assessment is a cause's triage.
type Assessment struct {
	KB           *KB
	Consequences []string
	Tier         schema.Tier
	Weight       float64
	Visibility   float64
}

// Finding is one cause after triage, before grouping.
type Finding struct {
	Key           string // grouping key: "kb:<id>" or a structural signature
	ID            string
	Kinds         []string
	Sel           string
	Side          string // "A" or "B"
	Tag           string
	ElID          string
	Text          string
	A             *[4]float64 // baseline box (nil on the side it is missing from)
	B             *[4]float64
	D             layoutdiff.Delta
	Y             float64
	Mechanism     *evidence.Verdict
	Consensus     string
	Perception    *float64
	Ink           *InkFeature
	BlockedBy     []string
	Calibration   *evidence.Explained
	Severity      int
	Events        []string
	IntentVerdict string
	Score         *float64
	Assessment
}
