package pipeline

import (
	"fmt"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// joinIntent joins each violation to the cause on the same element (or just
// above it), or creates an intent-* cause of its own (Task 48). A
// children-moved cause on the parent absorbs the violation when two or more of
// its children break relations; a single violating child stays its own cause.
func joinIntent(res *layoutdiff.Analysis, A, B *layoutdiff.Tree, m *layoutdiff.Match, vs []layoutdiff.Violation) []schema.IntentViolation {
	violatingKids := map[int]int{}
	for _, v := range vs {
		if v.Node.Parent != nil {
			violatingKids[v.Node.Parent.UID]++
		}
	}
	causeAt := func(n *layoutdiff.Node) *layoutdiff.Cause {
		for i := range res.Causes {
			c := &res.Causes[i]
			if c.Side != "A" {
				continue
			}
			if c.Node == n || c.Node.Parent == n ||
				(c.Node.Parent != nil && c.Node.Parent.Parent == n) {
				return c
			}
			if c.Node == n.Parent && n.Parent != nil && hasPrefix(c.Kinds, "children") && violatingKids[n.Parent.UID] >= 2 {
				return c
			}
		}
		return nil
	}
	var out []schema.IntentViolation
	for _, v := range vs {
		text := fmt.Sprintf("%s: %s breaks %s with %d sibling(s) by %gpx (kept by %s)",
			sideName(v.Engine, A, B), layoutdiff.Selector(v.Node), intentWords(v.Kinds), v.Others, v.Mag, strings.Join(v.HoldsIn, ", "))
		if c := causeAt(v.Node); c == nil {
			var d *layoutdiff.Delta
			if delta, ok := res.D[v.Node.UID]; ok {
				d = &delta
			}
			res.Causes = append(res.Causes, layoutdiff.Cause{
				Side: "A", Node: v.Node, Other: m.PairOf[v.Node.UID],
				Kinds: []string{layoutdiff.IntentKind(v.Kinds)}, D: d, Note: text,
			})
		}
		side := schema.SideBaseline
		if v.Engine == "B" {
			side = schema.SideEngine
		}
		out = append(out, schema.IntentViolation{
			Engine:    side,
			Selector:  layoutdiff.Selector(v.Node),
			Relations: v.Kinds,
			PX:        v.Mag,
			KeptBy:    v.HoldsIn,
		})
	}
	return out
}

func sideName(engine string, A, B *layoutdiff.Tree) string {
	if engine == "B" {
		return B.Engine
	}
	return A.Engine
}

func intentWords(kinds []string) string {
	var words []string
	for _, k := range kinds {
		if t, ok := layoutdiff.IntentText[k]; ok {
			words = append(words, t)
		} else {
			words = append(words, k)
		}
	}
	return strings.Join(words, ", ")
}

func hasPrefix(ss []string, p string) bool {
	for _, s := range ss {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
