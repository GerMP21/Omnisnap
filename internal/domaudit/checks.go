package domaudit

import "github.com/GerMP21/Omnisnap/internal/schema"

// The seven checks of SPEC-dom-audit.md, in reporting order. Each is a pure
// function of the walker; thresholds are named in audit.go.

type check struct {
	id  string
	run func(*walker) []Finding
}

var checks = []check{
	{"page-horizontal-scroll", pageHorizontalScroll},
	{"clipped-text", clippedText},
	{"truncated-text", truncatedText},
	{"overlap-interactive", overlapInteractive},
	{"offscreen-interactive", offscreenInteractive},
	{"zero-size-interactive", zeroSizeInteractive},
	{"protrudes-parent", protrudesParent},
}

// pageHorizontalScroll: the document is at least 2 px wider than the viewport.
func pageHorizontalScroll(w *walker) []Finding {
	if w.c.Meta.DocW <= w.c.Meta.VW+horizontalScrollPx {
		return nil
	}
	e := w.entries[0]
	return []Finding{{Check: "page-horizontal-scroll", Selector: "html",
		Box: e.node.B, Severity: Report, Engine: w.c.Meta.Engine}}
}

// clippedText: a text owner inside an element that hides overflow, where the
// hidden amount is at least 2 px and less than a viewport (a larger overflow
// is deliberate hiding, as in image replacement).
func clippedText(w *walker) []Finding {
	var out []Finding
	vw := float64(w.c.Meta.VW)
	for _, e := range w.entries {
		n := e.node
		if n.Text == nil || !w.visible(n) {
			continue
		}
		for p := e.parent; p >= 0; p = w.entries[p].parent {
			a := w.entries[p].node
			if a.O == nil || !w.visible(a) {
				continue
			}
			clipped := false
			for axis := range 2 {
				mode := overflowMode(a, axis)
				if mode != "hidden" && mode != "clip" {
					continue
				}
				over := a.O[axis]
				if over >= clippedMinPx && over < vw {
					clipped = true
				}
			}
			if clipped {
				out = append(out, w.finding("clipped-text", e, Report))
				break
			}
		}
	}
	return out
}

// truncatedText: the extractor marked the text as cut by an ellipsis or a line
// clamp. Informational: it is a design choice as often as a bug.
func truncatedText(w *walker) []Finding {
	var out []Finding
	for _, e := range w.entries {
		if bool(e.node.TR) && w.visible(e.node) {
			out = append(out, w.finding("truncated-text", e, Info))
		}
	}
	return out
}

// overlapInteractive: two visible interactive elements overlap by at least
// 4 px in both directions, and neither contains the other (a button inside a
// link is a pattern, not a bug).
func overlapInteractive(w *walker) []Finding {
	var hits []entry
	for _, e := range w.entries {
		if w.interactive(e.node) && w.visible(e.node) {
			hits = append(hits, e)
		}
	}
	var out []Finding
	for i := range hits {
		for j := i + 1; j < len(hits); j++ {
			a, b := hits[i].node.B, hits[j].node.B
			ox, oy := overlap(a, b)
			if ox < overlapMinPx || oy < overlapMinPx {
				continue
			}
			if contains(a, b) || contains(b, a) {
				continue
			}
			// Report at both: a reader needs to see both boxes.
			out = append(out, w.finding("overlap-interactive", hits[i], Info))
			out = append(out, w.finding("overlap-interactive", hits[j], Info))
		}
	}
	return out
}

// offscreenInteractive: interactive and visible, entirely left of 0 or right
// of the document.
func offscreenInteractive(w *walker) []Finding {
	var out []Finding
	docW := w.docWidth()
	for _, e := range w.entries {
		n := e.node
		if !w.interactive(n) || !w.visible(n) {
			continue
		}
		// A zero-width element at x=0 is degenerate, not offscreen: the
		// zero-size check owns that case.
		if n.B[2] < 1 {
			continue
		}
		if n.B[0]+n.B[2] <= 0 || n.B[0] >= docW {
			out = append(out, w.finding("offscreen-interactive", e, Info))
		}
	}
	return out
}

// zeroSizeInteractive: an interactive, visible element smaller than 1 px in
// one direction but with its own text: nothing to click.
func zeroSizeInteractive(w *walker) []Finding {
	var out []Finding
	for _, e := range w.entries {
		n := e.node
		if !w.interactive(n) || !w.visible(n) || n.Text == nil || n.TX == "" {
			continue
		}
		if n.B[2] < 1 || n.B[3] < 1 {
			out = append(out, w.finding("zero-size-interactive", e, Info))
		}
	}
	return out
}

// protrudesParent: a visible element extends at least 4 px beyond a parent
// whose overflow is visible, and overlaps a sibling.
//
// SPEC-dom-audit.md also asks for a parent with a border or background; the
// capture format does not record either (extract.js has no such field), so the
// geometric part is what can be checked without changing the injected script
// (which needs approval). Recorded as an open question.
func protrudesParent(w *walker) []Finding {
	var out []Finding
	for _, e := range w.entries {
		n := e.node
		if e.parent < 0 || !w.visible(n) {
			continue
		}
		parent := w.entries[e.parent].node
		if !w.visible(parent) {
			continue
		}
		if overflowMode(parent, 0) != "visible" || overflowMode(parent, 1) != "visible" {
			continue
		}
		p := parent.B
		// A parent with no box of its own (a zero-height body around absolutely
		// positioned children) cannot visibly contain anything, so a child
		// outside it is not a protrusion. With no border/background field in
		// the capture, this is the closest honest approximation.
		if p[2] < 1 || p[3] < 1 {
			continue
		}
		b := n.B
		protrudes := b[0] < p[0]-protrudeMinPx || b[1] < p[1]-protrudeMinPx ||
			b[0]+b[2] > p[0]+p[2]+protrudeMinPx || b[1]+b[3] > p[1]+p[3]+protrudeMinPx
		if !protrudes {
			continue
		}
		// It must overlap a sibling: a child that sticks out into empty space
		// is how menus and tooltips work.
		overlapsSibling := false
		for _, si := range w.children[e.parent] {
			if si == e.index {
				continue
			}
			s := w.entries[si]
			if !w.visible(s.node) {
				continue
			}
			ox, oy := overlap(b, s.node.B)
			if ox >= protrudeMinPx && oy >= protrudeMinPx {
				overlapsSibling = true
				break
			}
		}
		if overlapsSibling {
			out = append(out, w.finding("protrudes-parent", e, Info))
		}
	}
	return out
}

var _ = schema.Node{}
