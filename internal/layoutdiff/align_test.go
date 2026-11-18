package layoutdiff

import (
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// n builds a node with a border box and display.
func n(tag string, x, y, w, h float64, kids ...schema.Node) schema.Node {
	return schema.Node{T: tag, B: [4]float64{x, y, w, h}, D: "block", K: kids}
}

// nid is n with an id.
func nid(tag, id string, x, y, w, h float64, kids ...schema.Node) schema.Node {
	v := n(tag, x, y, w, h, kids...)
	v.ID = id
	return v
}

// text marks a node as a text owner.
func text(v schema.Node, s string, lines int) schema.Node {
	ti := 0.0
	v.Text = &schema.Text{TX: s, TH: "h-" + s, LN: lines, TW: 100, TI: &ti}
	return v
}

// capTree wraps a root in a capture with the given document size.
func capTree(root schema.Node, docW, docH int) *schema.Capture {
	root.T = "html"
	root.D = "block"
	return &schema.Capture{
		Meta: schema.Meta{VW: 1280, VH: 800, CW: 1280, DocW: docW, DocH: docH, Nodes: 1},
		Root: root,
	}
}

func TestMatchAnchorsAndLeftovers(t *testing.T) {
	a := Prep(capTree(n("html", 0, 0, 100, 100,
		nid("div", "card", 0, 0, 100, 50),
		n("p", 0, 50, 100, 20),
		n("span", 0, 70, 100, 20),
	), 100, 100), "a")
	b := Prep(capTree(n("html", 0, 0, 100, 100,
		nid("div", "card", 0, 0, 100, 50),
		n("p", 0, 50, 100, 20),
		n("span", 0, 70, 100, 20),
		n("b", 0, 90, 100, 10),
	), 100, 100), "b")
	m := MatchTrees(a, b)
	if len(m.OnlyA) != 0 || len(m.OnlyB) != 1 || m.OnlyB[0].N.T != "b" {
		t.Errorf("onlyA %v, onlyB %v; want the extra <b> as only-in-B", len(m.OnlyA), len(m.OnlyB))
	}
	// Dynamic ids are rescued by tag+class pairing.
	a2 := Prep(capTree(n("html", 0, 0, 100, 100,
		schema.Node{T: "div", C: "x y", B: [4]float64{0, 0, 100, 50}, D: "block"},
	), 100, 100), "a")
	b2 := Prep(capTree(n("html", 0, 0, 100, 100,
		schema.Node{T: "div", C: "x y", B: [4]float64{0, 0, 100, 50}, D: "block"},
	), 100, 100), "b")
	if m := MatchTrees(a2, b2); len(m.OnlyA)+len(m.OnlyB) != 0 {
		t.Errorf("class pairing failed: %d unmatched", len(m.OnlyA)+len(m.OnlyB))
	}
}

func TestDeltas(t *testing.T) {
	base := func(w, h float64) schema.Node {
		return n("html", 0, 0, 800, 800, n("div", 10, 20, w, h))
	}
	pair := func(a, b schema.Node) (*Node, *Node, *Match) {
		A := Prep(capTree(a, 800, 800), "a")
		B := Prep(capTree(b, 800, 800), "b")
		m := MatchTrees(A, B)
		return A.All[1], B.All[1], m
	}
	// A 1 px size change is under the absolute tolerance; 3 px is not.
	a, b, m := pair(base(200, 100), base(201, 100))
	if d := Deltas(a, b, m); d.DW != 0 {
		t.Errorf("1 px width change = %v; want 0 (tolerance)", d.DW)
	}
	a, b, m = pair(base(200, 100), base(203, 100))
	if d := Deltas(a, b, m); d.DW != 3 {
		t.Errorf("3 px width change = %v; want 3", d.DW)
	}
	// 1 % of 800 px wins over the 2 px absolute tolerance.
	a, b, m = pair(base(800, 100), base(810, 100))
	if d := Deltas(a, b, m); d.DW != 10 {
		t.Errorf("10 px on 800 = %v; want 10", d.DW)
	}
	a, b, m = pair(base(800, 100), base(806, 100))
	if d := Deltas(a, b, m); d.DW != 0 {
		t.Errorf("6 px on 800 = %v; want 0 (under 1 %%)", d.DW)
	}
	// An inline box's height is the font's content area: ignored.
	inlineA := n("html", 0, 0, 800, 800, func() schema.Node {
		v := n("span", 0, 0, 100, 18)
		v.D = "inline"
		return v
	}())
	inlineB := n("html", 0, 0, 800, 800, func() schema.Node {
		v := n("span", 0, 0, 100, 22)
		v.D = "inline"
		return v
	}())
	a, b, m = pair(inlineA, inlineB)
	if d := Deltas(a, b, m); d.DH != 0 || d.RH != 0 {
		t.Errorf("inline height delta = %v/%v; want 0", d.DH, d.RH)
	}
	// Position is measured against the parent, so a parent move alone is not
	// a child's movement.
	moveA := n("html", 0, 0, 800, 800, n("div", 0, 0, 800, 400, n("p", 10, 10, 100, 20)))
	moveB := n("html", 0, 0, 800, 800, n("div", 0, 50, 800, 400, n("p", 10, 60, 100, 20)))
	A := Prep(capTree(moveA, 800, 800), "a")
	B := Prep(capTree(moveB, 800, 800), "b")
	m = MatchTrees(A, B)
	if d := Deltas(A.All[2], B.All[2], m); d.DY != 0 {
		t.Errorf("child moved with its parent: dY = %v; want 0", d.DY)
	}
	// Text inset: the text moved 5 px inside an unchanged box.
	tiA := text(n("p", 0, 0, 200, 20), "hello", 1)
	tiB := text(n("p", 0, 0, 200, 20), "hello", 1)
	inset := 5.0
	tiB.TI = &inset
	A = Prep(capTree(n("html", 0, 0, 800, 800, tiA), 800, 800), "a")
	B = Prep(capTree(n("html", 0, 0, 800, 800, tiB), 800, 800), "b")
	m = MatchTrees(A, B)
	if d := Deltas(A.All[1], B.All[1], m); !d.TI {
		t.Errorf("text inset change not detected: %+v", d)
	}
	// Overflow: a 1 px spill is noise; 10 px appearing flips.
	overA := n("html", 0, 0, 800, 800, n("div", 0, 0, 100, 50))
	ov := [2]float64{10, 0}
	overDiv := n("div", 0, 0, 100, 50)
	overDiv.O = &ov
	overB := n("html", 0, 0, 800, 800, overDiv)
	A = Prep(capTree(overA, 800, 800), "a")
	B = Prep(capTree(overB, 800, 800), "b")
	m = MatchTrees(A, B)
	if d := Deltas(A.All[1], B.All[1], m); d.Ovf != "x" {
		t.Errorf("overflow flip = %q; want x", d.Ovf)
	}
}

func TestWidthFlowsDown(t *testing.T) {
	// The parent narrows; the child fills it. Only the parent is a cause.
	A := Prep(capTree(n("html", 0, 0, 800, 800,
		nid("div", "parent", 0, 0, 400, 100,
			nid("div", "child", 0, 0, 400, 40))), 800, 800), "a")
	B := Prep(capTree(n("html", 0, 0, 800, 800,
		nid("div", "parent", 0, 0, 300, 100,
			nid("div", "child", 0, 0, 300, 40))), 800, 800), "b")
	m := MatchTrees(A, B)
	res := Analyze(A, B, m, map[int]bool{}, map[int]bool{})
	var kinds []string
	for _, c := range res.Causes {
		if c.Node.N.ID == "child" {
			kinds = c.Kinds
		}
	}
	if len(kinds) != 0 {
		t.Errorf("child of a narrowed parent has causes %v; want none (it follows the parent)", kinds)
	}
	foundParent := false
	for _, c := range res.Causes {
		if c.Node.N.ID == "parent" && contains(c.Kinds, "width") {
			foundParent = true
		}
	}
	if !foundParent {
		t.Errorf("parent width change not a cause: %+v", res.Causes)
	}
}

func TestMovesKidsRule(t *testing.T) {
	// Two children move down, none resizes: one cause on the parent.
	kid := func(y float64) schema.Node {
		return n("li", 0, y, 100, 20)
	}
	// Same parent box; a padding/gap change pushes both children down.
	A := Prep(capTree(n("html", 0, 0, 800, 800, nid("ul", "list", 0, 0, 100, 100, kid(0), kid(30))), 800, 800), "a")
	B := Prep(capTree(n("html", 0, 0, 800, 800, nid("ul", "list", 0, 0, 100, 100, kid(10), kid(40))), 800, 800), "b")
	m := MatchTrees(A, B)
	res := Analyze(A, B, m, map[int]bool{}, map[int]bool{})
	var parentKinds []string
	childCauses := 0
	for _, c := range res.Causes {
		switch c.Node.N.ID {
		case "list":
			parentKinds = c.Kinds
		default:
			childCauses++
		}
	}
	if !contains(parentKinds, "children-moved") {
		t.Errorf("parent kinds %v; want children-moved", parentKinds)
	}
	if childCauses != 0 {
		t.Errorf("%d child causes; want children explained by the parent", childCauses)
	}
}

func TestOnlyInAndChildrenDiffer(t *testing.T) {
	// One extra child on B: an only-in-B cause at the child.
	A := Prep(capTree(n("html", 0, 0, 800, 800, nid("div", "box", 0, 0, 100, 100)), 800, 800), "a")
	B := Prep(capTree(n("html", 0, 0, 800, 800, nid("div", "box", 0, 0, 100, 100,
		n("span", 5, 5, 20, 10))), 800, 800), "b")
	res := Analyze(A, B, MatchTrees(A, B), map[int]bool{}, map[int]bool{})
	if len(res.Causes) != 1 || res.Causes[0].Side != "B" || !contains(res.Causes[0].Kinds, "only-in-b") {
		t.Errorf("causes %+v; want one only-in-b", res.Causes)
	}
	// Extras on both sides: one children-differ cause on the parent.
	A = Prep(capTree(n("html", 0, 0, 800, 800, nid("div", "box", 0, 0, 100, 100,
		n("span", 5, 5, 20, 10))), 800, 800), "a")
	B = Prep(capTree(n("html", 0, 0, 800, 800, nid("div", "box", 0, 0, 100, 100,
		n("b", 5, 5, 20, 10))), 800, 800), "b")
	res = Analyze(A, B, MatchTrees(A, B), map[int]bool{}, map[int]bool{})
	var got []string
	for _, c := range res.Causes {
		got = append(got, c.Kinds[0]+"@"+Selector(c.Node))
	}
	if len(got) != 1 || !strings.Contains(got[0], "children-differ") {
		t.Errorf("causes %v; want one children-differ on div#box", got)
	}
}

func TestTableColumnsCollapse(t *testing.T) {
	cell := func(x, w float64, label string) schema.Node {
		return n("td", x, 0, w, 20, text(n("span", x, 0, w, 20), label, 1))
	}
	A := Prep(capTree(n("html", 0, 0, 800, 800,
		nid("table", "t", 0, 0, 400, 40,
			n("tbody", 0, 0, 400, 40,
				n("tr", 0, 0, 400, 20, cell(0, 200, "a"), cell(200, 200, "b")),
				n("tr", 0, 20, 400, 20, cell(0, 200, "c"), cell(200, 200, "d"))))), 800, 800), "a")
	// The table keeps its width; the columns shift 40 px while the text is
	// the same: one columns cause on the table.
	B := Prep(capTree(n("html", 0, 0, 800, 800,
		nid("table", "t", 0, 0, 400, 40,
			n("tbody", 0, 0, 400, 40,
				n("tr", 0, 0, 400, 20, cell(0, 160, "a"), cell(160, 240, "b")),
				n("tr", 0, 20, 400, 20, cell(0, 160, "c"), cell(160, 240, "d"))))), 800, 800), "b")
	m := MatchTrees(A, B)
	res := Analyze(A, B, m, map[int]bool{}, map[int]bool{})
	counts := map[string]int{}
	for _, c := range res.Causes {
		counts[strings.Join(c.Kinds, ",")]++
	}
	if counts["columns"] != 1 || counts["width"] != 0 {
		t.Errorf("causes %v; want one columns and no cell width causes", counts)
	}
}

func TestDynamicMask(t *testing.T) {
	// A carousel child moves between runs: masked with its subtree.
	run := func(y float64) *Tree {
		return Prep(capTree(n("html", 0, 0, 800, 800,
			nid("div", "carousel", 0, 0, 200, 100, n("div", y, 0, 100, 20)),
			nid("div", "static", 0, 200, 200, 100)), 800, 800), "chrome")
	}
	mask := DynamicMask(run(0), run(50))
	if len(mask) == 0 {
		t.Fatal("nothing masked")
	}
	first := run(0)
	carousel := first.All[1]
	static := first.All[3]
	if !mask[carousel.UID] || mask[static.UID] {
		t.Errorf("mask: carousel %v, static %v; want only the carousel", mask[carousel.UID], mask[static.UID])
	}
}

func TestPairRunsPicksMatchingStates(t *testing.T) {
	tree := func(docH, nodes int) *Tree {
		return Prep(&schema.Capture{Meta: schema.Meta{DocH: docH, Nodes: nodes}, Root: schema.Node{T: "html"}}, "e")
	}
	// Chrome b matches WebKit a (same slide); a/a would be far apart.
	A, A2, B, _, runA, runB := PairRuns(tree(1000, 50), tree(1400, 50), tree(1400, 50), tree(1000, 49))
	if runA != "b" || runB != "a" {
		t.Errorf("pairing %s/%s; want b/a", runA, runB)
	}
	if A.Meta.DocH != 1400 || A2.Meta.DocH != 1000 || B.Meta.DocH != 1400 {
		t.Errorf("swapped trees wrong: A %d, A2 %d, B %d", A.Meta.DocH, A2.Meta.DocH, B.Meta.DocH)
	}
}

func TestWarnings(t *testing.T) {
	mk := func(cw, docH, nodes int, postH int, failed []schema.FailedRequest, fonts []string) *Tree {
		c := capTree(n("html", 0, 0, 1280, 800), 1280, docH)
		c.Meta.CW = cw
		c.Meta.Nodes = nodes
		c.Meta.Post = schema.Post{DocH: postH, Nodes: nodes}
		c.Meta.Failed = failed
		c.Meta.Fonts.Failed = fonts
		return Prep(c, "chrome")
	}
	A := mk(1280, 1000, 100, 1001, nil, nil)
	B := mk(1265, 1000, 100, 1001, []schema.FailedRequest{{URL: "https://ads.example.net/x.js", Err: "net::ERR_BLOCKED_BY_CLIENT"}}, nil)
	w := WarningsOf(Pair{A: A, A2: A, B: B, B2: B, MaskA: map[int]bool{}, MaskB: map[int]bool{}})
	joined := strings.Join(w.List, "\n")
	for _, want := range []string{"layout viewport differs: chrome 1280px vs chrome 1265px", "blocked requests chrome loaded: ads.example.net"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings %q missing %q", joined, want)
		}
	}
	// The page changed between geometry and screenshot.
	B = mk(1280, 1000, 100, 1200, nil, nil)
	w = WarningsOf(Pair{A: A, A2: A, B: B, B2: B, MaskA: map[int]bool{}, MaskB: map[int]bool{}})
	if !strings.Contains(strings.Join(w.List, "\n"), "page changed during capture") {
		t.Errorf("warnings %q; want the capture-changed warning", w.List)
	}
	// Different page states: named, not a list of layout causes.
	C := mk(1280, 1400, 300, 1401, nil, nil)
	w = WarningsOf(Pair{A: A, A2: A, B: C, B2: C, MaskA: map[int]bool{}, MaskB: map[int]bool{}})
	if !w.StateGap || !strings.Contains(strings.Join(w.List, "\n"), "different page states") {
		t.Errorf("warnings %q; want the page-state warning", w.List)
	}
	// A missing second run is a warning.
	w = WarningsOf(Pair{A: A, B: A, MaskA: map[int]bool{}, MaskB: map[int]bool{}})
	if !strings.Contains(strings.Join(w.List, "\n"), "second run missing") {
		t.Errorf("warnings %q; want the missing-run warning", w.List)
	}
}
