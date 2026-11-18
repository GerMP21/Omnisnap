package groundtruth

import (
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 71: mutant labels are pure geometry over two captures, so synthetic
// pairs pin every rule: the target changed, a far change, a hidden target, a
// dynamic region, and the clean-diff exclusion.

// node builds a rendered element. Boxes are absolute document pixels.
func node(tag, id string, x, y, w, h float64, kids ...schema.Node) schema.Node {
	return schema.Node{T: tag, ID: id, D: "block", B: [4]float64{x, y, w, h}, K: kids}
}

func leaf(tag, id string, x, y, w, h float64) schema.Node {
	return node(tag, id, x, y, w, h)
}

// fixture is the page both sides of every label test start from:
//
//	html > body > div#root
//	  section#a > div#card > p#t, span#s
//	  section#b > div#panel > div#inner > span#far, span#far2
//
// The target (p#t) is four levels from section#b, so a change under it is
// outside the target's neighbourhood.
func fixture() *schema.Capture {
	root := node("html", "", 0, 0, 1000, 600,
		node("body", "", 0, 0, 1000, 600,
			node("div", "root", 0, 0, 1000, 600,
				node("section", "a", 0, 0, 500, 300,
					node("div", "card", 0, 0, 500, 100,
						leaf("p", "t", 0, 0, 200, 20),
						leaf("span", "s", 0, 20, 200, 20),
					),
				),
				node("section", "b", 500, 0, 500, 300,
					node("div", "panel", 500, 0, 500, 300,
						node("div", "inner", 500, 0, 500, 300,
							leaf("span", "far", 500, 0, 100, 20),
							leaf("span", "far2", 500, 20, 100, 20),
						),
					),
				),
			),
		),
	)
	return captureOf(root)
}

func captureOf(root schema.Node) *schema.Capture {
	return &schema.Capture{
		Meta: schema.Meta{VW: 1000, VH: 600, CW: 1000, DocW: 1000, DocH: 600, Nodes: count(root), Engine: "chrome"},
		Root: root,
	}
}

func count(n schema.Node) int {
	total := 1
	for _, k := range n.K {
		total += count(k)
	}
	return total
}

// find returns a pointer into the capture's tree.
func find(n *schema.Node, id string) *schema.Node {
	if n.ID == id {
		return n
	}
	for i := range n.K {
		if got := find(&n.K[i], id); got != nil {
			return got
		}
	}
	return nil
}

func TestLabelMutantTargetChanged(t *testing.T) {
	clean := fixture()
	mut := fixture()
	p := find(&mut.Root, "t")
	p.B[2] = 120 // the injected width-shrink
	p.Mut = true
	find(&mut.Root, "card").MutP = true
	mut.Meta.Mutation = &schema.Mutation{Op: "width-shrink"}

	label, err := LabelMutant(clean, mut, map[int]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !label.Effective || label.CleanCause {
		t.Errorf("Effective = %v, CleanCause = %v; want a visible effect", label.Effective, label.CleanCause)
	}
	if label.Op != "width-shrink" {
		t.Errorf("op = %q", label.Op)
	}
	if label.Scope != 1 || label.Changed == 0 {
		t.Errorf("scope = %d, changed = %d", label.Scope, label.Changed)
	}
	if !strings.Contains(label.Target, "p#t") {
		t.Errorf("target = %q, want the p#t selector", label.Target)
	}
	if len(label.Positives) != 1 || !strings.Contains(label.Positives[0], "p#t") {
		t.Errorf("positives = %v, want just the target", label.Positives)
	}
	// A changed ancestor box would be reported too; here the boxes are fixed,
	// so only the target may move.
	for _, p := range label.Positives {
		if strings.Contains(p, "card") || strings.Contains(p, "section") {
			t.Errorf("positives include an ancestor: %v", label.Positives)
		}
	}
}

func TestLabelMutantFarChangeIsNotAPositive(t *testing.T) {
	clean := fixture()
	mut := fixture()
	p := find(&mut.Root, "t")
	p.Mut = true
	find(&mut.Root, "card").MutP = true
	// The target is untouched; something far away changes.
	find(&mut.Root, "far2").B[2] = 140

	label, err := LabelMutant(clean, mut, map[int]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if label.Effective || !label.CleanCause {
		t.Errorf("Effective = %v, CleanCause = %v; a far change must not label the mutant", label.Effective, label.CleanCause)
	}
	if label.Changed == 0 {
		t.Error("the far change was not counted at all")
	}
	if len(label.Positives) != 0 {
		t.Errorf("positives = %v, want none", label.Positives)
	}
}

func TestLabelMutantHiddenTargetUsesTheParent(t *testing.T) {
	clean := fixture()
	mut := fixture()
	// display:none removes the element from the capture; the parent carries
	// the mutp marker.
	card := find(&mut.Root, "card")
	card.K = card.K[1:] // drop p#t
	card.MutP = true
	card.B[3] = 60 // the card shrank when the paragraph went away
	mut.Meta.Nodes = count(mut.Root)
	mut.Meta.Mutation = &schema.Mutation{Op: "hide"}

	label, err := LabelMutant(clean, mut, map[int]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !label.Effective {
		t.Errorf("Effective = false, positives = %v; the hidden target must label", label.Positives)
	}
	if !strings.Contains(label.Target, "p#t") {
		t.Errorf("target = %q, want the vanished p#t", label.Target)
	}
	found := false
	for _, p := range label.Positives {
		if strings.Contains(p, "p#t") {
			found = true
		}
	}
	if !found {
		t.Errorf("positives = %v, want the hidden p#t", label.Positives)
	}
}

func TestLabelMutantIgnoresTheDynamicMask(t *testing.T) {
	clean := fixture()
	// A second clean run where far2 moved: the dynamic mask marks it.
	clean2 := fixture()
	find(&clean2.Root, "far2").B[2] = 140
	mask := layoutdiff.DynamicMask(layoutdiff.Prep(clean, ""), layoutdiff.Prep(clean2, ""))
	if len(mask) == 0 {
		t.Fatal("the dynamic mask is empty")
	}

	mut := fixture()
	find(&mut.Root, "t").Mut = true
	find(&mut.Root, "card").MutP = true
	const target = "t"
	find(&mut.Root, target).B[2] = 120
	find(&mut.Root, "far2").B[2] = 140

	label, err := LabelMutant(clean, mut, mask)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range label.Positives {
		if strings.Contains(p, "far2") {
			t.Errorf("a masked dynamic region became positive: %v", label.Positives)
		}
	}
	// Only the target changed as far as the label is concerned.
	if label.Changed != 1 {
		t.Errorf("changed = %d, want 1 (the target only)", label.Changed)
	}
}

func TestLabelMutantNeedsACapture(t *testing.T) {
	if _, err := LabelMutant(nil, fixture(), nil); err == nil {
		t.Error("a missing clean capture was accepted")
	}
	// No marker at all means the label cannot find its answer.
	if _, err := LabelMutant(fixture(), fixture(), map[int]bool{}); err == nil {
		t.Error("a capture with no mutation marker was labelled")
	}
}

func TestCauseKeyIgnoresOrderOfThePage(t *testing.T) {
	a := schema.Cause{Kinds: []string{"size", "position"}, Selector: "p#t"}
	b := schema.Cause{Kinds: []string{"size", "position"}, Selector: "p#t"}
	c := schema.Cause{Kinds: []string{"position"}, Selector: "p#t"}
	if CauseKey(a) != CauseKey(b) {
		t.Error("the same cause keyed differently")
	}
	if CauseKey(a) == CauseKey(c) {
		t.Error("a different kind set keyed the same")
	}
}

func TestPositiveCausesExcludesWhatTheCleanDiffAlreadyHad(t *testing.T) {
	label := MutantLabel{Positives: []string{"div#root > p#t"}}
	causes := []schema.Cause{
		{ID: "1", Kinds: []string{"size"}, Selector: "div#root > p#t"},
		{ID: "2", Kinds: []string{"size"}, Selector: "div#root > span#far"},
		{ID: "3", Kinds: []string{"position"}, Selector: "div#root > p#t"},
		{ID: "4", Kinds: []string{"size"}, Selector: "div#root", Instances: []string{"div#root > p#t"}},
	}
	cleanKeys := map[string]bool{CauseKey(causes[2]): true}
	refs := make([]CauseRef, len(causes))
	for i, c := range causes {
		refs[i] = CauseRef{Cause: c} // no nodeId: the selector fallback
	}
	got := PositiveCauses(CauseJoin{Causes: refs, Label: label, CleanKeys: cleanKeys})
	if !got["1"] {
		t.Error("the cause on the target is not positive")
	}
	if got["2"] {
		t.Error("a cause away from the target is positive")
	}
	if got["3"] {
		t.Error("a cause the clean diff already had is positive")
	}
	if !got["4"] {
		t.Error("a cause whose instance is the target is not positive")
	}
}

// uidOf returns a node's uid in its capture's tree (document order).
func uidOf(c *schema.Capture, id string) int {
	for _, n := range layoutdiff.Prep(c, "").All {
		if n.N.ID == id {
			return n.UID
		}
	}
	return -1
}

// The join is exact by node id, which is what the spec requires
// (SPEC-ground-truth-mutant-policy.md): the selectors below are deliberately
// wrong, so only identity can mark the target.
func TestPositiveCausesJoinsByNode(t *testing.T) {
	clean := fixture()
	mut := fixture()
	p := find(&mut.Root, "t")
	p.B[2] = 120
	p.Mut = true
	find(&mut.Root, "card").MutP = true
	mut.Meta.Mutation = &schema.Mutation{Op: "width-shrink"}
	label, err := LabelMutant(clean, mut, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(label.PositiveUIDs) == 0 {
		t.Fatal("the label has no positive uids")
	}
	baseline := fixture()
	baseline.Meta.Engine = "firefox"

	refs := []CauseRef{
		// The target, through the mutated engine (side B).
		{Cause: schema.Cause{ID: "b-target", Kinds: []string{"size"}, Selector: "wrong"}, Side: "B", Node: uidOf(mut, "t"), HasNode: true},
		// The same element, seen from the baseline engine (side A).
		{Cause: schema.Cause{ID: "a-target", Kinds: []string{"size"}, Selector: "wrong"}, Side: "A", Node: uidOf(baseline, "t"), HasNode: true},
		// A far element that did not change.
		{Cause: schema.Cause{ID: "far", Kinds: []string{"size"}, Selector: "wrong"}, Side: "B", Node: uidOf(mut, "far2"), HasNode: true},
	}
	got := PositiveCauses(CauseJoin{
		Causes: refs, Label: label, Mutant: mut, Clean: clean, Baseline: baseline,
	})
	if !got["b-target"] {
		t.Error("a side-B cause on the mutated target is not positive")
	}
	if !got["a-target"] {
		t.Error("a side-A cause on the same element is not positive")
	}
	if got["far"] {
		t.Error("an unchanged element became positive")
	}
	if len(got) != 2 {
		t.Errorf("positives = %v, want exactly the target", got)
	}
}

// Without ids (bundles written before findings.json carried nodeId) the
// selector fallback still works.
func TestPositiveCausesFallsBackToSelectors(t *testing.T) {
	clean := fixture()
	mut := fixture()
	p := find(&mut.Root, "t")
	p.B[2] = 120
	p.Mut = true
	find(&mut.Root, "card").MutP = true
	label, err := LabelMutant(clean, mut, nil)
	if err != nil {
		t.Fatal(err)
	}
	sel := label.Positives[0]
	got := PositiveCauses(CauseJoin{
		Causes: []CauseRef{{Cause: schema.Cause{ID: "old", Kinds: []string{"size"}, Selector: sel}}},
		Label:  label,
	})
	if !got["old"] {
		t.Error("the selector fallback did not mark the cause")
	}
}
