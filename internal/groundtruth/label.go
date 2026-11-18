package groundtruth

import (
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Mutant labels (Task 71): what did the injected bug change? Same engine,
// clean vs mutated — no engine noise, so every changed element outside the
// clean runs' dynamic regions is an effect of the mutation. A cross-engine
// cause is positive when its element changed AND sits in the target's
// neighbourhood (target, 3 levels of ancestors and descendants, siblings and
// their children): page drift elsewhere cannot become a positive.

// MutantLabel is one mutant's answer.
type MutantLabel struct {
	Target       string       // selector of the mutated element (or its parent for a hidden target)
	Scope        int          // how many elements the operator hit (a class-wide rule)
	Effective    bool         // at least one neighbourhood element changed
	Positives    []string     // selectors of changed neighbourhood elements
	PositiveUIDs map[int]bool // the same positives as clean-tree uids (exact join)
	Changed      int          // changed elements on the whole page
	Op           string
	CleanCause   bool // the mutation left no visible effect
}

// LabelMutant compares a clean and a mutated capture of the SAME engine. mask
// is the clean tree's dynamic-region mask (uids of the clean capture), from
// DynamicMask on its two runs.
func LabelMutant(clean, mutated *schema.Capture, mask map[int]bool) (MutantLabel, error) {
	if clean == nil || mutated == nil {
		return MutantLabel{}, fmt.Errorf("groundtruth: label: missing capture")
	}
	C := layoutdiff.Prep(clean, clean.Meta.Engine)
	M := layoutdiff.Prep(mutated, mutated.Meta.Engine)
	Ms := layoutdiff.MatchTrees(C, M)

	// Anchors: the mutated targets, mapped to the clean tree. A hidden target
	// is gone from the mutated tree, so use its parent and the child that
	// vanished.
	var anchors []*layoutdiff.Node
	for _, n := range M.All {
		if bool(n.N.Mut) {
			if a := Ms.RevPair[n.UID]; a != nil && len(anchors) < 30 {
				anchors = append(anchors, a)
			}
		}
	}
	if len(anchors) == 0 {
		for _, p := range M.All {
			if !bool(p.N.MutP) || len(anchors) >= 30 {
				continue
			}
			pc := Ms.RevPair[p.UID]
			if pc == nil {
				continue
			}
			var vanished []*layoutdiff.Node
			for _, k := range pc.Kids {
				if Ms.PairOf[k.UID] == nil {
					vanished = append(vanished, k)
				}
			}
			if len(vanished) > 0 {
				anchors = append(anchors, vanished...)
			} else {
				anchors = append(anchors, pc)
			}
		}
	}
	label := MutantLabel{}
	if mutated.Meta.Mutation != nil {
		label.Op = mutated.Meta.Mutation.Op
	}
	if len(anchors) == 0 {
		return label, fmt.Errorf("groundtruth: label: mutation target not found")
	}
	label.Target = layoutdiff.Selector(anchors[0])
	label.Scope = len(anchors)

	// Changed: unmatched or any delta, outside the dynamic regions.
	changed := map[int]bool{}
	for _, c := range C.All {
		if mask[c.UID] {
			continue
		}
		m := Ms.PairOf[c.UID]
		if m == nil || layoutdiff.Deltas(c, m, Ms).Any {
			changed[c.UID] = true
		}
	}
	label.Changed = len(changed)

	// Neighbourhood: the targets, their ancestors (3), their descendants (3),
	// and their siblings plus those siblings' children.
	hood := map[int]bool{}
	var down func(n *layoutdiff.Node, depth int)
	down = func(n *layoutdiff.Node, depth int) {
		hood[n.UID] = true
		if depth > 0 {
			for _, k := range n.Kids {
				down(k, depth-1)
			}
		}
	}
	for _, a := range anchors {
		down(a, 3)
		for x, i := a.Parent, 0; x != nil && i < 3; x, i = x.Parent, i+1 {
			hood[x.UID] = true
		}
		if a.Parent != nil {
			for _, s := range a.Parent.Kids {
				down(s, 1)
			}
		}
	}
	for uid := range hood {
		if changed[uid] {
			label.Effective = true
		}
	}
	// Positive elements: changed AND in the neighbourhood.
	label.PositiveUIDs = map[int]bool{}
	for _, n := range C.All {
		if hood[n.UID] && changed[n.UID] {
			label.Positives = append(label.Positives, layoutdiff.Selector(n))
			label.PositiveUIDs[n.UID] = true
		}
	}
	label.CleanCause = !label.Effective
	return label, nil
}

// CauseKey is a cause's identity without the page prefix: the spike excludes a
// mutated positive when the clean diff already had the same cause (same kinds,
// same element).
func CauseKey(c schema.Cause) string {
	k := fmt.Sprintf("%s|%s", joinKinds(c.Kinds), c.Selector)
	return k
}

func joinKinds(kinds []string) string {
	out := ""
	for i, k := range kinds {
		if i > 0 {
			out += "+"
		}
		out += k
	}
	return out
}

// CauseRef locates a cause in the capture it came from. HasNode is false for
// bundles written before findings.json carried nodeId.
type CauseRef struct {
	Cause   schema.Cause
	Side    string // "A" (baseline) or "B" (compared engine)
	Node    int
	HasNode bool
}

// CauseJoin is everything the label needs to mark causes positive.
type CauseJoin struct {
	Causes   []CauseRef
	Label    MutantLabel
	Mutant   *schema.Capture // the mutated capture the side-B causes came from
	Clean    *schema.Capture // the same engine, clean run
	Baseline *schema.Capture // baseline engine, clean run (side-A causes only)
	// CleanKeys are the causes the clean cross-engine diff already had, keyed
	// by CauseKey: those existed before the mutation and are never positive.
	CleanKeys map[string]bool
}

// PositiveCauses labels the causes of the mutated cross-engine diff. A cause
// is positive when its element changed inside the target's neighbourhood and
// the clean diff did not already have it.
//
// The join is by node identity, the spike's rule: a side-B cause maps through
// the same-engine match (clean vs mutated), a side-A cause through the
// cross-engine match (baseline clean vs the mutated engine's clean run). The
// selector comparison is only the fallback for bundles without nodeId.
func PositiveCauses(j CauseJoin) map[string]bool {
	out := map[string]bool{}
	posSel := map[string]bool{}
	for _, sel := range j.Label.Positives {
		posSel[sel] = true
	}
	exact := len(j.Label.PositiveUIDs) > 0
	var ms, mc *layoutdiff.Match
	if exact && j.Mutant != nil && j.Clean != nil {
		ms = layoutdiff.MatchTrees(layoutdiff.Prep(j.Clean, ""), layoutdiff.Prep(j.Mutant, ""))
	}
	if exact && j.Baseline != nil && j.Clean != nil {
		mc = layoutdiff.MatchTrees(layoutdiff.Prep(j.Baseline, ""), layoutdiff.Prep(j.Clean, ""))
	}
	for _, ref := range j.Causes {
		c := ref.Cause
		if j.CleanKeys[CauseKey(c)] {
			continue
		}
		hit := false
		if exact && ref.HasNode {
			var n *layoutdiff.Node
			switch ref.Side {
			case "A":
				if mc != nil {
					n = mc.PairOf[ref.Node]
				}
			case "B":
				if ms != nil {
					n = ms.RevPair[ref.Node]
				}
			}
			hit = n != nil && j.Label.PositiveUIDs[n.UID]
		} else {
			hit = posSel[c.Selector]
			if !hit {
				for _, inst := range c.Instances {
					if posSel[inst] {
						hit = true
						break
					}
				}
			}
		}
		if hit {
			out[c.ID] = true
		}
	}
	return out
}
