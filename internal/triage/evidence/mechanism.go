// Package evidence explains a cause: which neutralizing probe removes the
// difference (mechanism), and later what a human would perceive and whether
// other engines agree (SPEC-triage.md).
package evidence

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// ProbeOrder is the order of neutralizing probes (probes.js).
var ProbeOrder = []string{"lh", "font", "controls"}

// ProbeMeaning is the human explanation per verdict.
var ProbeMeaning = map[string]string{
	"lh":          "fractional or font-derived line-height (fix: whole-pixel line-height)",
	"font":        "text metrics: advance, shaping or font metrics (no fix needed unless it wraps or clips)",
	"controls":    "native form-control defaults (fix: explicit control styling and size)",
	"all":         "a combination of text metrics, line-height and control defaults",
	"layout":      "persists with text, line-height and controls neutralized: a layout or author-CSS difference",
	"author-font": "the font probe removes it, but the page asks for DIFFERENT fonts in each engine (author CSS or a failed web font): worth reporting",
}

// ReadProbes decodes <engine>-<run>.probes.json.gz.
func ReadProbes(r io.Reader) (map[string]*schema.Capture, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("evidence: probes: %w", err)
	}
	defer func() { _ = zr.Close() }()
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("evidence: probes: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("evidence: probes: %w", err)
	}
	out := map[string]*schema.Capture{}
	for id, msg := range raw {
		var c schema.Capture
		if err := json.Unmarshal(msg, &c); err != nil {
			return nil, fmt.Errorf("evidence: probe %s: %w", id, err)
		}
		out[id] = &c
	}
	return out, nil
}

// Context holds the matchings the mechanism test needs, computed once per pair.
type Context struct {
	M  *layoutdiff.Match
	A  *layoutdiff.Tree
	B  *layoutdiff.Tree
	MA map[string]*layoutdiff.Match
	MB map[string]*layoutdiff.Match
	MP map[string]*layoutdiff.Match
	PA map[string]*layoutdiff.Tree
	PB map[string]*layoutdiff.Tree
	PM map[string]*layoutdiff.Match
}

// NewContext precomputes base->probe matchings for each engine and
// probe(A)->probe(B) for the cross-engine comparison.
func NewContext(A, B *layoutdiff.Tree, probesA, probesB map[string]*schema.Capture, engineA, engineB string) *Context {
	if len(probesA) == 0 || len(probesB) == 0 {
		return nil
	}
	ctx := &Context{
		M:  layoutdiff.MatchTrees(A, B),
		A:  A,
		B:  B,
		MA: map[string]*layoutdiff.Match{},
		MB: map[string]*layoutdiff.Match{},
		MP: map[string]*layoutdiff.Match{},
		PA: map[string]*layoutdiff.Tree{},
		PB: map[string]*layoutdiff.Tree{},
		PM: map[string]*layoutdiff.Match{},
	}
	for id, ca := range probesA {
		cb, ok := probesB[id]
		if !ok {
			continue
		}
		pa := layoutdiff.Prep(ca, engineA)
		pb := layoutdiff.Prep(cb, engineB)
		ctx.PA[id], ctx.PB[id] = pa, pb
		ctx.MA[id] = layoutdiff.MatchTrees(A, pa)
		ctx.MB[id] = layoutdiff.MatchTrees(B, pb)
		ctx.MP[id] = layoutdiff.MatchTrees(pa, pb)
		ctx.PM[id] = ctx.MP[id]
	}
	return ctx
}

// Verdict is the mechanism diagnosis of one cause.
type Verdict struct {
	Verdict     string          `json:"verdict"` // lh, font, controls, all, layout, author-font, unknown
	Explanation string          `json:"explanation,omitempty"`
	Results     map[string]bool `json:"results,omitempty"`
	Verified    bool            `json:"verified"`
}

// Mechanism diagnoses a cause by which probe removes it.
func Mechanism(c layoutdiff.Cause, ctx *Context) *Verdict {
	if ctx == nil {
		return nil
	}
	results := map[string]bool{}
	for _, id := range append(append([]string{}, ProbeOrder...), "all") {
		if _, ok := ctx.MA[id]; !ok {
			continue
		}
		if s, ok := survives(c, id, ctx); ok {
			results[id] = s
		}
	}
	gone := ""
	for _, id := range ProbeOrder {
		if v, ok := results[id]; ok && !v {
			gone = id
			break
		}
	}
	if gone == "font" && fontsDiffer(c, ctx.M) {
		return &Verdict{Verdict: "author-font", Explanation: ProbeMeaning["author-font"], Results: results, Verified: true}
	}
	if gone != "" {
		return &Verdict{Verdict: gone, Explanation: ProbeMeaning[gone], Results: results, Verified: true}
	}
	if v, ok := results["all"]; ok && !v {
		return &Verdict{Verdict: "all", Explanation: ProbeMeaning["all"], Results: results, Verified: true}
	}
	if v, ok := results["all"]; ok && v {
		return &Verdict{Verdict: "layout", Explanation: ProbeMeaning["layout"], Results: results}
	}
	return &Verdict{Verdict: "unknown", Results: results}
}

// survives reports whether the cause's difference survives probe id; ok is
// false when the probe cannot answer for this cause.
func survives(c layoutdiff.Cause, id string, ctx *Context) (bool, bool) {
	var aNode, bNode *layoutdiff.Node
	if c.Side == "A" {
		aNode, bNode = c.Node, c.Other
	} else {
		bNode = c.Node
	}
	ma, mb := ctx.MA[id], ctx.MB[id]
	if aNode != nil && bNode != nil {
		ap := ma.PairOf[aNode.UID]
		bp := mb.PairOf[bNode.UID]
		if ap == nil || bp == nil {
			return false, false
		}
		if contains(c.Kinds, "children-differ") {
			return visibleKids(ap) != visibleKids(bp), true
		}
		own := false
		for _, k := range c.Kinds {
			if len(k) < 8 || k[:8] != "children" {
				own = true
			}
		}
		if own && layoutdiff.Deltas(ap, bp, ctx.PM[id]).Any {
			return true, true
		}
		if contains(c.Kinds, "children-moved") {
			tested := 0
			for i, k := range aNode.Kids {
				if i >= 40 {
					break
				}
				kb := ctx.M.PairOf[k.UID]
				kp := ma.PairOf[k.UID]
				var kbp *layoutdiff.Node
				if kb != nil {
					kbp = mb.PairOf[kb.UID]
				}
				if kp == nil || kbp == nil {
					continue
				}
				tested++
				if layoutdiff.Deltas(kp, kbp, ctx.PM[id]).Any {
					return true, true
				}
			}
			if tested > 0 {
				return false, true
			}
			return false, false
		}
		return false, own
	}
	// Present in one engine only: does it get a partner under the probe?
	if aNode != nil {
		ap := ma.PairOf[aNode.UID]
		if ap == nil {
			return false, false
		}
		_, paired := ctx.PM[id].PairOf[ap.UID]
		return !paired, true
	}
	if bNode != nil {
		bp := mb.PairOf[bNode.UID]
		if bp == nil {
			return false, false
		}
		_, paired := ctx.PM[id].RevPair[bp.UID]
		return !paired, true
	}
	return false, false
}

func visibleKids(n *layoutdiff.Node) int {
	count := 0
	for _, k := range n.Kids {
		if !bool(k.N.Hid) {
			count++
		}
	}
	return count
}

// fontsDiffer: the page asks for different font families in the two engines
// (author CSS per engine, or a web font that failed in one).
func fontsDiffer(c layoutdiff.Cause, M *layoutdiff.Match) bool {
	if c.Side != "A" {
		return false
	}
	stack := []*layoutdiff.Node{c.Node}
	for seen := 0; len(stack) > 0 && seen < 60; seen++ {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		b := M.PairOf[n.UID]
		if n.HasText() && b != nil && b.HasText() && n.N.FF != "" && b.N.FF != "" && n.N.FF != b.N.FF {
			return true
		}
		stack = append(stack, n.Kids...)
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
