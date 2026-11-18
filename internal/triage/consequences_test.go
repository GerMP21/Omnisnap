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
