package triage

import (
	"slices"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 53: one case per consequence.

func causeNode(uid int, box [4]float64, mut func(*schema.Node)) *layoutdiff.Node {
	n := &schema.Node{T: "div", D: "block", B: box}
	if mut != nil {
		mut(n)
	}
	return &layoutdiff.Node{N: n, UID: uid}
}

func overflowNode(uid int, dx, dy float64, mode string) *layoutdiff.Node {
	o := [2]float64{dx, dy}
	return causeNode(uid, [4]float64{0, 0, 100, 20}, func(n *schema.Node) {
		n.O = &o
		n.OF = "visible/" + mode
	})
}

// overflowNodeOF sets both axes' overflow modes ("hidden/visible").
func overflowNodeOF(uid int, dx, dy float64, of string) *layoutdiff.Node {
	o := [2]float64{dx, dy}
	return causeNode(uid, [4]float64{0, 0, 100, 20}, func(n *schema.Node) {
		n.O = &o
		n.OF = of
	})
}

func TestConsequences(t *testing.T) {
	inside := causeNode(1, [4]float64{0, 0, 100, 20}, nil)
	beyond := causeNode(2, [4]float64{1250, 0, 100, 20}, nil) // 1350 > vw 1280
	truncated := causeNode(3, [4]float64{0, 0, 100, 20}, func(n *schema.Node) { n.TR = true })
	interactive := causeNode(4, [4]float64{0, 0, 100, 20}, func(n *schema.Node) { n.IA = true })

	cases := []struct {
		name string
		c    layoutdiff.Cause
		ctx  Context
		want []string
	}{
		{"presence", layoutdiff.Cause{Kinds: []string{"children-differ"}}, Context{}, []string{"presence"}},
		{"presence only-in", layoutdiff.Cause{Kinds: []string{"only-in-B"}}, Context{}, []string{"presence"}},
		{"visibility", layoutdiff.Cause{Kinds: []string{"hidden-in-B"}}, Context{}, []string{"visibility"}},
		{"structural columns", layoutdiff.Cause{Kinds: []string{"columns"}}, Context{}, []string{"structural"}},
		{"content moved", layoutdiff.Cause{Kinds: []string{"text-shifted"}}, Context{}, []string{"content-moved"}},
		{"appearance", layoutdiff.Cause{Kinds: []string{"appearance"}}, Context{}, []string{"appearance"}},
		{"intent", layoutdiff.Cause{Kinds: []string{"intent-broken"}}, Context{}, []string{"intent-broken"}},
		{"clipped", layoutdiff.Cause{Side: "B", Node: overflowNode(10, 0, 200, "hidden"), Kinds: []string{"overflow-y"}},
			Context{VWA: 1280, VWB: 1280}, []string{"clipped"}},
		{"scrollbar", layoutdiff.Cause{Side: "B", Node: overflowNode(11, 0, 200, "auto"), Kinds: []string{"overflow-y"}},
			Context{VWA: 1280, VWB: 1280}, []string{"scrollbar"}},
		{"visible overflow is soft", layoutdiff.Cause{Side: "B", Node: overflowNode(12, 0, 200, "visible"), Kinds: []string{"overflow-y"}},
			Context{VWA: 1280, VWB: 1280}, []string{"overflow"}},
		{"protrusion", layoutdiff.Cause{Side: "A", Node: inside, Other: beyond}, Context{VWA: 1280, VWB: 1280}, []string{"protrusion"}},
		{"truncation", layoutdiff.Cause{Side: "A", Node: inside, Other: truncated}, Context{VWA: 1280, VWB: 1280}, []string{"truncation"}},
		{"line count", layoutdiff.Cause{Side: "B", Node: inside, D: &layoutdiff.Delta{DLN: 2}}, Context{}, []string{"line-count"}},
		{"interactive moved", layoutdiff.Cause{Side: "A", Node: interactive, Other: inside},
			Context{D: map[int]layoutdiff.Delta{4: {DX: 5}}}, []string{"interactive-moved"}},
	}
	for _, tc := range cases {
		if tc.c.Side == "" {
			tc.c.Side = "B"
		}
		got := consequences(tc.c, tc.ctx)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: consequences = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The hard set is always Report with weight 3; a KB-expected soft consequence
// is Info and down-weighted.
func TestConsequenceTiers(t *testing.T) {
	hard := layoutdiff.Cause{Side: "B", Node: causeNode(20, [4]float64{0, 0, 10, 10}, nil), Kinds: []string{"hidden-in-B"}}
	a := tierOf(hard, nil, []string{"visibility"}, Context{})
	if a.Tier != schema.TierReport || a.Weight != 3 {
		t.Errorf("hard consequence: tier %v weight %v; want report/3", a.Tier, a.Weight)
	}
	soft := layoutdiff.Cause{Side: "B", Node: causeNode(21, [4]float64{0, 0, 10, 10}, nil), Kinds: []string{"text-shifted"}}
	a = tierOf(soft, nil, []string{"content-moved"}, Context{})
	if a.Tier != schema.TierReport || a.Weight != 2 {
		t.Errorf("soft consequence: tier %v weight %v; want report/2", a.Tier, a.Weight)
	}
	a = tierOf(soft, &KB{Expected: true}, []string{"content-moved"}, Context{})
	if a.Tier != schema.TierInfo || a.Weight != 1 {
		t.Errorf("KB-expected: tier %v weight %v; want info/1", a.Tier, a.Weight)
	}
}

// The image-replacement exception (Task 53): a hidden box whose content is
// pushed a whole viewport beyond it is deliberate hiding, not a cut-off. It
// is not `clipped`, and a cause whose only consequence is that hiding is Info.
func TestImageReplacementDemotion(t *testing.T) {
	ctx := Context{VWA: 1280, VWB: 1280}
	// The Mozilla social icons: the icon text sits 9 999 px to the left,
	// hidden by its box.
	replaced := layoutdiff.Cause{
		Side:  "B",
		Node:  overflowNodeOF(30, 9999, 0, "hidden/visible"),
		Kinds: []string{"overflow-x"},
	}
	got := consequences(replaced, ctx)
	if !slices.Equal(got, []string{"overflow"}) {
		t.Errorf("image replacement: consequences = %v, want [overflow]", got)
	}
	a := tierOf(replaced, nil, got, ctx)
	if a.Tier != schema.TierInfo || a.Weight != 1 {
		t.Errorf("image replacement: tier %v weight %v; want info/1", a.Tier, a.Weight)
	}

	// The near miss: the same hidden box, but the content is only just
	// beyond the viewport — that is a cut-off, and it stays Report.
	cut := layoutdiff.Cause{
		Side:  "B",
		Node:  overflowNodeOF(31, 1279, 0, "hidden/visible"),
		Kinds: []string{"overflow-x"},
	}
	got = consequences(cut, ctx)
	if !slices.Equal(got, []string{"clipped"}) {
		t.Errorf("near miss: consequences = %v, want [clipped]", got)
	}
	a = tierOf(cut, nil, got, ctx)
	if a.Tier != schema.TierReport || a.Weight != 3 {
		t.Errorf("near miss: tier %v weight %v; want report/3", a.Tier, a.Weight)
	}

	// Something else changed too: the hiding is not the whole cause, so it
	// stays Report.
	mixed := layoutdiff.Cause{
		Side:  "B",
		Node:  overflowNodeOF(32, 9999, 0, "hidden/visible"),
		Kinds: []string{"overflow-x", "columns"},
	}
	got = consequences(mixed, ctx)
	a = tierOf(mixed, nil, got, ctx)
	if a.Tier != schema.TierReport {
		t.Errorf("mixed cause: tier %v; want report", a.Tier)
	}

	// A hard consequence is never demoted, whatever else is true.
	hard := layoutdiff.Cause{
		Side:  "B",
		Node:  overflowNodeOF(33, 9999, 0, "hidden/visible"),
		Kinds: []string{"overflow-x", "hidden-in-B"},
	}
	got = consequences(hard, ctx)
	a = tierOf(hard, nil, got, ctx)
	if a.Tier != schema.TierReport {
		t.Errorf("hard consequence: tier %v; want report", a.Tier)
	}
}
