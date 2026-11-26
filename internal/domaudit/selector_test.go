package domaudit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Change fix-selector-sibling-index: an audit finding names its element with
// the selector a layout-diff cause uses, DOM position included.

func TestFindingSelectorHasPosition(t *testing.T) {
	button := func(x float64) schema.Node {
		return interactive(text(schema.Node{T: "button", B: [4]float64{x, 0, 0, 20}, D: "inline-block"}, "Go"))
	}
	bar := schema.Node{T: "div", C: "toolbar", B: [4]float64{0, 0, 300, 20}, D: "block",
		K: []schema.Node{button(0), button(100), button(200)}}
	bar.K[0].B[2], bar.K[1].B[2] = 80, 80 // only the third is zero-size
	got := fires(capture(bar), "zero-size-interactive")
	if len(got) != 1 || got[0].Selector != "html > div.toolbar > button:nth-of-type(3)" {
		t.Fatalf("findings %+v; want the third button", got)
	}
	// With nt the DOM position wins: a hidden button before the three makes
	// the zero-size one the fourth.
	for i := range bar.K {
		bar.K[i].NT = i + 2
	}
	got = fires(capture(bar), "zero-size-interactive")
	if len(got) != 1 || got[0].Selector != "html > div.toolbar > button:nth-of-type(4)" {
		t.Fatalf("findings %+v; want button:nth-of-type(4)", got)
	}
}

// Every node of a golden capture (no nt: the fallback) and of a capture with
// nt gets the same selector from the audit as from layoutdiff.Selector. Both
// walk the tree in document order, so their indices line up.
func TestSelectorEqualsLayoutDiff(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "planted", "19-list-rhythm", "chrome-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden schema.Capture
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	withNT := capture(
		schema.Node{T: "ul", ID: "steps", B: [4]float64{0, 0, 480, 60}, D: "block", K: []schema.Node{
			{T: "li", NT: 1, B: [4]float64{0, 0, 480, 20}, D: "list-item"},
			{T: "li", NT: 3, B: [4]float64{0, 20, 480, 20}, D: "list-item"},
		}},
		schema.Node{T: "div", C: "md:flex a b", B: [4]float64{0, 60, 480, 20}, D: "block"},
	)
	for name, c := range map[string]*schema.Capture{"golden 19-list-rhythm": &golden, "with nt": withNT} {
		w := walk(c)
		tr := layoutdiff.Prep(c, "chrome")
		if len(w.entries) != len(tr.All) {
			t.Fatalf("%s: %d entries, %d layoutdiff nodes", name, len(w.entries), len(tr.All))
		}
		for i, e := range w.entries {
			if got, want := w.selector(e), layoutdiff.Selector(tr.All[i]); got != want {
				t.Errorf("%s node %d: audit %q; layoutdiff %q", name, i, got, want)
			}
		}
	}
}
