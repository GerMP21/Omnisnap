// Package domaudit reports layout anomalies within one engine's render
// (SPEC-dom-audit.md): problems a page has even if every browser agrees. Pure:
// it reads one capture file and never touches the page.
package domaudit

import (
	"fmt"
	"sort"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Severity is report (worth the user's time) or info (tuned later).
type Severity string

// Severities.
const (
	Report Severity = "report"
	Info   Severity = "info"
)

// Finding is one anomaly.
type Finding struct {
	Check    string     `json:"check"`
	Selector string     `json:"selector"`
	Box      [4]float64 `json:"box"`
	Severity Severity   `json:"severity"`
	Engine   string     `json:"engine,omitempty"`
}

// Thresholds of SPEC-dom-audit.md. Promoting a check to report needs approval;
// the numbers are the spec's initial ones, tuned on the planted suite.
const (
	horizontalScrollPx = 1 // docW > vw + 1: at least 2 px of horizontal scroll
	clippedMinPx       = 2 // overflow of less than this is rounding
	overlapMinPx       = 4 // overlap in both directions
	protrudeMinPx      = 4 // protrusion beyond the parent
)

// Audit runs every check over one capture.
func Audit(c *schema.Capture) []Finding {
	w := walk(c)
	var out []Finding
	for _, check := range checks {
		out = append(out, check.run(w)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == Report
		}
		return out[i].Selector < out[j].Selector
	})
	return out
}

// entry is one node with what the checks need: its parent and a selector.
type entry struct {
	node   *schema.Node
	parent int // -1 for the root
	index  int // position among all entries
	path   string
}

// walker is the flat view of the tree: checks need parents and siblings.
type walker struct {
	c        *schema.Capture
	entries  []entry
	children map[int][]int // parent index -> child indices (sibling scans stay linear)
}

func walk(c *schema.Capture) *walker {
	w := &walker{c: c, children: map[int][]int{}}
	w.add(&c.Root, -1, "html")
	return w
}

func (w *walker) add(n *schema.Node, parent int, path string) {
	e := entry{node: n, parent: parent, index: len(w.entries), path: path}
	w.entries = append(w.entries, e)
	if parent >= 0 {
		w.children[parent] = append(w.children[parent], e.index)
	}
	for i := range n.K {
		child := &n.K[i]
		w.add(child, e.index, childPath(path, child))
	}
}

// childPath is a stable selector: tag, #id, first class.
func childPath(parent string, n *schema.Node) string {
	p := n.T
	if n.ID != "" {
		p += "#" + n.ID
	} else if n.C != "" {
		if first, _, _ := cutAny(n.C, " \t\n"); first != "" {
			p += "." + first
		}
	}
	if parent == "" {
		return p
	}
	return parent + " > " + p
}

func cutAny(s, seps string) (before, after string, found bool) {
	for i := range len(s) {
		for j := range len(seps) {
			if s[i] == seps[j] {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}

func (w *walker) visible(n *schema.Node) bool { return !bool(n.Hid) }

func (w *walker) interactive(n *schema.Node) bool { return bool(n.IA) }

// docWidth is the document width the horizontal-scroll check uses.
func (w *walker) docWidth() float64 { return float64(w.c.Meta.DocW) }

// sel formats a finding.
func (w *walker) finding(check string, e entry, sev Severity) Finding {
	return Finding{Check: check, Selector: e.path, Box: e.node.B, Severity: sev, Engine: w.c.Meta.Engine}
}

// overlap returns how much two boxes overlap in x and y (0 when they do not).
func overlap(a, b [4]float64) (float64, float64) {
	ox := min(a[0]+a[2], b[0]+b[2]) - max(a[0], b[0])
	oy := min(a[1]+a[3], b[1]+b[3]) - max(a[1], b[1])
	return max(0, ox), max(0, oy)
}

// contains reports whether outer fully contains inner.
func contains(outer, inner [4]float64) bool {
	return inner[0] >= outer[0] && inner[1] >= outer[1] &&
		inner[0]+inner[2] <= outer[0]+outer[2] && inner[1]+inner[3] <= outer[1]+outer[3]
}

// overflowMode returns a node's overflow mode for an axis (x=0, y=1).
func overflowMode(n *schema.Node, axis int) string {
	modes := n.OF
	if modes == "" {
		return "visible"
	}
	var parts []string
	if a, b, ok := cutAny(modes, "/"); ok {
		parts = []string{a, b}
	} else {
		parts = []string{modes, modes}
	}
	if axis >= len(parts) {
		return "visible"
	}
	return parts[axis]
}

func (w *walker) String() string {
	return fmt.Sprintf("domaudit: %d nodes", len(w.entries))
}
