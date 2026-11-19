package triage

import (
	"fmt"
	"math"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
)

// Feature vector of one CAUSE for the tree model (Task 62): exactly the
// spike's lib/features.mjs, names and order included. One function serves
// training rows and inference, so they can never drift. Every value is finite:
// a missing measurement is -1, with the evidence's presence as its own
// feature.
//
// Deliberately NOT features: anything that names the page, the site or the
// mutation, and the judge's P (derived from the same evidence).

// FeatureKinds are the classed cause kinds.
var FeatureKinds = []string{"presence", "hidden", "truncated", "intent", "wrap", "width", "height", "position", "children-moved", "overflow-x", "overflow-y", "text-shifted", "columns"}

// FeatureConsequences are the consequence one-hots.
var FeatureConsequences = []string{"presence", "visibility", "clipped", "scrollbar", "overflow", "protrusion", "truncation", "line-count", "interactive-moved", "structural", "intent-broken", "content-moved"}

// FeatureKB are the knowledge-base entries.
var FeatureKB = []string{"K1", "K2", "K4", "K5", "K8"}

// FeatureMechanisms are the mechanism verdicts.
var FeatureMechanisms = []string{"lh", "font", "controls", "all", "layout", "author-font"}

// FeatureEvents are the perceptual events.
var FeatureEvents = []string{"overlap-created", "alignment-broken", "order-changed", "protrudes-parent", "spacing-changed", "size-noticeable"}

// FeatureConsensus are the consensus verdicts.
var FeatureConsensus = []string{"B-odd", "A-odd", "all-differ"}

// FeatureRoles are the element roles.
var FeatureRoles = []string{"control", "link", "heading", "media", "text", "container"}

// FeatureEngines are the compared engines the model was trained on.
var FeatureEngines = []string{"firefox", "webkit"}

// FeatureNames is the model's input order (82 values).
var FeatureNames = buildFeatureNames()

// FeatureIndex is the position of a feature in FeatureNames, or -1. The
// novelty column is recomputed at export time (see internal/groundtruth), so
// both sides agree on where it lives.
func FeatureIndex(name string) int {
	for i, n := range FeatureNames {
		if n == name {
			return i
		}
	}
	return -1
}

func buildFeatureNames() []string {
	out := []string{
		"count_log", "side_b", "dw_abs", "dh_abs", "dx_abs", "dy_abs", "dln", "rel_w", "rel_h", "area_log", "top_log", "first_viewport",
		"severity", "impact", "rules_report", "text_len", "kb_expected", "perception_score", "calibrated", "ink_present", "ink_agreement", "ink_floor",
		"ink_below_floor", "ink_amount_log", "intent_b", "intent_a", "novelty_other_pages", "probes_present", "third_engine",
	}
	for _, k := range FeatureKinds {
		out = append(out, "kind_"+k)
	}
	for _, c := range FeatureConsequences {
		out = append(out, "cons_"+c)
	}
	for _, k := range FeatureKB {
		out = append(out, "kb_"+k)
	}
	for _, m := range FeatureMechanisms {
		out = append(out, "mech_"+m)
	}
	for _, e := range FeatureEvents {
		out = append(out, "event_"+e)
	}
	for _, c := range FeatureConsensus {
		out = append(out, "consensus_"+c)
	}
	for _, r := range FeatureRoles {
		out = append(out, "role_"+r)
	}
	for _, e := range FeatureEngines {
		out = append(out, "engine_"+e)
	}
	return out
}

// InkFeature is the ink evidence one cause carries.
type InkFeature struct {
	Agreement float64
	Floor     float64
	Ink       int
}

// FeatureInput is everything the vector needs, flat so the training rows and
// inference cannot diverge.
type FeatureInput struct {
	Count         int
	Side          string
	D             layoutdiff.Delta
	Box           *[4]float64 // the baseline box, else the engine's
	Severity      int
	Impact        float64
	RulesTier     string // "report", "info", or "" when only Tier is known
	Tier          string
	Text          string
	KBExpected    bool
	KBID          string
	Perception    *float64
	Events        []string
	Calibration   bool
	Ink           *InkFeature
	IntentVerdict string // B-violates, A-violates, none, unknown
	NoveltyOthers *int
	Mechanism     string // "" or unknown when absent
	Consensus     string
	Kinds         []string
	Consequences  []string
	Tag           string
	Engine        string // the compared engine (firefox, webkit, ...)
}

// roleOf classifies an element for the role one-hots.
func roleOf(tag, text string) string {
	switch tag {
	case "input", "select", "textarea", "button":
		return "control"
	case "a":
		return "link"
	}
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return "heading"
	}
	switch tag {
	case "img", "svg", "video", "canvas", "picture", "iframe":
		return "media"
	}
	if text != "" {
		return "text"
	}
	return "container"
}

// kindClass maps a cause kind to its feature class.
func kindClass(k string) string {
	switch {
	case strings.HasPrefix(k, "only-in-") || k == "children-differ":
		return "presence"
	case strings.HasPrefix(k, "hidden-in-"):
		return "hidden"
	case strings.HasPrefix(k, "truncated-in-"):
		return "truncated"
	case strings.HasPrefix(k, "intent-"):
		return "intent"
	}
	return k
}

// Features computes the vector; it returns an error if the length is wrong
// (a feature was added in one place only).
func Features(in FeatureInput) ([]float64, error) {
	box := [4]float64{0, 0, 0, 0}
	if in.Box != nil {
		box = *in.Box
	}
	kinds := map[string]bool{}
	for _, k := range in.Kinds {
		kinds[kindClass(k)] = true
	}
	cons := map[string]bool{}
	for _, c := range in.Consequences {
		cons[c] = true
	}
	events := map[string]bool{}
	for _, e := range in.Events {
		events[e] = true
	}
	w := math.Max(1, box[2])
	h := math.Max(1, box[3])
	abs := math.Abs
	v := []float64{
		log1p(math.Max(1, float64(in.Count))),
		b1(in.Side == "B"),
		abs(in.D.DW), abs(in.D.DH), abs(in.D.DX), abs(in.D.DY), float64(in.D.DLN),
		abs(in.D.DW) / w, abs(in.D.DH) / h,
		log1p(w * h), log1p(box[1]), b1(box[1] < 800),
		severityOr1(in.Severity), in.Impact,
		b1(in.RulesTier == "report" || (in.RulesTier == "" && in.Tier == "report")),
		float64(len(in.Text)), b1(in.KBExpected),
		numOr(in.Perception, -1),
		b1(in.Calibration),
	}
	if in.Ink != nil {
		v = append(v, 1, in.Ink.Agreement, in.Ink.Floor, b1(in.Ink.Agreement < in.Ink.Floor), log1p(float64(in.Ink.Ink)))
	} else {
		v = append(v, 0, -1, -1, -1, -1)
	}
	v = append(v,
		b1(in.IntentVerdict == "B-violates"),
		b1(in.IntentVerdict == "A-violates"),
	)
	if in.NoveltyOthers != nil {
		v = append(v, float64(*in.NoveltyOthers))
	} else {
		v = append(v, -1)
	}
	v = append(v,
		b1(in.Mechanism != "" && in.Mechanism != "unknown"),
		b1(in.Consensus != "" && in.Consensus != "unknown" && in.Consensus != "ambiguous"),
	)
	for _, k := range FeatureKinds {
		v = append(v, b1(kinds[k]))
	}
	for _, c := range FeatureConsequences {
		v = append(v, b1(cons[c]))
	}
	for _, k := range FeatureKB {
		v = append(v, b1(in.KBID == k))
	}
	for _, m := range FeatureMechanisms {
		v = append(v, b1(in.Mechanism == m))
	}
	for _, e := range FeatureEvents {
		v = append(v, b1(events[e]))
	}
	for _, c := range FeatureConsensus {
		v = append(v, b1(in.Consensus == c))
	}
	role := roleOf(in.Tag, in.Text)
	for _, r := range FeatureRoles {
		v = append(v, b1(role == r))
	}
	for _, e := range FeatureEngines {
		v = append(v, b1(in.Engine == e))
	}
	if len(v) != len(FeatureNames) {
		return nil, fmt.Errorf("triage: feature vector %d values, %d names", len(v), len(FeatureNames))
	}
	out := make([]float64, len(v))
	for i, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			out[i] = -1
			continue
		}
		out[i] = math.Floor(x*1e4+0.5) / 1e4
	}
	return out, nil
}

func log1p(x float64) float64 { return math.Log1p(math.Max(0, x)) }

func b1(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func severityOr1(n int) float64 {
	if n < 1 {
		return 1
	}
	return float64(n)
}

func numOr(p *float64, dflt float64) float64 {
	if p == nil {
		return dflt
	}
	return *p
}
