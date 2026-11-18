package triage

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
)

// Grouping and ranking (Task 57): one cause per mechanism (knowledge-base
// entry) or per structural signature (the same CSS on N repeated items). The
// headline instance is the most impactful one; the cause is reported if any
// instance is.

var kindTitles = map[string]string{
	"columns":         "Table columns sized differently",
	"text-shifted":    "Text moved inside its box",
	"intent-align":    "Breaks an alignment the other engines keep",
	"intent-size":     "Breaks equal sizes the other engines keep",
	"intent-gap":      "Breaks even spacing the other engines keep",
	"intent-column":   "Breaks a grid column the other engines keep",
	"intent-overlap":  "Overlaps where the other engines keep it clear",
	"children-moved":  "Children moved",
	"children-differ": "Different children",
	"appearance":      "Drawn differently",
	"wrap":            "Wraps differently",
	"width":           "Width differs",
	"height":          "Height differs",
	"position":        "Moved",
}

func titleOf(kinds []string) string {
	var parts []string
	for _, k := range kinds {
		if t, ok := kindTitles[k]; ok {
			parts = append(parts, t)
		} else {
			parts = append(parts, strings.ReplaceAll(k, "-", " "))
		}
	}
	return strings.Join(parts, " + ")
}

// signature is the structural identity used to group repeated instances: the
// element, its parent and grandparent (tags and first classes, no ids or
// positions), the kinds, and the change rounded to px.
func signature(c layoutdiff.Cause, d layoutdiff.Delta, note string) string {
	part := func(n *layoutdiff.Node) string {
		if n == nil {
			return ""
		}
		classes := strings.Fields(n.N.C)
		if len(classes) > 2 {
			classes = classes[:2]
		}
		return n.N.T + "." + strings.Join(classes, ".")
	}
	var parent, grand *layoutdiff.Node
	if c.Node.Parent != nil {
		parent = c.Node.Parent
		if parent.Parent != nil {
			grand = parent.Parent
		}
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%.0f|%.0f|%d|%s",
		part(c.Node), part(parent), part(grand), c.Side, strings.Join(c.Kinds, "+"),
		jsRound(d.DW), jsRound(d.DH), d.DLN, note)
}

func jsRound(f float64) float64 { return math.Floor(f + 0.5) }

// Build triages and groups one pair's causes into the summary's causes.
// slugBase is the page slug (with @engine appended for non-Firefox pairs, as
// the spike's ids do); engine is the compared engine's name.
func Build(res *layoutdiff.Analysis, A, B *layoutdiff.Tree, slugBase, engine string, opts Options) []schema.Cause {
	causes, _ := BuildWithFeatures(res, A, B, slugBase, engine, opts)
	return causes
}

// BuildWithFeatures is Build plus the model input of every cause, for the
// dataset export (Task 74).
func BuildWithFeatures(res *layoutdiff.Analysis, A, B *layoutdiff.Tree, slugBase, engine string, opts Options) ([]schema.Cause, map[string][]float64) {
	features := map[string][]float64{}
	ctx := Context{
		D:   res.D,
		VWA: A.Meta.VW, VWB: B.Meta.VW,
		A: A, B: B,
	}
	ctx.Engines.A, ctx.Engines.B = A.Engine, B.Engine
	ctx.Evidence = opts.Evidence
	ctx.Thirds = opts.Thirds
	ctx.M = opts.M
	var findings []Finding
	seen := map[string]bool{}
	for _, c := range res.Causes {
		d := layoutdiff.Delta{}
		if c.D != nil {
			d = *c.D
		}
		sel := layoutdiff.Selector(c.Node)
		id := fmt.Sprintf("%s|%s|%s", slugBase, strings.Join(c.Kinds, "+"), sel)
		for i := 2; seen[id]; i++ {
			id = fmt.Sprintf("%s|%s|%s|%d", slugBase, strings.Join(c.Kinds, "+"), sel, i)
		}
		seen[id] = true
		kb := Match(c, ctx)
		cons := consequences(c, ctx)
		assessment := tierOf(c, kb, cons, ctx)
		mech := evidence.Mechanism(c, ctx.Evidence)
		consensus := evidence.ConsensusOf(c, ctx.Thirds)
		var ink *InkFeature
		if opts.Ink != nil {
			ink = opts.Ink[c.Node.UID]
		}
		var events []string
		if ctx.M != nil {
			for _, ev := range evidence.Events(c, ctx.M).Events {
				events = append(events, ev.Type)
			}
		}
		intentVerdict := "unknown"
		if opts.HasThirds {
			intentVerdict = "none"
		}
		if opts.IntentVerdicts != nil {
			if v, ok := opts.IntentVerdicts[c.Node.UID]; ok {
				intentVerdict = v
			}
		}
		var perception *float64
		if ctx.M != nil {
			perception = evidence.Events(c, ctx.M).Score
		}
		key := ""
		if kb != nil {
			key = "kb:" + kb.ID
		} else {
			key = signature(c, d, c.Note)
		}
		cal := evidence.ExplainsModel(c, opts.Calibration)
		severity := layoutdiff.Severity(c.Kinds, c.D)
		f := Finding{Key: key, ID: id, Kinds: c.Kinds, Sel: sel, Side: c.Side, BlockedBy: c.BlockedBy,
			Tag: c.Node.N.T, ElID: c.Node.N.ID, Text: c.Node.TX(), D: d,
			Y: c.Node.N.B[1], Assessment: assessment, Mechanism: mech, Severity: severity,
			Consensus: consensus, Perception: perception, Ink: ink, Calibration: &cal,
			Events: events, IntentVerdict: intentVerdict}
		box := func(n *layoutdiff.Node) *[4]float64 {
			if n == nil {
				return nil
			}
			b := n.N.B
			return &b
		}
		if c.Side == "A" {
			f.A = box(c.Node)
			f.B = box(c.Other)
		} else {
			f.B = box(c.Node)
		}
		findings = append(findings, f)
	}

	groups := map[string][]Finding{}
	var order []string
	for _, f := range findings {
		if _, ok := groups[f.Key]; !ok {
			order = append(order, f.Key)
		}
		groups[f.Key] = append(groups[f.Key], f)
	}
	impactOf := func(inst []Finding) float64 {
		best := 0.0
		for _, f := range inst {
			best = math.Max(best, f.Weight*f.Visibility)
		}
		return best * (1 + math.Log2(float64(len(inst))))
	}
	type cause struct {
		c          schema.Cause
		impact     float64
		y          float64
		side       string
		severity   int
		text       string
		kbID       string
		kbExpected bool
		events     []string
		calibrated bool
		ink        *InkFeature
		intent     string
		tag        string
	}
	var causes []cause
	for _, key := range order {
		inst := groups[key]
		head := inst[0]
		for _, f := range inst[1:] {
			if f.Weight*f.Visibility > head.Weight*head.Visibility ||
				(f.Weight*f.Visibility == head.Weight*head.Visibility && f.Y < head.Y) {
				head = f
			}
		}
		tier := schema.TierInfo
		var cons []string
		consSeen := map[string]bool{}
		for _, f := range inst {
			if f.Tier == schema.TierReport {
				tier = schema.TierReport
			}
			for _, c := range f.Consequences {
				if !consSeen[c] {
					consSeen[c] = true
					cons = append(cons, c)
				}
			}
		}
		title := titleOf(head.Kinds)
		var kb *string
		var kbDetail *schema.KBEntry
		if head.KB != nil {
			title = head.KB.Title
			id := head.KB.ID
			kb = &id
			kbDetail = &schema.KBEntry{ID: head.KB.ID, Title: head.KB.Title, Explanation: head.KB.Explanation, Fix: head.KB.Fix}
		}
		var instances []string
		for _, f := range inst {
			if len(instances) < 50 {
				instances = append(instances, f.Sel)
			}
		}
		if instances == nil {
			instances = []string{}
		}
		headInk := head.Ink
		var calEvidence *schema.CalibrationEvidence
		if head.Calibration != nil {
			reasons := make([]schema.CalibrationReason, len(head.Calibration.By))
			for i, r := range head.Calibration.By {
				reasons[i] = schema.CalibrationReason{Model: r.Model, Expected: r.Expected, Tol: r.Tol, DW: r.DW, DH: r.DH}
			}
			calEvidence = &schema.CalibrationEvidence{Explained: head.Calibration.Explained, By: reasons}
		}
		var consensus *schema.Consensus
		switch head.Consensus {
		case evidence.ConsensusBaselineOdd:
			v := schema.ConsensusBaselineOdd
			consensus = &v
		case evidence.ConsensusEngineOdd:
			v := schema.ConsensusEngineOdd
			consensus = &v
		case evidence.ConsensusAllDiffer:
			v := schema.ConsensusAllDiffer
			consensus = &v
		}
		var mechName *string
		if head.Mechanism != nil && head.Mechanism.Verdict != "unknown" {
			v := head.Mechanism.Verdict
			mechName = &v
		}
		schemaCause := schema.Cause{
			ID:           head.ID,
			Tier:         tier,
			Title:        title,
			Kinds:        head.Kinds,
			Selector:     head.Sel,
			Boxes:        schema.Boxes{Baseline: head.A, Engine: head.B},
			Delta:        schema.Delta{DW: head.D.DW, DH: head.D.DH, DX: head.D.DX, DY: head.D.DY, DLn: float64(head.D.DLN)},
			KB:           kb,
			KBDetail:     kbDetail,
			Count:        len(inst),
			Instances:    instances,
			Consequences: cons,
			Evidence:     schema.Evidence{Mechanism: mechName, Consensus: consensus, Perception: head.Perception, Ink: inkAgreement(headInk), Intent: []string{}, Calibration: calEvidence},
		}
		if schemaCause.Consequences == nil {
			schemaCause.Consequences = []string{}
		}
		var inkFeature *InkFeature
		if head.Ink != nil {
			inkFeature = head.Ink
		}
		calibrated := head.Calibration != nil && head.Calibration.Explained
		kbID, kbExpected := "", false
		if head.KB != nil {
			kbID, kbExpected = head.KB.ID, head.KB.Expected
		}
		var blockedBy []string
		for _, f := range inst {
			for _, h := range f.BlockedBy {
				found := false
				for _, b := range blockedBy {
					if b == h {
						found = true
						break
					}
				}
				if !found {
					blockedBy = append(blockedBy, h)
				}
			}
		}
		if len(blockedBy) > 0 {
			schemaCause.BlockedBy = blockedBy
		}
		causes = append(causes, cause{c: schemaCause, impact: impactOf(inst), y: head.Y,
			side: head.Side, severity: head.Severity, text: head.Text, kbID: kbID, kbExpected: kbExpected,
			events: head.Events, calibrated: calibrated, ink: inkFeature, intent: head.IntentVerdict, tag: head.Tag})
	}
	// Model score (Task 63): written to each cause when the shipped model
	// loads and agrees with this binary's feature list. The tier stays
	// rule-based; the score ranks within it.
	if model := ShippedModel(); model != nil {
		for i := range causes {
			c := &causes[i].c
			var mechanisms, consensus string
			if c.Evidence.Mechanism != nil {
				mechanisms = *c.Evidence.Mechanism
			}
			if c.Evidence.Consensus != nil {
				consensus = string(*c.Evidence.Consensus)
			}
			var box *[4]float64
			if c.Boxes.Baseline != nil {
				box = c.Boxes.Baseline
			} else {
				box = c.Boxes.Engine
			}
			in := FeatureInput{
				Count: c.Count, Side: causes[i].side, D: layoutdiff.Delta{DW: c.Delta.DW, DH: c.Delta.DH, DX: c.Delta.DX, DY: c.Delta.DY, DLN: int(c.Delta.DLn)},
				Box: box, Severity: causes[i].severity, Impact: causes[i].impact,
				RulesTier: string(c.Tier), Tier: string(c.Tier), Text: causes[i].text,
				KBID: causes[i].kbID, KBExpected: causes[i].kbExpected,
				Perception: c.Evidence.Perception, Events: causes[i].events,
				Calibration: causes[i].calibrated, Ink: causes[i].ink,
				IntentVerdict: causes[i].intent, Mechanism: mechanisms, Consensus: consensus,
				Kinds: c.Kinds, Consequences: c.Consequences, Tag: causes[i].tag,
				Engine: B.Engine,
			}
			if x, err := Features(in); err == nil {
				features[c.ID] = x
			}
			if p, err := model.Score(in); err == nil {
				v := math.Round(p*1e4) / 1e4
				c.Score = &v
			}
		}
	}
	sort.SliceStable(causes, func(i, j int) bool {
		ri, rj := causes[i].c.Tier == schema.TierReport, causes[j].c.Tier == schema.TierReport
		if ri != rj {
			return ri
		}
		si, sj := causes[i].c.Score, causes[j].c.Score
		if si != nil && sj != nil && *si != *sj {
			return *si > *sj
		}
		if causes[i].impact != causes[j].impact {
			return causes[i].impact > causes[j].impact
		}
		return causes[i].y < causes[j].y
	})
	out := make([]schema.Cause, len(causes))
	for i, c := range causes {
		out[i] = c.c
	}
	return out, features
}
