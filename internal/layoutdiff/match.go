// Package layoutdiff compares two engines' geometry trees (SPEC-layout-diff.md):
// it matches elements, measures what changed and explains where a difference
// starts. It is a port of the spike's lib/align.mjs, the algorithm validated in
// spikes/layout-diff: golden parity (testdata/golden) is the gate for changes.
package layoutdiff

import (
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/selector"
)

// Tolerances of the spike (lib/align.mjs TOL). Changing them changes what is
// reported: ask first.
const (
	TolAbs      = 2.0  // px a box size may differ before it counts as changed
	TolRel      = 0.01 // ...or this fraction of the larger size, whichever is bigger
	TolPos      = 2.0  // px a parent-relative position may shift
	TolOvf      = 2.0  // px of content beyond a box before it counts as overflowing
	TolOvfDelta = 4.0  // ...and the overflow itself must change by more than this
)

// Node is one element of a prepared tree: the capture's node plus the links
// the analysis needs (parent, children, a stable uid).
type Node struct {
	N      *schema.Node
	UID    int
	Parent *Node
	Kids   []*Node
}

// Text accessors: schema.Node embeds *Text, which is nil for elements without
// their own text. Reading a field through the nil pointer panics, so every
// access goes through these.

// LN is the line count (0 without own text).
func (n *Node) LN() int {
	if n.N.Text == nil {
		return 0
	}
	return n.N.LN
}

// TX is the first characters of the element's own text.
func (n *Node) TX() string {
	if n.N.Text == nil {
		return ""
	}
	return n.N.TX
}

// TW is the text extent.
func (n *Node) TW() float64 {
	if n.N.Text == nil {
		return 0
	}
	return n.N.TW
}

// TI is the text inset, or nil.
func (n *Node) TI() *float64 {
	if n.N.Text == nil {
		return nil
	}
	return n.N.TI
}

// HasText reports whether the element owns text.
func (n *Node) HasText() bool { return n.N.Text != nil }

// TH is the text hash.
func (n *Node) TH() string {
	if n.N.Text == nil {
		return ""
	}
	return n.N.TH
}

// Tree is one capture prepared for comparison.
type Tree struct {
	Root   *Node
	All    []*Node
	Engine string
	Meta   schema.Meta
}

// Prep wraps a capture's tree.
func Prep(c *schema.Capture, engine string) *Tree {
	t := &Tree{Engine: engine, Meta: c.Meta}
	var add func(n *schema.Node, parent *Node) *Node
	add = func(n *schema.Node, parent *Node) *Node {
		node := &Node{N: n, UID: len(t.All), Parent: parent}
		t.All = append(t.All, node)
		for i := range n.K {
			node.Kids = append(node.Kids, add(&n.K[i], node))
		}
		return node
	}
	t.Root = add(&c.Root, nil)
	if engine == "" {
		t.Engine = c.Meta.Engine
	}
	return t
}

// Subtree yields n and its descendants in document order.
func Subtree(n *Node) []*Node {
	out := []*Node{n}
	for _, k := range n.Kids {
		out = append(out, Subtree(k)...)
	}
	return out
}

// Selector names n for people and for devtools (package selector): valid CSS,
// with the DOM position (the capture's nt) when the parent has other children
// of n's tag, at most five segments, starting at the nearest id, body or html.
func Selector(n *Node) string {
	return selector.Path(n,
		func(n *Node) (*Node, bool) { return n.Parent, n.Parent != nil },
		func(n *Node) *schema.Node { return n.N })
}

// token is an element's strongest available identity.
func token(n *Node) string {
	m := n.N
	switch {
	case m.ID != "":
		return "i:" + m.ID
	case m.TID != "":
		return "t:" + m.TID
	case m.U != "":
		return m.T + "|u:" + m.U
	case m.N != "":
		return m.T + "|n:" + m.N
	case n.TH() != "":
		return m.T + "|h:" + n.TH()
	}
	return m.T + "|c:" + m.C
}

// Match maps the two trees' nodes.
type Match struct {
	PairOf  map[int]*Node // A uid -> B node
	RevPair map[int]*Node // B uid -> A node
	OnlyA   []*Node       // topmost A nodes with no partner (their parent matched)
	OnlyB   []*Node
}

// lcsPairs aligns two token lists. For huge sibling lists it degrades to a
// greedy in-order match with a 50-element lookahead (the spike's guard: the
// dynamic-programming table would be 4M cells).
func lcsPairs(ta, tb []string) [][2]int {
	n, m := len(ta), len(tb)
	if n == 0 || m == 0 {
		return nil
	}
	if n*m > 4_000_000 {
		var pairs [][2]int
		j := 0
		for i := 0; i < n && j < m; i++ {
			k := j
			for k < m && k-j < 50 && tb[k] != ta[i] {
				k++
			}
			if k < m && tb[k] == ta[i] {
				pairs = append(pairs, [2]int{i, k})
				j = k + 1
			}
		}
		return pairs
	}
	w := m + 1
	dp := make([]uint32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if ta[i] == tb[j] {
				dp[i*w+j] = dp[(i+1)*w+j+1] + 1
			} else {
				dp[i*w+j] = max(dp[(i+1)*w+j], dp[i*w+j+1])
			}
		}
	}
	var pairs [][2]int
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case ta[i] == tb[j]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case dp[(i+1)*w+j] >= dp[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

// alignChildren anchors on identity tokens and pairs the leftovers between
// anchors in order by tag+class, then by tag (which rescues dynamic ids and
// changed text).
func alignChildren(ka, kb []*Node) [][2]int {
	ta := make([]string, len(ka))
	for i, n := range ka {
		ta[i] = token(n)
	}
	tb := make([]string, len(kb))
	for i, n := range kb {
		tb[i] = token(n)
	}
	anchors := lcsPairs(ta, tb)
	pairs := append([][2]int{}, anchors...)
	bounds := append([][2]int{{-1, -1}}, anchors...)
	bounds = append(bounds, [2]int{len(ka), len(kb)})
	for g := 0; g+1 < len(bounds); g++ {
		i0, j0 := bounds[g][0], bounds[g][1]
		i1, j1 := bounds[g+1][0], bounds[g+1][1]
		var ga, gb []int
		for i := i0 + 1; i < i1; i++ {
			ga = append(ga, i)
		}
		for j := j0 + 1; j < j1; j++ {
			gb = append(gb, j)
		}
		if len(ga) == 0 || len(gb) == 0 {
			continue
		}
		keys := []func(*Node) string{
			func(n *Node) string { return n.N.T + "." + n.N.C },
			func(n *Node) string { return n.N.T },
		}
		for _, key := range keys {
			from := 0
			for x := range ga {
				if ga[x] < 0 {
					continue
				}
				for y := from; y < len(gb); y++ {
					if gb[y] < 0 {
						continue
					}
					if key(ka[ga[x]]) == key(kb[gb[y]]) {
						pairs = append(pairs, [2]int{ga[x], gb[y]})
						ga[x], gb[y] = -1, -1
						from = y + 1
						break
					}
				}
			}
		}
	}
	return pairs
}

// MatchTrees pairs every element of A with one of B.
func MatchTrees(a, b *Tree) *Match {
	m := &Match{PairOf: map[int]*Node{}, RevPair: map[int]*Node{}}
	type pair struct{ a, b *Node }
	stack := []pair{{a.Root, b.Root}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		m.PairOf[p.a.UID] = p.b
		m.RevPair[p.b.UID] = p.a
		pairs := alignChildren(p.a.Kids, p.b.Kids)
		usedA := map[int]bool{}
		usedB := map[int]bool{}
		for _, pr := range pairs {
			usedA[pr[0]] = true
			usedB[pr[1]] = true
		}
		for i, c := range p.a.Kids {
			if !usedA[i] {
				m.OnlyA = append(m.OnlyA, c)
			}
		}
		for j, c := range p.b.Kids {
			if !usedB[j] {
				m.OnlyB = append(m.OnlyB, c)
			}
		}
		for _, pr := range pairs {
			stack = append(stack, pair{p.a.Kids[pr[0]], p.b.Kids[pr[1]]})
		}
	}
	return m
}
