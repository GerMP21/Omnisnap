package layoutdiff

import (
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Change fix-selector-sibling-index: a selector's :nth-of-type is the DOM
// position (the capture's nt), so display: none siblings count, as in CSS.

// li is a list item with text t and DOM position nt (0: none recorded).
func li(t string, nt int, y float64) schema.Node {
	v := text(n("li", 0, y, 400, 20), t, 1)
	v.NT = nt
	return v
}

// byText finds the node whose own text is s.
func byText(t *testing.T, tr *Tree, s string) *Node {
	t.Helper()
	for _, x := range tr.All {
		if x.TX() == s {
			return x
		}
	}
	t.Fatalf("no node with text %q", s)
	return nil
}

func TestSelectorHiddenFirstCard(t *testing.T) {
	card := func(s string, nt int, y float64) schema.Node {
		v := text(n("div", 0, y, 320, 80), s, 1)
		v.C, v.NT = "project-card", nt
		return v
	}
	// The first card is display: none, so it is not captured; the rendered
	// ones are the 2nd to 4th div in the DOM.
	tr := Prep(capTree(n("html", 0, 0, 1280, 800, n("body", 0, 0, 1280, 800,
		nid("div", "projects-list", 0, 0, 1280, 300,
			card("Atlas", 2, 0), card("Beacon", 3, 100), card("Cinder", 4, 200)))), 1280, 800), "")
	if got, want := Selector(byText(t, tr, "Atlas")), "div#projects-list > div.project-card:nth-of-type(2)"; got != want {
		t.Errorf("first shown card: %q; want %q", got, want)
	}
	if got, want := Selector(byText(t, tr, "Beacon")), "div#projects-list > div.project-card:nth-of-type(3)"; got != want {
		t.Errorf("second shown card: %q; want %q", got, want)
	}
}

func TestSelectorOneRenderedOfTwo(t *testing.T) {
	// <main><p hidden>…</p><p>Shown</p></main>: one p captured, the second.
	p := text(n("p", 0, 0, 400, 20), "Shown", 1)
	p.NT = 2
	tr := Prep(capTree(n("html", 0, 0, 1280, 800, n("body", 0, 0, 1280, 800,
		n("main", 0, 0, 1280, 100, p))), 1280, 800), "")
	if got, want := Selector(byText(t, tr, "Shown")), "body > main > p:nth-of-type(2)"; got != want {
		t.Errorf("%q; want %q", got, want)
	}
}

func TestSelectorEngineSpecificHiddenSibling(t *testing.T) {
	steps := []string{"Discover", "Define", "Sketch", "Prototype", "Test", "Ship"}
	var a, b []schema.Node
	for i, s := range steps {
		a = append(a, li(s, i+1, float64(20*i)))
		if i != 1 { // the second step is display: none in B only
			b = append(b, li(s, i+1, float64(20*i)))
		}
	}
	ul := func(kids []schema.Node) schema.Node {
		return n("html", 0, 0, 1280, 800, n("body", 0, 0, 1280, 800, nid("ul", "steps", 0, 0, 480, 120, kids...)))
	}
	ta, tb := Prep(capTree(ul(a), 1280, 800), "chrome"), Prep(capTree(ul(b), 1280, 800), "firefox")
	m := MatchTrees(ta, tb)
	inA := byText(t, ta, "Test")
	inB := m.PairOf[inA.UID]
	if inB == nil || inB.TX() != "Test" {
		t.Fatalf("the fifth step is not paired: %v", inB)
	}
	want := "ul#steps > li:nth-of-type(5)"
	if got := Selector(inA); got != want {
		t.Errorf("A: %q; want %q", got, want)
	}
	if got := Selector(inB); got != want {
		t.Errorf("B: %q; want %q (the same element, the same selector)", got, want)
	}
}

func TestSelectorWithoutNTCountsCaptured(t *testing.T) {
	// A capture from before nt: the captured same-tag siblings are counted,
	// which is the selector the goldens hold.
	tr := Prep(capTree(n("html", 0, 0, 1280, 800, n("body", 0, 0, 1280, 800,
		n("ul", 0, 0, 480, 60, li("One", 0, 0), li("Two", 0, 20), li("Three", 0, 40)))), 1280, 800), "")
	if got, want := Selector(byText(t, tr, "Two")), "body > ul > li:nth-of-type(2)"; got != want {
		t.Errorf("%q; want %q", got, want)
	}
}

func TestSelectorUniqueTag(t *testing.T) {
	h := text(n("h1", 0, 0, 400, 40), "Title", 1)
	tr := Prep(capTree(n("html", 0, 0, 1280, 800, n("body", 0, 0, 1280, 800,
		n("main", 0, 0, 1280, 100, h, li("Lone", 0, 40)))), 1280, 800), "")
	if got, want := Selector(byText(t, tr, "Title")), "body > main > h1"; got != want {
		t.Errorf("%q; want %q", got, want)
	}
}
