// Package pipeline runs the M1 comparison for one page pair: layout diff,
// triage, and (later) appearance and intent. It is the glue between capture
// artifacts and summary.json (SPEC-triage.md, SPEC-layout-diff.md).
package pipeline

import (
	"fmt"
	"image"
	"time"

	"github.com/GerMP21/Omnisnap/internal/appearance"
	"github.com/GerMP21/Omnisnap/internal/branded"
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
)

// ThirdCapture is another engine family's captures, for design intent: a
// relation needs two families keeping it, and a third deciding which side is
// odd.
type ThirdCapture struct {
	Engine string
	A, B   *schema.Capture
}

// Pair is one comparison's outcome.
type Pair struct {
	Causes   []schema.Cause
	Intent   []schema.IntentViolation
	Warnings []string
	Dynamic  schema.Sides
	Blocked  schema.Sides
	Stats    schema.Stats
	Symptoms []layoutdiff.Symptom
	Notes    []string
	Features map[string][]float64
	// CauseRefs locate each cause in the capture it came from (side A or B,
	// node uid), for the findings file and the mutant labels.
	CauseRefs map[string]triage.CauseRef
}

// Inputs is everything one comparison needs.
type Inputs struct {
	SlugBase       string
	BaselineEngine string
	Engine         string
	A, A2          *schema.Capture
	B, B2          *schema.Capture
	Thirds         []ThirdCapture
	// Probes are the decoded probes.json.gz maps (id -> capture) of the two
	// engines' compared runs; nil when probes were not captured.
	ProbesA, ProbesB map[string]*schema.Capture
	// Images are the full-page screenshots of the two compared runs, for the
	// appearance check and ink evidence; nil when not available.
	ImagesA, ImagesB image.Image
}

// Diff is Run with everything spelled out.
//
// It compares the baseline's captures (a, a2) with the engine's (b, b2). Any
// of a2/b2 may be nil (one run): dynamic content is then not masked and a
// warning says so. slugBase is the cause-id prefix ("<slug>" for Firefox,
// "<slug>@<engine>" otherwise, as the spike names them).
func Diff(slugBase, baselineEngine, engine string, a, a2, b, b2 *schema.Capture, thirds ...ThirdCapture) Pair {
	return Run(Inputs{SlugBase: slugBase, BaselineEngine: baselineEngine, Engine: engine,
		A: a, A2: a2, B: b, B2: b2, Thirds: thirds})
}

// Run compares and triages one page pair.
func Run(in Inputs) Pair {
	start := nowMS()
	slugBase, baselineEngine, engine := in.SlugBase, in.BaselineEngine, in.Engine
	a, a2, b, b2, thirds := in.A, in.A2, in.B, in.B2, in.Thirds
	A, A2 := prepPair(a, a2, baselineEngine)
	B, B2 := prepPair(b, b2, engine)
	A, A2, B, B2, runA, runB := layoutdiff.PairRuns(A, A2, B, B2)
	p := layoutdiff.Pair{
		A: A, A2: A2, B: B, B2: B2, RunA: runA, RunB: runB,
		MaskA: map[int]bool{}, MaskB: map[int]bool{},
	}
	if A2 != nil {
		p.MaskA = layoutdiff.DynamicMask(A, A2)
	}
	if B2 != nil {
		p.MaskB = layoutdiff.DynamicMask(B, B2)
	}
	m := layoutdiff.MatchTrees(A, B)
	res := layoutdiff.Analyze(A, B, m, p.MaskA, p.MaskB)
	// Appearance (Tasks 49/50): graphics whose geometry matched but whose
	// pixels differ; ink evidence for every cause.
	ink := map[int]*triage.InkFeature{}
	if in.ImagesA != nil && in.ImagesB != nil {
		floor := inkFloor(in.ImagesA, in.ImagesB, A, B, m, res.D, p.MaskA, p.MaskB)
		for _, c := range res.Causes {
			if c.Other == nil {
				continue
			}
			if ev, ok := appearance.InkEvidence(in.ImagesA, in.ImagesB, c.Node.N.B, c.Other.N.B); ok && ev.Ink >= 20 {
				ink[c.Node.UID] = &triage.InkFeature{Agreement: ev.Agreement, Floor: floor, Ink: ev.Ink}
			}
		}
		for _, g := range appearance.Detect(A, B, m, res.D, p.MaskA, p.MaskB, in.ImagesA, in.ImagesB) {
			res.Causes = append(res.Causes, layoutdiff.Cause{
				Side: "A", Node: g.Node, Other: m.PairOf[g.Node.UID], Kinds: []string{"appearance"},
				Note: fmt.Sprintf("drawn differently (SSIM %.3f)", g.SSIM),
			})
		}
	}
	// Blocked-request attribution (Task 66): an element whose resources come
	// from a host the engine blocked carries that host.
	if hosts := branded.BlockedHosts(B.Meta.Failed); len(hosts) > 0 {
		for i := range res.Causes {
			seen := map[string]bool{}
			for _, n := range layoutdiff.Subtree(res.Causes[i].Node) {
				if n.N.U == "" {
					continue
				}
				if h, ok := branded.Matches(branded.HostOf(n.N.U), hosts); ok && !seen[h] {
					seen[h] = true
					res.Causes[i].BlockedBy = append(res.Causes[i].BlockedBy, h)
				}
			}
		}
	}
	// Design intent (Tasks 46/48): needs a third engine family. Its
	// violations join the cause on the same element or become intent-* causes.
	var intentResult layoutdiff.IntentResult
	var intents []schema.IntentViolation
	if len(thirds) > 0 {
		intentResult = layoutdiff.IntentViolations(A, B, m, prepThirds(thirds, A), p.MaskA, p.MaskB)
		intents = joinIntent(res, A, B, m, intentResult.Violations)
	}
	// Intent verdicts per A node, for the feature vector (Task 62).
	intentVerdicts := map[int]string{}
	for _, v := range intentResult.Violations {
		if v.Node == nil {
			continue
		}
		if v.Engine == "B" {
			intentVerdicts[v.Node.UID] = "B-violates"
		} else if _, seen := intentVerdicts[v.Node.UID]; !seen {
			intentVerdicts[v.Node.UID] = "A-violates"
		}
	}
	w := layoutdiff.WarningsOf(p)
	causes, features, causeRefs := triage.BuildWithFeatures(res, A, B, slugBase, B.Engine, triage.Options{
		Evidence:       evidenceContext(A, B, in.ProbesA, in.ProbesB),
		Thirds:         prepThirds(thirds, A),
		M:              m,
		Ink:            ink,
		Calibration:    evidence.Calibrate(A, m),
		IntentVerdicts: intentVerdicts,
		HasThirds:      len(thirds) > 0,
	})
	pair := Pair{
		Causes:    causes,
		Features:  features,
		CauseRefs: causeRefs,
		Intent:    intents,
		Warnings:  w.List,
		Notes:     w.Notes,
		Dynamic:   schema.Sides{Baseline: w.DynamicA, Engine: w.DynamicB},
		Blocked:   schema.Sides{Baseline: w.BlockedA, Engine: w.BlockedB},
		Symptoms:  res.Symptoms,
	}
	for _, c := range causes {
		if c.Tier == schema.TierReport {
			pair.Stats.Report++
		} else {
			pair.Stats.Info++
		}
	}
	if nodes := len(A.All); nodes > 0 {
		pair.Stats.MatchRate = float64(len(m.PairOf)) / float64(nodes)
	}
	pair.Stats.MS = nowMS() - start
	if pair.Dynamic.Baseline == nil {
		pair.Dynamic.Baseline = []string{}
	}
	if pair.Dynamic.Engine == nil {
		pair.Dynamic.Engine = []string{}
	}
	if pair.Blocked.Baseline == nil {
		pair.Blocked.Baseline = []string{}
	}
	if pair.Blocked.Engine == nil {
		pair.Blocked.Engine = []string{}
	}
	if pair.Causes == nil {
		pair.Causes = []schema.Cause{}
	}
	if pair.Intent == nil {
		pair.Intent = []schema.IntentViolation{}
	}
	if intentResult.Reason != "" {
		pair.Warnings = append(pair.Warnings, "design intent not checked: "+intentResult.Reason)
	}
	if pair.Warnings == nil {
		pair.Warnings = []string{}
	}
	return pair
}

func prepPair(a, a2 *schema.Capture, engine string) (*layoutdiff.Tree, *layoutdiff.Tree) {
	if a == nil {
		return nil, nil
	}
	first := layoutdiff.Prep(a, engine)
	if a2 == nil {
		return first, nil
	}
	return first, layoutdiff.Prep(a2, engine)
}

// inkFloor is the page's rasterization noise floor: the agreement of up to 120
// unchanged, visible text/media elements with enough ink.
func inkFloor(imgA, imgB image.Image, A, B *layoutdiff.Tree, m *layoutdiff.Match, deltas map[int]layoutdiff.Delta, maskA, maskB map[int]bool) float64 {
	var samples []appearance.Ink
	for _, a := range A.All {
		if len(samples) >= 120 {
			break
		}
		if bool(a.N.Hid) || a.N.B[2]*a.N.B[3] < 200 || !a.HasText() && !appearance.Graphics[a.N.T] {
			continue
		}
		b := m.PairOf[a.UID]
		if b == nil || maskA[a.UID] || maskB[b.UID] {
			continue
		}
		if d, ok := deltas[a.UID]; ok && d.Any {
			continue
		}
		if ev, ok := appearance.InkEvidence(imgA, imgB, a.N.B, b.N.B); ok {
			samples = append(samples, ev)
		}
	}
	floor, _, _ := appearance.NoiseFloor(samples)
	return floor
}

// evidenceContext builds the probe diagnosis context; nil without probes.
func evidenceContext(A, B *layoutdiff.Tree, probesA, probesB map[string]*schema.Capture) *evidence.Context {
	if len(probesA) == 0 || len(probesB) == 0 {
		return nil
	}
	return evidence.NewContext(A, B, probesA, probesB, A.Engine, B.Engine)
}

// prepThirds picks, per third engine, the run whose page state matches A best
// (the spike pairs by docH) and matches it against A.
func prepThirds(thirds []ThirdCapture, A *layoutdiff.Tree) []layoutdiff.Third {
	var out []layoutdiff.Third
	for _, t := range thirds {
		var best *schema.Capture
		for _, c := range []*schema.Capture{t.A, t.B} {
			if c == nil {
				continue
			}
			if best == nil || absInt(c.Meta.DocH-A.Meta.DocH) < absInt(best.Meta.DocH-A.Meta.DocH) {
				best = c
			}
		}
		if best == nil {
			continue
		}
		tree := layoutdiff.Prep(best, t.Engine)
		out = append(out, layoutdiff.Third{Engine: t.Engine, T: tree, M: layoutdiff.MatchTrees(A, tree)})
	}
	return out
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// nowMS is a variable so tests can make timings predictable if needed.
var nowMS = func() int64 { return time.Now().UnixMilli() }
