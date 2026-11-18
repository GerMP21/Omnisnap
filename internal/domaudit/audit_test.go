package domaudit

import (
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// capture builds a minimal capture: viewport 1280x800, one root and children.
func capture(children ...schema.Node) *schema.Capture {
	c := &schema.Capture{Meta: schema.Meta{VW: 1280, VH: 800, DocW: 1280, DocH: 800, Engine: "chrome"}}
	c.Root = schema.Node{T: "html", B: [4]float64{0, 0, 1280, 800}, D: "block"}
	c.Root.K = children
	return c
}

func text(n schema.Node, s string) schema.Node {
	n.Text = &schema.Text{TX: s, LN: 1}
	return n
}

func interactive(n schema.Node) schema.Node { n.IA = schema.Flag(true); return n }

// fires runs the audit and keeps the findings of one check.
func fires(c *schema.Capture, check string) []Finding {
	var out []Finding
	for _, f := range Audit(c) {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

func TestPageHorizontalScroll(t *testing.T) {
	c := capture()
	c.Meta.DocW = 1283
	if got := fires(c, "page-horizontal-scroll"); len(got) != 1 || got[0].Severity != Report {
		t.Errorf("findings %+v; want one report finding", got)
	}
	c.Meta.DocW = 1281 // 1 px: rounding
	if got := fires(c, "page-horizontal-scroll"); len(got) != 0 {
		t.Errorf("1 px wider fired: %+v", got)
	}
}

func TestClippedText(t *testing.T) {
	over := [2]float64{40, 0}
	box := schema.Node{T: "div", B: [4]float64{0, 0, 200, 40}, D: "block", O: &over, OF: "hidden/visible",
		K: []schema.Node{text(schema.Node{T: "span", B: [4]float64{0, 0, 240, 20}, D: "inline"}, "a long line")}}
	c := capture(box)
	got := fires(c, "clipped-text")
	if len(got) != 1 || got[0].Selector != "html > div > span" {
		t.Fatalf("findings %+v; want one at the text owner", got)
	}
	if got[0].Severity != Report {
		t.Errorf("severity %q; want report", got[0].Severity)
	}
	// 1 px: rounding, not clipping.
	small := [2]float64{1, 0}
	box.O = &small
	if got := fires(capture(box), "clipped-text"); len(got) != 0 {
		t.Errorf("1 px overflow fired: %+v", got)
	}
	// More than a viewport: deliberate hiding (image replacement).
	big := [2]float64{2000, 0}
	box.O = &big
	if got := fires(capture(box), "clipped-text"); len(got) != 0 {
		t.Errorf("a viewport of hidden content fired: %+v", got)
	}
	// Visible overflow is not clipping.
	box.OF = "visible/visible"
	box.O = &over
	if got := fires(capture(box), "clipped-text"); len(got) != 0 {
		t.Errorf("visible overflow fired: %+v", got)
	}
}

func TestTruncatedText(t *testing.T) {
	n := text(schema.Node{T: "p", B: [4]float64{0, 0, 100, 20}, D: "block"}, "hello…")
	n.TR = schema.Flag(true)
	if got := fires(capture(n), "truncated-text"); len(got) != 1 || got[0].Severity != Info {
		t.Errorf("findings %+v; want one info finding", got)
	}
}

func TestOverlapInteractive(t *testing.T) {
	a := interactive(schema.Node{T: "button", ID: "a", B: [4]float64{0, 0, 100, 40}, D: "block"})
	b := interactive(schema.Node{T: "button", ID: "b", B: [4]float64{90, 10, 100, 40}, D: "block"})
	if got := fires(capture(a, b), "overlap-interactive"); len(got) != 2 {
		t.Errorf("findings %+v; want both boxes reported", got)
	}
	// 3 px overlap: under the threshold.
	b.B = [4]float64{97, 10, 100, 40}
	if got := fires(capture(a, b), "overlap-interactive"); len(got) != 0 {
		t.Errorf("3 px overlap fired: %+v", got)
	}
	// Containment is a pattern, not a bug.
	inner := interactive(schema.Node{T: "a", B: [4]float64{10, 10, 80, 20}, D: "inline"})
	outer := interactive(schema.Node{T: "a", B: [4]float64{0, 0, 100, 40}, D: "block", K: []schema.Node{inner}})
	if got := fires(capture(outer), "overlap-interactive"); len(got) != 0 {
		t.Errorf("contained interactive fired: %+v", got)
	}
	// Invisible elements do not overlap.
	b = interactive(schema.Node{T: "button", ID: "b", B: [4]float64{90, 10, 100, 40}, D: "block"})
	b.Hid = schema.Flag(true)
	if got := fires(capture(a, b), "overlap-interactive"); len(got) != 0 {
		t.Errorf("invisible element fired: %+v", got)
	}
}

func TestOffscreenInteractive(t *testing.T) {
	left := interactive(schema.Node{T: "a", B: [4]float64{-200, 0, 100, 40}, D: "block"})
	right := interactive(schema.Node{T: "a", B: [4]float64{1300, 0, 100, 40}, D: "block"})
	got := fires(capture(left, right), "offscreen-interactive")
	if len(got) != 2 {
		t.Errorf("findings %+v; want both", got)
	}
	// Partly visible is fine.
	partial := interactive(schema.Node{T: "a", B: [4]float64{-50, 0, 100, 40}, D: "block"})
	if got := fires(capture(partial), "offscreen-interactive"); len(got) != 0 {
		t.Errorf("partly visible fired: %+v", got)
	}
	// Zero-width at 0 is the zero-size check's case, not offscreen.
	zero := interactive(schema.Node{T: "a", B: [4]float64{0, 0, 0, 20}, D: "inline"})
	if got := fires(capture(zero), "offscreen-interactive"); len(got) != 0 {
		t.Errorf("zero-width element fired offscreen: %+v", got)
	}
}

func TestZeroSizeInteractive(t *testing.T) {
	empty := interactive(text(schema.Node{T: "a", B: [4]float64{10, 10, 0, 40}, D: "inline"}, "Go"))
	if got := fires(capture(empty), "zero-size-interactive"); len(got) != 1 {
		t.Errorf("findings %+v; want one", got)
	}
	// No text: an icon link with a zero box is caught by overlap or not at all.
	noText := interactive(schema.Node{T: "a", B: [4]float64{10, 10, 0, 40}, D: "inline"})
	if got := fires(capture(noText), "zero-size-interactive"); len(got) != 0 {
		t.Errorf("text-less element fired: %+v", got)
	}
}

func TestProtrudesParent(t *testing.T) {
	parent := schema.Node{T: "div", B: [4]float64{100, 100, 200, 100}, D: "block", OF: "visible/visible",
		K: []schema.Node{
			{T: "div", ID: "wide", B: [4]float64{100, 100, 260, 40}, D: "block"},
			{T: "div", ID: "sib", B: [4]float64{280, 100, 20, 40}, D: "block"},
		}}
	got := fires(capture(parent), "protrudes-parent")
	if len(got) != 1 || got[0].Selector != "html > div > div#wide" {
		t.Fatalf("findings %+v; want the wide child", got)
	}
	// No sibling overlap: a child sticking out into empty space (a tooltip).
	alone := schema.Node{T: "div", B: [4]float64{100, 100, 200, 100}, D: "block", OF: "visible/visible",
		K: []schema.Node{{T: "div", ID: "wide", B: [4]float64{100, 100, 260, 40}, D: "block"}}}
	if got := fires(capture(alone), "protrudes-parent"); len(got) != 0 {
		t.Errorf("protrusion without a sibling fired: %+v", got)
	}
	// A clipping parent contains its children by design.
	clip := alone
	clip.OF = "hidden/hidden"
	if got := fires(capture(clip), "protrudes-parent"); len(got) != 0 {
		t.Errorf("clipped parent fired: %+v", got)
	}
	// A parent with no box (a zero-height body around absolute children)
	// cannot visibly contain anything.
	noBox := parent
	noBox.B = [4]float64{0, 0, 1280, 0}
	noBox.K = []schema.Node{
		{T: "button", ID: "a", B: [4]float64{10, 10, 100, 40}, D: "block", IA: schema.Flag(true)},
		{T: "button", ID: "b", B: [4]float64{80, 25, 100, 40}, D: "block", IA: schema.Flag(true)},
	}
	if got := fires(capture(noBox), "protrudes-parent"); len(got) != 0 {
		t.Errorf("zero-box parent fired: %+v", got)
	}
}

// The whole audit on a clean capture: nothing fires.
func TestCleanCaptureHasNoFindings(t *testing.T) {
	c := capture(schema.Node{T: "main", B: [4]float64{0, 0, 1280, 400}, D: "block",
		K: []schema.Node{text(schema.Node{T: "p", B: [4]float64{10, 10, 300, 40}, D: "block"}, "hello")}})
	if got := Audit(c); len(got) != 0 {
		t.Errorf("findings %+v; want none", got)
	}
}

// 5,000 elements must audit in under 50 ms (Task 34, success criterion 4).
func TestAuditIsFast(t *testing.T) {
	root := schema.Node{T: "html", B: [4]float64{0, 0, 1280, 800}, D: "block"}
	row := schema.Node{T: "div", B: [4]float64{0, 0, 1280, 40}, D: "block"}
	for i := range 20 {
		row.K = append(row.K, schema.Node{T: "div", B: [4]float64{float64(i * 60), 0, 50, 20}, D: "block"})
	}
	for range 240 { // 240 rows x 20 cells ≈ 5,000 nodes
		root.K = append(root.K, row)
	}
	c := capture(root)
	start := time.Now()
	findings := Audit(c)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("audit of ~5,000 nodes took %v; want < 50 ms", d)
	}
	_ = findings
}

func BenchmarkAudit(b *testing.B) {
	root := schema.Node{T: "html", B: [4]float64{0, 0, 1280, 800}, D: "block"}
	row := schema.Node{T: "div", B: [4]float64{0, 0, 1280, 40}, D: "block"}
	for i := range 20 {
		row.K = append(row.K, schema.Node{T: "div", B: [4]float64{float64(i * 60), 0, 50, 20}, D: "block"})
	}
	for range 240 {
		root.K = append(root.K, row)
	}
	c := capture(root)
	b.ResetTimer()
	for range b.N {
		_ = Audit(c)
	}
}
