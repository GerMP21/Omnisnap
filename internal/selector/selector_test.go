package selector

import (
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The expected strings are CSS.escape() in Chrome 158.0.8127.36, recorded with
//
//	google-chrome-stable --headless --dump-dom file:///path/esc.html
//
// where esc.html sets document.body.textContent to
// JSON.stringify(inputs.map(s => [s, CSS.escape(s)])) for these inputs.
func TestIdentMatchesCSSEscape(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"md:w-1/2", `md\:w-1\/2`},
		{"1col", `\31 col`},
		{"-1a", `-\31 a`},
		{"-", `\-`},
		{"--", "--"},
		{"-a", "-a"},
		{"a b", `a\ b`},
		{":r1:", `\:r1\:`},
		{"\x00x", "�x"},
		{"a\x01b", `a\1 b`},
		{"a\x7fb", `a\7f b`},
		{"été", "été"},
		{"日本", "日本"},
		{"a_b-c", "a_b-c"},
		{"", ""},
		{"_", "_"},
		{"#x.y", `\#x\.y`},
		{`a\b`, `a\\b`},
		{"😀", "😀"},
		{"0", `\30 `},
		{"-0", `-\30 `},
		{"a1", "a1"},
		{"[x]", `\[x\]`},
		{"\u0080", "\u0080"},
	} {
		if got := Ident(tc.in); got != tc.want {
			t.Errorf("Ident(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// tree is a test parent chain: parents[i] is node i's parent index, -1 at the
// root.
type tree struct {
	nodes   []*schema.Node
	parents []int
}

func (tr tree) path(i int) string {
	return Path(i, func(i int) (int, bool) { return tr.parents[i], tr.parents[i] >= 0 },
		func(i int) *schema.Node { return tr.nodes[i] })
}

// chain builds html > body > ...levels, continuing through child pick[i] of
// level i, and returns the tree with the last node's index.
func chain(levels [][]schema.Node, pick []int) (tree, int) {
	root := &schema.Node{T: "html", K: []schema.Node{{T: "body"}}}
	tr := tree{nodes: []*schema.Node{root, &root.K[0]}, parents: []int{-1, 0}}
	for i, kids := range levels {
		parent := tr.nodes[len(tr.nodes)-1]
		parent.K = kids
		tr.nodes = append(tr.nodes, &parent.K[pick[i]])
		tr.parents = append(tr.parents, len(tr.nodes)-2)
	}
	return tr, len(tr.nodes) - 1
}

func TestPath(t *testing.T) {
	for _, tc := range []struct {
		name   string
		levels [][]schema.Node
		pick   []int // the child each level continues through
		want   string
	}{
		{
			// The hidden first card is not captured: nt still counts it.
			name: "hidden first card",
			levels: [][]schema.Node{
				{{T: "div", ID: "projects-list"}},
				{{T: "div", C: "project-card", NT: 2}, {T: "div", C: "project-card", NT: 3}},
			},
			pick: []int{0, 0},
			want: "div#projects-list > div.project-card:nth-of-type(2)",
		},
		{
			// A valid custom element name may hold a '.': unescaped, CSS
			// would read x-a.b as tag x-a with class b.
			name: "custom element with a dot",
			levels: [][]schema.Node{
				{{T: "x-a.b", C: "card"}},
			},
			pick: []int{0},
			want: "body > x-a\\.b.card",
		},
		{
			name: "one rendered p of two",
			levels: [][]schema.Node{
				{{T: "main"}},
				{{T: "h1"}, {T: "p", NT: 2}},
			},
			pick: []int{0, 1},
			want: "body > main > p:nth-of-type(2)",
		},
		{
			// Before nt: captured same-tag siblings, as the spike counted.
			name: "no nt among captured duplicates",
			levels: [][]schema.Node{
				{{T: "ul"}},
				{{T: "li"}, {T: "li"}, {T: "li"}},
			},
			pick: []int{0, 1},
			want: "body > ul > li:nth-of-type(2)",
		},
		{
			name: "unique tag",
			levels: [][]schema.Node{
				{{T: "header"}, {T: "main", C: "a b c"}},
			},
			pick: []int{0},
			want: "body > header",
		},
		{
			name: "two classes at most, escaped",
			levels: [][]schema.Node{
				{{T: "div", C: "md:flex w-1/2 hidden"}},
			},
			pick: []int{0},
			want: `body > div.md\:flex.w-1\/2`,
		},
		{
			name: "escaped id anchors",
			levels: [][]schema.Node{
				{{T: "section"}},
				{{T: "div", ID: ":r1:"}},
			},
			pick: []int{0, 0},
			want: `div#\:r1\:`,
		},
		{
			name: "cut at five segments",
			levels: [][]schema.Node{
				{{T: "main"}}, {{T: "section"}}, {{T: "div"}}, {{T: "ul"}}, {{T: "li"}}, {{T: "a"}},
			},
			pick: []int{0, 0, 0, 0, 0, 0},
			want: "section > div > ul > li > a",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr, last := chain(tc.levels, tc.pick)
			if got := tr.path(last); got != tc.want {
				t.Errorf("Path = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestPathRoot(t *testing.T) {
	tr := tree{nodes: []*schema.Node{{T: "html"}}, parents: []int{-1}}
	if got := tr.path(0); got != "html" {
		t.Errorf("Path(root) = %q; want html", got)
	}
}
